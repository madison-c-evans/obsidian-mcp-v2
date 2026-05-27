package vault

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// newWriteTestVault materializes a small fixture vault on disk under a
// per-test tempdir, builds the index, and returns the live Vault plus the
// absolute path. Layout:
//
//	<root>/
//	  Alpha.md          (note, upstream -> "Topic One")
//	  Beta.md           (note, upstream -> "Alpha", mentions [[Alpha]] and [[Alpha|alias]])
//	  Gamma.md          (note, plain body, no upstream)
//	  topics/
//	    Topic One.md    (topic)
//
// This helper is intentionally named distinctly from the read-side test
// helper to avoid collision when both files land in the same package.
func newWriteTestVault(t *testing.T) (*Vault, string) {
	t.Helper()
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, "topics"), 0o755); err != nil {
		t.Fatalf("mkdir topics: %v", err)
	}

	writeFile := func(rel, contents string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}

	writeFile("topics/Topic One.md", `---
tags:
  - topic
external-links:
upstream:
date: 2024-01-01
status:
description:
---
Topic One body.
`)

	writeFile("Alpha.md", `---
tags:
  - notes
external-links:
upstream: "[[Topic One]]"
date: 2024-01-02
status: TODO
---
## Intro
Alpha intro.

## Details
Alpha details.

### Nested
Nested detail.

## Closing
Alpha closing.
`)

	writeFile("Beta.md", `---
tags:
  - notes
external-links:
upstream: "[[Alpha]]"
date: 2024-01-03
status: DONE
---
Beta refs [[Alpha]] and aliased [[Alpha|alpha-display]].
`)

	writeFile("Gamma.md", `---
tags:
  - notes
external-links:
upstream:
date: 2024-01-04
status: TODO
---
Gamma body.
`)

	v, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := v.BuildIndex(); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	return v, root
}

// readNoteRaw returns the on-disk contents of a note file.
func readNoteRaw(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

func TestNewWriteTestVault_SetsUpFixture(t *testing.T) {
	v, root := newWriteTestVault(t)

	for _, title := range []string{"Alpha", "Beta", "Gamma", "Topic One"} {
		if v.notes[title] == nil {
			t.Fatalf("expected note %q in fixture vault", title)
		}
	}
	if got := v.notes["Topic One"].IsTopic; !got {
		t.Errorf("Topic One.IsTopic = false, want true")
	}
	if got := v.notes["Alpha"].IsTopic; got {
		t.Errorf("Alpha.IsTopic = true, want false")
	}
	wantPath := filepath.Join(root, "Alpha.md")
	if v.notes["Alpha"].FilePath != wantPath {
		t.Errorf("Alpha.FilePath = %q, want %q", v.notes["Alpha"].FilePath, wantPath)
	}
}

func TestCreateNote_DefaultNote(t *testing.T) {
	v, root := newWriteTestVault(t)

	note, err := v.CreateNote(CreateOptions{
		Title:   "Fresh Note",
		Content: "Hello world.",
	})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	if note.Title != "Fresh Note" {
		t.Errorf("Title = %q, want %q", note.Title, "Fresh Note")
	}
	if note.FilePath != filepath.Join(root, "Fresh Note.md") {
		t.Errorf("FilePath = %q, want vault root", note.FilePath)
	}
	if note.IsTopic {
		t.Errorf("IsTopic = true, want false")
	}
	if !equalStringSlices(note.Frontmatter.Tags, []string{"notes"}) {
		t.Errorf("Tags = %v, want [notes]", note.Frontmatter.Tags)
	}
	if note.Frontmatter.Status != "TODO" {
		t.Errorf("Status = %q, want TODO", note.Frontmatter.Status)
	}
	if note.Frontmatter.Description != "" {
		t.Errorf("Description = %q, want empty", note.Frontmatter.Description)
	}

	raw := readNoteRaw(t, note.FilePath)
	// Note (non-topic) without HasDesc should NOT emit a `description:` key.
	if strings.Contains(raw, "description:") {
		t.Errorf("default note unexpectedly emits description key:\n%s", raw)
	}
	if !strings.Contains(raw, "status: TODO") {
		t.Errorf("expected status: TODO line, got:\n%s", raw)
	}
	if !strings.Contains(raw, "Hello world.") {
		t.Errorf("expected body content, got:\n%s", raw)
	}
}

func TestCreateNote_DefaultTopic(t *testing.T) {
	v, root := newWriteTestVault(t)

	note, err := v.CreateNote(CreateOptions{
		Title:   "Fresh Topic",
		IsTopic: true,
	})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	wantPath := filepath.Join(root, "topics", "Fresh Topic.md")
	if note.FilePath != wantPath {
		t.Errorf("FilePath = %q, want %q", note.FilePath, wantPath)
	}
	if !note.IsTopic {
		t.Errorf("IsTopic = false, want true")
	}
	if !equalStringSlices(note.Frontmatter.Tags, []string{"topic"}) {
		t.Errorf("Tags = %v, want [topic]", note.Frontmatter.Tags)
	}
	if note.Frontmatter.Status != "" {
		t.Errorf("Status = %q, want empty for topics", note.Frontmatter.Status)
	}

	raw := readNoteRaw(t, note.FilePath)
	// Topics ALWAYS emit a description key (even when empty / not provided).
	if !strings.Contains(raw, "description:") {
		t.Errorf("topic should always emit description key, got:\n%s", raw)
	}
}

func TestCreateNote_TopicStatusAlwaysClearedEvenIfProvided(t *testing.T) {
	v, _ := newWriteTestVault(t)
	note, err := v.CreateNote(CreateOptions{
		Title:   "Override Topic",
		IsTopic: true,
		Status:  "IGNORED",
	})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	if note.Frontmatter.Status != "" {
		t.Errorf("topic Status = %q, want empty (must be cleared)", note.Frontmatter.Status)
	}
}

func TestCreateNote_ExplicitOverrides(t *testing.T) {
	v, _ := newWriteTestVault(t)

	note, err := v.CreateNote(CreateOptions{
		Title:       "Custom",
		HasTags:     true,
		Tags:        []string{"alpha", "beta"},
		Status:      "WIP",
		HasDesc:     true,
		Description: "A custom description.",
		Content:     "Body.",
	})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	if !equalStringSlices(note.Frontmatter.Tags, []string{"alpha", "beta"}) {
		t.Errorf("Tags = %v, want [alpha beta]", note.Frontmatter.Tags)
	}
	if note.Frontmatter.Status != "WIP" {
		t.Errorf("Status = %q, want WIP", note.Frontmatter.Status)
	}
	if note.Frontmatter.Description != "A custom description." {
		t.Errorf("Description = %q, want %q", note.Frontmatter.Description, "A custom description.")
	}
	raw := readNoteRaw(t, note.FilePath)
	if !strings.Contains(raw, "description: A custom description.") {
		t.Errorf("expected unquoted description, got:\n%s", raw)
	}
}

func TestCreateNote_HasTagsEmptySlice(t *testing.T) {
	v, _ := newWriteTestVault(t)
	note, err := v.CreateNote(CreateOptions{
		Title:   "TagLess",
		HasTags: true,
		Tags:    []string{},
	})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	if len(note.Frontmatter.Tags) != 0 {
		t.Errorf("Tags = %v, want empty", note.Frontmatter.Tags)
	}
}

func TestCreateNote_Upstream(t *testing.T) {
	v, _ := newWriteTestVault(t)
	note, err := v.CreateNote(CreateOptions{
		Title:    "Child",
		Upstream: []string{"Alpha", "Topic One"},
	})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	if !equalStringSlices(note.Frontmatter.Upstream, []string{"Alpha", "Topic One"}) {
		t.Errorf("Upstream = %v, want [Alpha, Topic One]", note.Frontmatter.Upstream)
	}
	raw := readNoteRaw(t, note.FilePath)
	if !strings.Contains(raw, `  - "[[Alpha]]"`) || !strings.Contains(raw, `  - "[[Topic One]]"`) {
		t.Errorf("expected wikilinked upstream list, got:\n%s", raw)
	}
}

func TestCreateNote_Duplicate(t *testing.T) {
	v, _ := newWriteTestVault(t)
	if _, err := v.CreateNote(CreateOptions{Title: "Alpha"}); err == nil {
		t.Fatal("expected duplicate create to fail")
	}
}

func TestCreateNote_EmptyTitle(t *testing.T) {
	v, _ := newWriteTestVault(t)
	if _, err := v.CreateNote(CreateOptions{Title: "   "}); err == nil {
		t.Fatal("expected empty title to fail")
	}
}

func TestEditNoteContent_ReplacesBody(t *testing.T) {
	v, _ := newWriteTestVault(t)
	note, err := v.EditNoteContent("Gamma", "Brand new gamma body.")
	if err != nil {
		t.Fatalf("EditNoteContent: %v", err)
	}
	if !strings.Contains(note.Content, "Brand new gamma body.") {
		t.Errorf("Content = %q, want to contain new body", note.Content)
	}
	if strings.Contains(note.Content, "Gamma body.") {
		t.Errorf("expected old body to be replaced, got: %q", note.Content)
	}
	raw := readNoteRaw(t, note.FilePath)
	if !strings.Contains(raw, "Brand new gamma body.") {
		t.Errorf("file content does not contain new body:\n%s", raw)
	}
	// Frontmatter preserved
	if note.Frontmatter.Status != "TODO" {
		t.Errorf("status lost on edit: got %q, want TODO", note.Frontmatter.Status)
	}
}

func TestEditNoteContent_NotFound(t *testing.T) {
	v, _ := newWriteTestVault(t)
	if _, err := v.EditNoteContent("Nonesuch-XYZ123", "x"); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestAppendToNote_Appends(t *testing.T) {
	v, _ := newWriteTestVault(t)
	note, err := v.AppendToNote("Gamma", "Extra line.")
	if err != nil {
		t.Fatalf("AppendToNote: %v", err)
	}
	if !strings.Contains(note.Content, "Gamma body.") {
		t.Errorf("expected original body preserved, got: %q", note.Content)
	}
	if !strings.Contains(note.Content, "Extra line.") {
		t.Errorf("expected appended content, got: %q", note.Content)
	}
	idxOrig := strings.Index(note.Content, "Gamma body.")
	idxNew := strings.Index(note.Content, "Extra line.")
	if idxOrig < 0 || idxNew < 0 || idxOrig >= idxNew {
		t.Errorf("appended content not after original; content=%q", note.Content)
	}
}

func TestEditNoteSection_ReplacesUnderHeading(t *testing.T) {
	v, _ := newWriteTestVault(t)
	note, err := v.EditNoteSection("Alpha", "Details", "New details body.")
	if err != nil {
		t.Fatalf("EditNoteSection: %v", err)
	}

	// Nested sub-heading under Details must also be replaced (## Details ...
	// up to next same-or-higher level heading, which is ## Closing).
	if strings.Contains(note.Content, "### Nested") {
		t.Errorf("nested heading should have been removed, got: %s", note.Content)
	}
	if !strings.Contains(note.Content, "New details body.") {
		t.Errorf("expected new section body, got: %s", note.Content)
	}
	// Other sections preserved
	if !strings.Contains(note.Content, "Alpha intro.") {
		t.Errorf("Intro section lost: %s", note.Content)
	}
	if !strings.Contains(note.Content, "Alpha closing.") {
		t.Errorf("Closing section lost: %s", note.Content)
	}
	// Heading itself preserved
	if !strings.Contains(note.Content, "## Details") {
		t.Errorf("Details heading dropped: %s", note.Content)
	}
}

func TestEditNoteSection_NotFound(t *testing.T) {
	v, _ := newWriteTestVault(t)
	if _, err := v.EditNoteSection("Alpha", "Definitely-Missing-Section", "x"); err == nil {
		t.Fatal("expected section-not-found error")
	}
}

func TestReplaceSection_RespectsHigherLevelBoundary(t *testing.T) {
	// Direct test of the helper to lock in the level boundary rule.
	in := "# Top\nintro\n## A\naaa\n### A1\nnested\n## B\nbbb\n# End\nend\n"
	got, err := replaceSection(in, "A", "NEW")
	if err != nil {
		t.Fatalf("replaceSection: %v", err)
	}
	// The ## A section (and its ### A1 child) must be replaced; ## B must
	// remain; # End at level 1 must remain too.
	if strings.Contains(got, "aaa") || strings.Contains(got, "### A1") || strings.Contains(got, "nested") {
		t.Errorf("expected section A content to be replaced; got:\n%s", got)
	}
	if !strings.Contains(got, "NEW") {
		t.Errorf("expected new section content; got:\n%s", got)
	}
	if !strings.Contains(got, "## B") || !strings.Contains(got, "bbb") {
		t.Errorf("expected B section preserved; got:\n%s", got)
	}
	if !strings.Contains(got, "# End") {
		t.Errorf("expected higher-level End heading preserved; got:\n%s", got)
	}
}

func TestReplaceSection_NotFound(t *testing.T) {
	_, err := replaceSection("# Top\nintro\n", "Missing-XYZ", "x")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateFrontmatter_PartialUpdatesOnlyTouchIntendedFields(t *testing.T) {
	v, _ := newWriteTestVault(t)

	// Update only status; everything else should be preserved.
	note, err := v.UpdateFrontmatter("Alpha", FrontmatterUpdate{
		HasStatus: true,
		Status:    "WIP",
	})
	if err != nil {
		t.Fatalf("UpdateFrontmatter: %v", err)
	}
	if note.Frontmatter.Status != "WIP" {
		t.Errorf("Status = %q, want WIP", note.Frontmatter.Status)
	}
	if !equalStringSlices(note.Frontmatter.Tags, []string{"notes"}) {
		t.Errorf("Tags changed unexpectedly: %v", note.Frontmatter.Tags)
	}
	if !equalStringSlices(note.Frontmatter.Upstream, []string{"Topic One"}) {
		t.Errorf("Upstream changed unexpectedly: %v", note.Frontmatter.Upstream)
	}
	if note.Frontmatter.Date != "2024-01-02" {
		t.Errorf("Date changed unexpectedly: %q", note.Frontmatter.Date)
	}
}

func TestUpdateFrontmatter_ClearViaEmpty(t *testing.T) {
	v, _ := newWriteTestVault(t)

	// Clear upstream (Alpha -> Topic One) via HasUpstream + empty slice.
	note, err := v.UpdateFrontmatter("Alpha", FrontmatterUpdate{
		HasUpstream: true,
		Upstream:    nil,
	})
	if err != nil {
		t.Fatalf("UpdateFrontmatter: %v", err)
	}
	if len(note.Frontmatter.Upstream) != 0 {
		t.Errorf("Upstream = %v, want empty", note.Frontmatter.Upstream)
	}

	// Clear tags via HasTags + empty slice.
	note2, err := v.UpdateFrontmatter("Alpha", FrontmatterUpdate{
		HasTags: true,
		Tags:    nil,
	})
	if err != nil {
		t.Fatalf("UpdateFrontmatter (tags): %v", err)
	}
	if len(note2.Frontmatter.Tags) != 0 {
		t.Errorf("Tags = %v, want empty", note2.Frontmatter.Tags)
	}

	// Clear status via HasStatus + empty.
	note3, err := v.UpdateFrontmatter("Alpha", FrontmatterUpdate{
		HasStatus: true,
		Status:    "",
	})
	if err != nil {
		t.Fatalf("UpdateFrontmatter (status): %v", err)
	}
	if note3.Frontmatter.Status != "" {
		t.Errorf("Status = %q, want empty", note3.Frontmatter.Status)
	}
}

func TestUpdateFrontmatter_ExternalLinks(t *testing.T) {
	v, _ := newWriteTestVault(t)
	note, err := v.UpdateFrontmatter("Alpha", FrontmatterUpdate{
		HasExternalLinks: true,
		ExternalLinks:    []string{"https://example.com", "https://example.org"},
	})
	if err != nil {
		t.Fatalf("UpdateFrontmatter: %v", err)
	}
	if !equalStringSlices(note.Frontmatter.ExternalLinks, []string{"https://example.com", "https://example.org"}) {
		t.Errorf("ExternalLinks = %v", note.Frontmatter.ExternalLinks)
	}

	// Now clear them.
	note2, err := v.UpdateFrontmatter("Alpha", FrontmatterUpdate{
		HasExternalLinks: true,
		ExternalLinks:    nil,
	})
	if err != nil {
		t.Fatalf("UpdateFrontmatter clear: %v", err)
	}
	if len(note2.Frontmatter.ExternalLinks) != 0 {
		t.Errorf("ExternalLinks after clear = %v", note2.Frontmatter.ExternalLinks)
	}
}

func TestUpdateFrontmatter_NotFound(t *testing.T) {
	v, _ := newWriteTestVault(t)
	if _, err := v.UpdateFrontmatter("Nonesuch-XYZ123", FrontmatterUpdate{}); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestDeleteNote_RemovesFileAndReportsAffected(t *testing.T) {
	v, _ := newWriteTestVault(t)

	// Alpha is referenced as upstream by Beta.
	alphaPath := v.notes["Alpha"].FilePath
	res, err := v.DeleteNote("Alpha")
	if err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	if res.Deleted != "Alpha" {
		t.Errorf("Deleted = %q, want Alpha", res.Deleted)
	}
	if !equalStringSlices(res.AffectedNotes, []string{"Beta"}) {
		t.Errorf("AffectedNotes = %v, want [Beta]", res.AffectedNotes)
	}
	if _, err := os.Stat(alphaPath); !os.IsNotExist(err) {
		t.Errorf("file still exists after delete: err=%v", err)
	}
	if _, ok := v.notes["Alpha"]; ok {
		t.Errorf("Alpha still in index after delete")
	}
}

func TestDeleteNote_NotFound(t *testing.T) {
	v, _ := newWriteTestVault(t)
	if _, err := v.DeleteNote("Nonesuch-XYZ123"); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestRenameNote_RenamesAndCascadesUpstreamAndWikilinks(t *testing.T) {
	v, _ := newWriteTestVault(t)

	res, err := v.RenameNote("Alpha", "AlphaPrime")
	if err != nil {
		t.Fatalf("RenameNote: %v", err)
	}
	if res.Renamed != "AlphaPrime" {
		t.Errorf("Renamed = %q, want AlphaPrime", res.Renamed)
	}

	// Beta should appear in UpdatedRefs (both because of upstream and inline
	// wikilink); it should be the only updated ref here.
	sort.Strings(res.UpdatedRefs)
	if !equalStringSlices(res.UpdatedRefs, []string{"Beta"}) {
		t.Errorf("UpdatedRefs = %v, want [Beta]", res.UpdatedRefs)
	}

	// Index reflects the rename.
	if _, ok := v.notes["Alpha"]; ok {
		t.Errorf("old title still in index")
	}
	newNote := v.notes["AlphaPrime"]
	if newNote == nil {
		t.Fatalf("new title missing from index")
	}
	if !strings.HasSuffix(newNote.FilePath, "AlphaPrime.md") {
		t.Errorf("FilePath = %q, want ...AlphaPrime.md", newNote.FilePath)
	}

	// Beta's upstream now points at AlphaPrime.
	beta := v.notes["Beta"]
	if beta == nil {
		t.Fatalf("Beta missing")
	}
	if !equalStringSlices(beta.Frontmatter.Upstream, []string{"AlphaPrime"}) {
		t.Errorf("Beta upstream = %v, want [AlphaPrime]", beta.Frontmatter.Upstream)
	}

	// Beta's body: both plain and aliased wikilinks must have been rewritten.
	betaRaw := readNoteRaw(t, beta.FilePath)
	if strings.Contains(betaRaw, "[[Alpha]]") {
		t.Errorf("plain [[Alpha]] not rewritten:\n%s", betaRaw)
	}
	if strings.Contains(betaRaw, "[[Alpha|") {
		t.Errorf("aliased [[Alpha|...]] not rewritten:\n%s", betaRaw)
	}
	if !strings.Contains(betaRaw, "[[AlphaPrime]]") {
		t.Errorf("expected [[AlphaPrime]] in body:\n%s", betaRaw)
	}
	if !strings.Contains(betaRaw, "[[AlphaPrime|alpha-display]]") {
		t.Errorf("expected [[AlphaPrime|alpha-display]] in body:\n%s", betaRaw)
	}
}

func TestRenameNote_TargetExists(t *testing.T) {
	v, _ := newWriteTestVault(t)
	if _, err := v.RenameNote("Alpha", "Beta"); err == nil {
		t.Fatal("expected error when target title exists")
	}
}

func TestRenameNote_NotFound(t *testing.T) {
	v, _ := newWriteTestVault(t)
	if _, err := v.RenameNote("Nonesuch-XYZ123", "Anything"); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestRenameNote_EmptyNewTitle(t *testing.T) {
	v, _ := newWriteTestVault(t)
	if _, err := v.RenameNote("Alpha", "   "); err == nil {
		t.Fatal("expected error on empty new title")
	}
}

func TestRenameNote_NoReferencesEmptyUpdatedRefs(t *testing.T) {
	v, _ := newWriteTestVault(t)
	// Gamma is not referenced by anyone, so UpdatedRefs should be empty.
	res, err := v.RenameNote("Gamma", "GammaPrime")
	if err != nil {
		t.Fatalf("RenameNote: %v", err)
	}
	if len(res.UpdatedRefs) != 0 {
		t.Errorf("UpdatedRefs = %v, want empty", res.UpdatedRefs)
	}
}

// equalStringSlices: helper used across tests for order-sensitive equality.
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

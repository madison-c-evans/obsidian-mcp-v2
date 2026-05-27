package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeNote(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func newTestVault(t *testing.T) *Vault {
	t.Helper()
	dir := t.TempDir()

	writeNote(t, dir, "Postgres Indexing.md", `---
tags: [notes]
status: sprout
description: Reference for picking the right index type when query latency spikes.
---

# Postgres Indexing

We rely on B-tree indexes for the majority of equality and range filters
in pgdb. The composite index on (tenant_id, created_at) is the workhorse
behind tenant-scoped listings.

For hot keysets a partial index can carve out the active subset.
`)

	writeNote(t, dir, "Frontend Routing.md", `---
tags: [notes]
status: TODO
---

Next.js app router handles all SSR routes. We avoid client-side routing
hacks because they bypass middleware auth checks.
`)

	writeNote(t, dir, "Long Wall.md", `---
tags: [notes]
---

`+strings.Repeat("filler text that is not relevant. ", 40)+`The composite index discussion lives here too with enough surrounding context to force trimming.`+strings.Repeat(" more filler after the match.", 40)+`
`)

	v, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := v.BuildIndex(); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	return v
}

func TestSearchExcerpt_ContentMatchIncludesQuery(t *testing.T) {
	v := newTestVault(t)
	results := v.Search("composite index", SearchOptions{Scope: "content", MaxResults: 5})
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}

	var hit *SearchResult
	for i := range results {
		if results[i].Note.Title == "Postgres Indexing" {
			hit = &results[i]
			break
		}
	}
	if hit == nil {
		t.Fatalf("Postgres Indexing not in results: %+v", results)
	}
	if hit.Excerpt == "" {
		t.Fatal("expected non-empty Excerpt on content match")
	}
	if !strings.Contains(strings.ToLower(hit.Excerpt), "composite index") {
		t.Errorf("excerpt missing matched phrase: %q", hit.Excerpt)
	}
	if !strings.Contains(hit.Excerpt, "**") {
		t.Errorf("expected highlight markers in excerpt: %q", hit.Excerpt)
	}
}

func TestSearchExcerpt_DescriptionMatch(t *testing.T) {
	v := newTestVault(t)
	results := v.Search("query latency", SearchOptions{Scope: "all", MaxResults: 5})
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	var hit *SearchResult
	for i := range results {
		if results[i].Note.Title == "Postgres Indexing" {
			hit = &results[i]
			break
		}
	}
	if hit == nil {
		t.Fatalf("Postgres Indexing not in results: %+v", results)
	}
	if hit.MatchType != "description" {
		t.Fatalf("expected description match, got %q", hit.MatchType)
	}
	if !strings.Contains(strings.ToLower(hit.Excerpt), "query latency") {
		t.Errorf("description excerpt missing matched phrase: %q", hit.Excerpt)
	}
}

func TestSearchExcerpt_TitleOnlyMatchIsEmpty(t *testing.T) {
	v := newTestVault(t)
	results := v.Search("Frontend Routing", SearchOptions{Scope: "title", MaxResults: 5})
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].Excerpt != "" {
		t.Errorf("expected empty excerpt for title-only match, got %q", results[0].Excerpt)
	}
}

func TestSearchExcerpt_BoundedSize(t *testing.T) {
	v := newTestVault(t)
	results := v.Search("composite index", SearchOptions{Scope: "content", MaxResults: 10})
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	var hit *SearchResult
	for i := range results {
		if results[i].Note.Title == "Long Wall" {
			hit = &results[i]
			break
		}
	}
	if hit == nil {
		t.Fatalf("Long Wall not in results: %+v", results)
	}
	// Cap is excerptMaxChars (240) for the trimmed window; allow generous
	// slack for highlight markers (**…**) and ellipsis bookends.
	if got := len(hit.Excerpt); got > excerptMaxChars+32 {
		t.Errorf("excerpt too long: %d chars (cap %d): %q", got, excerptMaxChars, hit.Excerpt)
	}
	if strings.Count(hit.Excerpt, "\n") >= excerptMaxLines {
		t.Errorf("excerpt has too many lines: %q", hit.Excerpt)
	}
	if !strings.Contains(strings.ToLower(hit.Excerpt), "composite index") {
		t.Errorf("trimmed excerpt lost matched phrase: %q", hit.Excerpt)
	}
}

func TestSearchExcerpt_NoMatchReturnsEmpty(t *testing.T) {
	v := newTestVault(t)
	results := v.Search("nonexistent-zzz-token", SearchOptions{Scope: "all", MaxResults: 5})
	for _, r := range results {
		if r.Excerpt != "" {
			t.Errorf("expected empty excerpt for non-matching query, got %q on %s", r.Excerpt, r.Note.Title)
		}
	}
}

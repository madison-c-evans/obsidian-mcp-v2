package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// CreateNote writes a new note (or topic) to disk and refreshes the index.
func (v *Vault) CreateNote(opts CreateOptions) (*Note, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	title := strings.TrimSpace(opts.Title)
	if title == "" {
		return nil, errors.New("title is required")
	}

	fileName := title + ".md"
	var filePath string
	if opts.IsTopic {
		filePath = filepath.Join(v.vaultPath, "topics", fileName)
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			return nil, err
		}
	} else {
		filePath = filepath.Join(v.vaultPath, fileName)
	}

	if _, err := os.Stat(filePath); err == nil {
		return nil, fmt.Errorf("note already exists: %s", title)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	tags := opts.Tags
	if !opts.HasTags {
		if opts.IsTopic {
			tags = []string{"topic"}
		} else {
			tags = []string{"notes"}
		}
	}

	status := opts.Status
	if status == "" && !opts.IsTopic {
		status = "TODO"
	}
	if opts.IsTopic {
		status = ""
	}

	data := map[string]any{
		"tags":           tags,
		"external-links": nil,
		"upstream":       upstreamToRaw(opts.Upstream),
		"date":           time.Now().Format("2006-01-02"),
		"status":         status,
	}

	// description: topics always get the key (empty by default); notes only
	// when explicitly provided.
	if opts.IsTopic {
		if opts.HasDesc {
			data["description"] = opts.Description
		} else {
			data["description"] = ""
		}
	} else if opts.HasDesc {
		data["description"] = opts.Description
	}

	body := opts.Content
	if body != "" {
		body = "\n" + body
	}
	fileContent := serializeNote(data, body)
	if err := os.WriteFile(filePath, []byte(fileContent), 0o644); err != nil {
		return nil, err
	}

	note, err := v.parseNoteFile(filePath)
	if err != nil {
		return nil, err
	}
	v.notes[note.Title] = note
	v.buildGraphEdges()
	v.buildMentionEdges()
	return note, nil
}

func (v *Vault) EditNoteContent(title, newContent string) (*Note, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.rewriteBody(title, func(_ string) string {
		return "\n" + newContent
	})
}

func (v *Vault) AppendToNote(title, extra string) (*Note, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.rewriteBody(title, func(existing string) string {
		return strings.TrimRight(existing, "\n\t ") + "\n\n" + extra
	})
}

func (v *Vault) EditNoteSection(title, heading, newSectionContent string) (*Note, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	note := v.resolveNoteLocked(title)
	if note == nil {
		return nil, fmt.Errorf("note not found: %s", title)
	}
	raw, err := os.ReadFile(note.FilePath)
	if err != nil {
		return nil, err
	}
	yamlBlock, body := splitFrontmatter(string(raw))

	newBody, err := replaceSection(body, heading, newSectionContent)
	if err != nil {
		return nil, fmt.Errorf(`%w in note %q`, err, note.Title)
	}

	data := map[string]any{}
	if yamlBlock != "" {
		_ = yaml.Unmarshal([]byte(yamlBlock), &data)
	}
	fileContent := serializeNote(data, newBody)
	if err := os.WriteFile(note.FilePath, []byte(fileContent), 0o644); err != nil {
		return nil, err
	}

	updated, err := v.parseNoteFile(note.FilePath)
	if err != nil {
		return nil, err
	}
	v.notes[updated.Title] = updated
	v.buildGraphEdges()
	v.buildMentionEdges()
	return updated, nil
}

// rewriteBody is the shared write path for body-only edits.
func (v *Vault) rewriteBody(title string, transform func(existing string) string) (*Note, error) {
	note := v.resolveNoteLocked(title)
	if note == nil {
		return nil, fmt.Errorf("note not found: %s", title)
	}

	raw, err := os.ReadFile(note.FilePath)
	if err != nil {
		return nil, err
	}
	yamlBlock, body := splitFrontmatter(string(raw))
	data := map[string]any{}
	if yamlBlock != "" {
		_ = yaml.Unmarshal([]byte(yamlBlock), &data)
	}

	newBody := transform(body)
	fileContent := serializeNote(data, newBody)
	if err := os.WriteFile(note.FilePath, []byte(fileContent), 0o644); err != nil {
		return nil, err
	}

	updated, err := v.parseNoteFile(note.FilePath)
	if err != nil {
		return nil, err
	}
	v.notes[updated.Title] = updated
	v.buildGraphEdges()
	v.buildMentionEdges()
	return updated, nil
}

var headingRe = regexp.MustCompile(`^(#{1,6})\s`)

func replaceSection(content, heading, newSectionContent string) (string, error) {
	lines := strings.Split(content, "\n")
	target := strings.ToLower(heading)

	headingLevel := func(line string) int {
		m := headingRe.FindStringSubmatch(line)
		if m == nil {
			return 0
		}
		return len(m[1])
	}

	sectionStart := -1
	sectionLevel := 0
	for i, line := range lines {
		lvl := headingLevel(line)
		if lvl == 0 {
			continue
		}
		text := strings.TrimSpace(strings.TrimLeft(line, "# "))
		text = strings.ToLower(text)
		if text == target || strings.Contains(text, target) {
			sectionStart = i
			sectionLevel = lvl
			break
		}
	}
	if sectionStart == -1 {
		return "", fmt.Errorf("section not found: %q", heading)
	}

	sectionEnd := len(lines)
	for i := sectionStart + 1; i < len(lines); i++ {
		lvl := headingLevel(lines[i])
		if lvl > 0 && lvl <= sectionLevel {
			sectionEnd = i
			break
		}
	}

	newLines := make([]string, 0, len(lines)+4)
	newLines = append(newLines, lines[:sectionStart]...)
	newLines = append(newLines, lines[sectionStart], "", newSectionContent, "")
	newLines = append(newLines, lines[sectionEnd:]...)
	return strings.Join(newLines, "\n"), nil
}

func (v *Vault) UpdateFrontmatter(title string, updates FrontmatterUpdate) (*Note, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.applyFrontmatterUpdate(title, updates)
}

// applyFrontmatterUpdate runs under the caller's write lock.
func (v *Vault) applyFrontmatterUpdate(title string, updates FrontmatterUpdate) (*Note, error) {
	note := v.resolveNoteLocked(title)
	if note == nil {
		return nil, fmt.Errorf("note not found: %s", title)
	}

	raw, err := os.ReadFile(note.FilePath)
	if err != nil {
		return nil, err
	}
	yamlBlock, body := splitFrontmatter(string(raw))
	data := map[string]any{}
	if yamlBlock != "" {
		_ = yaml.Unmarshal([]byte(yamlBlock), &data)
	}

	if updates.HasTags {
		data["tags"] = updates.Tags
	}
	if updates.HasExternalLinks {
		if len(updates.ExternalLinks) > 0 {
			data["external-links"] = updates.ExternalLinks
		} else {
			data["external-links"] = nil
		}
	}
	if updates.HasUpstream {
		data["upstream"] = upstreamToRaw(updates.Upstream)
	}
	if updates.HasStatus {
		data["status"] = updates.Status
	}
	if updates.HasDesc {
		data["description"] = updates.Description
	}

	fileContent := serializeNote(data, body)
	if err := os.WriteFile(note.FilePath, []byte(fileContent), 0o644); err != nil {
		return nil, err
	}

	updated, err := v.parseNoteFile(note.FilePath)
	if err != nil {
		return nil, err
	}
	v.notes[updated.Title] = updated
	v.buildGraphEdges()
	return updated, nil
}

type DeleteResult struct {
	Deleted       string
	AffectedNotes []string
}

func (v *Vault) DeleteNote(title string) (*DeleteResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	note := v.resolveNoteLocked(title)
	if note == nil {
		return nil, fmt.Errorf("note not found: %s", title)
	}

	var affected []string
	for otherTitle, other := range v.notes {
		if containsString(other.Frontmatter.Upstream, note.Title) {
			affected = append(affected, otherTitle)
		}
	}

	if err := os.Remove(note.FilePath); err != nil {
		return nil, err
	}
	delete(v.notes, note.Title)
	v.buildGraphEdges()
	v.buildMentionEdges()

	return &DeleteResult{Deleted: note.Title, AffectedNotes: affected}, nil
}

type RenameResult struct {
	Renamed     string
	UpdatedRefs []string
}

func (v *Vault) RenameNote(oldTitle, newTitle string) (*RenameResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	newTitle = strings.TrimSpace(newTitle)
	if newTitle == "" {
		return nil, errors.New("new title is required")
	}

	note := v.resolveNoteLocked(oldTitle)
	if note == nil {
		return nil, fmt.Errorf("note not found: %s", oldTitle)
	}
	if _, exists := v.notes[newTitle]; exists {
		return nil, fmt.Errorf("a note with title %q already exists", newTitle)
	}

	dir := filepath.Dir(note.FilePath)
	newFilePath := filepath.Join(dir, newTitle+".md")
	if err := os.Rename(note.FilePath, newFilePath); err != nil {
		return nil, err
	}
	delete(v.notes, note.Title)
	updated, err := v.parseNoteFile(newFilePath)
	if err != nil {
		return nil, err
	}
	v.notes[updated.Title] = updated

	// Snapshot titles to iterate without mutating during the loop.
	type pair struct {
		title string
		note  *Note
	}
	snapshot := func() []pair {
		out := make([]pair, 0, len(v.notes))
		for t, n := range v.notes {
			out = append(out, pair{t, n})
		}
		return out
	}

	updatedSet := make(map[string]struct{})
	for _, p := range snapshot() {
		if containsString(p.note.Frontmatter.Upstream, note.Title) {
			newUpstream := make([]string, len(p.note.Frontmatter.Upstream))
			for i, u := range p.note.Frontmatter.Upstream {
				if u == note.Title {
					newUpstream[i] = newTitle
				} else {
					newUpstream[i] = u
				}
			}
			if _, err := v.applyFrontmatterUpdate(p.title, FrontmatterUpdate{
				HasUpstream: true,
				Upstream:    newUpstream,
			}); err != nil {
				return nil, err
			}
			updatedSet[p.title] = struct{}{}
		}
	}

	// Cascade inline [[wikilinks]] in body content across the vault.
	escapedOld := regexp.QuoteMeta(note.Title)
	wikiRe := regexp.MustCompile(`\[\[` + escapedOld + `(\|[^\]]*)?\]\]`)

	for _, p := range snapshot() {
		raw, err := os.ReadFile(p.note.FilePath)
		if err != nil {
			return nil, err
		}
		yamlBlock, body := splitFrontmatter(string(raw))
		if !wikiRe.MatchString(body) {
			continue
		}
		newBody := wikiRe.ReplaceAllStringFunc(body, func(match string) string {
			sub := wikiRe.FindStringSubmatch(match)
			display := ""
			if len(sub) > 1 {
				display = sub[1]
			}
			return "[[" + newTitle + display + "]]"
		})
		if newBody == body {
			continue
		}
		data := map[string]any{}
		if yamlBlock != "" {
			_ = yaml.Unmarshal([]byte(yamlBlock), &data)
		}
		fileContent := serializeNote(data, newBody)
		if err := os.WriteFile(p.note.FilePath, []byte(fileContent), 0o644); err != nil {
			return nil, err
		}
		reparsed, err := v.parseNoteFile(p.note.FilePath)
		if err != nil {
			return nil, err
		}
		v.notes[reparsed.Title] = reparsed
		updatedSet[p.title] = struct{}{}
	}

	v.buildGraphEdges()
	v.buildMentionEdges()

	updatedRefs := make([]string, 0, len(updatedSet))
	for t := range updatedSet {
		updatedRefs = append(updatedRefs, t)
	}
	return &RenameResult{Renamed: newTitle, UpdatedRefs: updatedRefs}, nil
}

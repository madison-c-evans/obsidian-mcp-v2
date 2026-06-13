package vault

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func writeNote(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}

func buildFixtureVault(t *testing.T) *Vault {
	t.Helper()
	dir := t.TempDir()

	writeNote(t, dir, "topics/Engineering.md", `---
tags: [topic]
upstream: []
status:
description: Engineering work
---
# Engineering
`)

	writeNote(t, dir, "Project Alpha.md", `---
tags: [notes, work]
upstream: ["[[Engineering]]"]
status: TODO
---
# Alpha

- [ ] design the API
- [x] sketch the diagram
- [X] file the ticket
not a task line
- [ ]   wire up handler
`)

	writeNote(t, dir, "Personal Errands.md", `---
tags: [notes, personal]
upstream: []
status: TODO
---
- [ ] buy groceries
- [x] take out trash
`)

	writeNote(t, dir, "Empty.md", `---
tags: [notes]
upstream: []
status: TODO
---
nothing to do here
`)

	writeNote(t, dir, "templates/Daily.md", `---
tags: [template]
upstream: []
status:
---
- [ ] template task that should be ignored
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

func taskTexts(tasks []Task) []string {
	out := make([]string, len(tasks))
	for i, t := range tasks {
		out[i] = t.Text
	}
	sort.Strings(out)
	return out
}

func TestListTasks_DefaultStatusIsOpen(t *testing.T) {
	v := buildFixtureVault(t)
	tasks := v.ListTasks(TaskFilters{})

	got := taskTexts(tasks)
	want := []string{"buy groceries", "design the API", "wire up handler"}
	sort.Strings(want)
	if !equalSlices(got, want) {
		t.Fatalf("default status: got %v, want %v", got, want)
	}
	for _, ts := range tasks {
		if ts.Done {
			t.Errorf("default filter returned done task: %+v", ts)
		}
	}
}

func TestListTasks_StatusDone(t *testing.T) {
	v := buildFixtureVault(t)
	tasks := v.ListTasks(TaskFilters{Status: "done"})

	got := taskTexts(tasks)
	want := []string{"file the ticket", "sketch the diagram", "take out trash"}
	if !equalSlices(got, want) {
		t.Fatalf("done status: got %v, want %v", got, want)
	}
	for _, ts := range tasks {
		if !ts.Done {
			t.Errorf("done filter returned open task: %+v", ts)
		}
	}
}

func TestListTasks_StatusAll(t *testing.T) {
	v := buildFixtureVault(t)
	tasks := v.ListTasks(TaskFilters{Status: "all"})

	got := taskTexts(tasks)
	want := []string{
		"buy groceries",
		"design the API",
		"file the ticket",
		"sketch the diagram",
		"take out trash",
		"wire up handler",
	}
	if !equalSlices(got, want) {
		t.Fatalf("all status: got %v, want %v", got, want)
	}
}

func TestListTasks_FiltersOutTemplates(t *testing.T) {
	v := buildFixtureVault(t)
	tasks := v.ListTasks(TaskFilters{Status: "all"})
	for _, ts := range tasks {
		if ts.Text == "template task that should be ignored" {
			t.Fatalf("template task leaked through: %+v", ts)
		}
	}
}

func TestListTasks_TopicFilter(t *testing.T) {
	v := buildFixtureVault(t)
	tasks := v.ListTasks(TaskFilters{Status: "all", Topic: "Engineering"})

	got := taskTexts(tasks)
	want := []string{"design the API", "file the ticket", "sketch the diagram", "wire up handler"}
	if !equalSlices(got, want) {
		t.Fatalf("topic filter: got %v, want %v", got, want)
	}
	for _, ts := range tasks {
		if ts.NoteTitle != "Project Alpha" {
			t.Errorf("topic filter included non-engineering note: %+v", ts)
		}
	}
}

func TestListTasks_TagFilter(t *testing.T) {
	v := buildFixtureVault(t)
	tasks := v.ListTasks(TaskFilters{Status: "all", Tag: "personal"})

	got := taskTexts(tasks)
	want := []string{"buy groceries", "take out trash"}
	if !equalSlices(got, want) {
		t.Fatalf("tag filter: got %v, want %v", got, want)
	}
	for _, ts := range tasks {
		if ts.NoteTitle != "Personal Errands" {
			t.Errorf("tag filter included wrong note: %+v", ts)
		}
	}
}

func TestListTasks_CapturesNoteMetadata(t *testing.T) {
	v := buildFixtureVault(t)
	tasks := v.ListTasks(TaskFilters{Status: "all"})

	byText := map[string]Task{}
	for _, ts := range tasks {
		byText[ts.Text] = ts
	}
	alpha, ok := byText["design the API"]
	if !ok {
		t.Fatalf("missing 'design the API' task")
	}
	if alpha.NoteTitle != "Project Alpha" {
		t.Errorf("NoteTitle: got %q, want %q", alpha.NoteTitle, "Project Alpha")
	}
	if alpha.NotePath != "Project Alpha.md" {
		t.Errorf("NotePath: got %q, want %q", alpha.NotePath, "Project Alpha.md")
	}
	if alpha.Done {
		t.Errorf("expected open, got done")
	}

	xCase, ok := byText["file the ticket"]
	if !ok {
		t.Fatalf("missing 'file the ticket' task (capital X variant)")
	}
	if !xCase.Done {
		t.Errorf("expected '- [X]' to be parsed as done")
	}
}

func equalSlices(a, b []string) bool {
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

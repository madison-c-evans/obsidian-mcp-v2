package vault

import (
	"regexp"
	"sort"
	"strings"
)

type Task struct {
	Text      string
	Done      bool
	NoteTitle string
	NotePath  string
}

type TaskFilters struct {
	Status string
	Topic  string
	Tag    string
}

var taskLineRe = regexp.MustCompile(`(?m)^[ \t]*-[ \t]+\[([ xX])\][ \t]+(.+?)[ \t\r]*$`)

func (v *Vault) ListTasks(filters TaskFilters) []Task {
	v.mu.RLock()
	defer v.mu.RUnlock()

	status := strings.ToLower(strings.TrimSpace(filters.Status))
	if status == "" {
		status = "open"
	}

	titles := make([]string, 0, len(v.notes))
	for t := range v.notes {
		titles = append(titles, t)
	}
	sort.Strings(titles)

	var tasks []Task
	for _, title := range titles {
		note := v.notes[title]
		if isTemplatePath(note.RelativePath) {
			continue
		}
		if filters.Topic != "" && !v.isUnderTopicLocked(note.Title, filters.Topic) {
			continue
		}
		if filters.Tag != "" && !containsString(note.Frontmatter.Tags, filters.Tag) {
			continue
		}

		for _, m := range taskLineRe.FindAllStringSubmatch(note.Content, -1) {
			done := m[1] == "x" || m[1] == "X"
			if status == "open" && done {
				continue
			}
			if status == "done" && !done {
				continue
			}
			text := strings.TrimSpace(m[2])
			if text == "" {
				continue
			}
			tasks = append(tasks, Task{
				Text:      text,
				Done:      done,
				NoteTitle: note.Title,
				NotePath:  note.RelativePath,
			})
		}
	}
	return tasks
}

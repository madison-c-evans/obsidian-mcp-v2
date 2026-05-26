package vault

import (
	"sort"
	"time"
)

func (v *Vault) GetHealth() HealthReport {
	v.mu.RLock()
	defer v.mu.RUnlock()

	var report HealthReport
	today := time.Now()

	titles := make([]string, 0, len(v.notes))
	for t := range v.notes {
		titles = append(titles, t)
	}
	sort.Strings(titles)

	for _, title := range titles {
		note := v.notes[title]
		if isTemplatePath(note.RelativePath) {
			continue
		}

		for _, ref := range note.Frontmatter.Upstream {
			if _, ok := v.notes[ref]; !ok {
				report.BrokenUpstream = append(report.BrokenUpstream, BrokenRef{Note: title, BrokenRef: ref})
			}
		}

		if !note.IsTopic && len(note.Frontmatter.Upstream) == 0 {
			if _, hasChildren := v.children[title]; !hasChildren {
				report.Orphans = append(report.Orphans, title)
			}
		}

		if note.Frontmatter.Status == "TODO" && note.Frontmatter.Date != "" {
			noteDate, err := time.Parse("2006-01-02", note.Frontmatter.Date)
			if err == nil {
				daysOld := int(today.Sub(noteDate).Hours() / 24)
				if daysOld > 7 {
					report.StaleTodos = append(report.StaleTodos, formatStaleTodo(title, daysOld))
				}
			}
		}

		if !note.IsTopic && note.Content == "" {
			report.EmptyNotes = append(report.EmptyNotes, title)
		}
	}
	return report
}

func formatStaleTodo(title string, daysOld int) string {
	return title + " (" + itoa(daysOld) + " days old)"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}

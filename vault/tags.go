package vault

import "sort"

// buildTagIndex aggregates frontmatter tags into a tag -> note titles map.
// Templates are excluded to match Search/ListNotes behavior.
func (v *Vault) buildTagIndex() {
	v.tagIndex = make(map[string]map[string]struct{})
	for title, note := range v.notes {
		if isTemplatePath(note.RelativePath) {
			continue
		}
		for _, tag := range note.Frontmatter.Tags {
			if tag == "" {
				continue
			}
			set, ok := v.tagIndex[tag]
			if !ok {
				set = make(map[string]struct{})
				v.tagIndex[tag] = set
			}
			set[title] = struct{}{}
		}
	}
}

// ListTags returns every tag with the number of notes carrying it, sorted by
// count descending then tag ascending. If topic is non-empty, only notes whose
// upstream chain reaches a topic matching that query are counted.
func (v *Vault) ListTags(topic string) []TagCount {
	v.mu.RLock()
	defer v.mu.RUnlock()

	counts := make(map[string]int, len(v.tagIndex))
	for tag, titles := range v.tagIndex {
		if topic == "" {
			counts[tag] = len(titles)
			continue
		}
		n := 0
		for title := range titles {
			if v.isUnderTopicLocked(title, topic) {
				n++
			}
		}
		if n > 0 {
			counts[tag] = n
		}
	}

	out := make([]TagCount, 0, len(counts))
	for tag, c := range counts {
		out = append(out, TagCount{Tag: tag, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	return out
}

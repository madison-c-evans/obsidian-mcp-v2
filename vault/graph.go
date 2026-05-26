package vault

import "sort"

func (v *Vault) GetAncestors(title string, maxDepth int) []*Note {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.getAncestorsLocked(title, maxDepth)
}

func (v *Vault) getAncestorsLocked(title string, maxDepth int) []*Note {
	if maxDepth <= 0 {
		maxDepth = 3
	}
	visited := make(map[string]struct{})
	var result []*Note
	var walk func(current string, depth int)
	walk = func(current string, depth int) {
		if depth > maxDepth {
			return
		}
		if _, seen := visited[current]; seen {
			return
		}
		visited[current] = struct{}{}
		note, ok := v.notes[current]
		if !ok {
			return
		}
		for _, parentTitle := range note.Frontmatter.Upstream {
			if _, seen := visited[parentTitle]; seen {
				continue
			}
			if parent, ok := v.notes[parentTitle]; ok {
				result = append(result, parent)
				walk(parentTitle, depth+1)
			}
		}
	}
	walk(title, 0)
	return result
}

func (v *Vault) GetDescendants(title string, maxDepth int) []*Note {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.getDescendantsLocked(title, maxDepth)
}

func (v *Vault) getDescendantsLocked(title string, maxDepth int) []*Note {
	if maxDepth <= 0 {
		maxDepth = 3
	}
	visited := make(map[string]struct{})
	var result []*Note
	var walk func(current string, depth int)
	walk = func(current string, depth int) {
		if depth > maxDepth {
			return
		}
		if _, seen := visited[current]; seen {
			return
		}
		visited[current] = struct{}{}
		children, ok := v.children[current]
		if !ok {
			return
		}
		titles := make([]string, 0, len(children))
		for t := range children {
			titles = append(titles, t)
		}
		sort.Strings(titles)
		for _, childTitle := range titles {
			if _, seen := visited[childTitle]; seen {
				continue
			}
			if child, ok := v.notes[childTitle]; ok {
				result = append(result, child)
				walk(childTitle, depth+1)
			}
		}
	}
	walk(title, 0)
	return result
}

func (v *Vault) GetSiblings(title string) []*Note {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.getSiblingsLocked(title)
}

func (v *Vault) getSiblingsLocked(title string) []*Note {
	note, ok := v.notes[title]
	if !ok {
		return nil
	}
	seen := map[string]struct{}{title: {}}
	var siblings []*Note
	for _, parentTitle := range note.Frontmatter.Upstream {
		children, ok := v.children[parentTitle]
		if !ok {
			continue
		}
		titles := make([]string, 0, len(children))
		for t := range children {
			titles = append(titles, t)
		}
		sort.Strings(titles)
		for _, sibTitle := range titles {
			if _, exists := seen[sibTitle]; exists {
				continue
			}
			seen[sibTitle] = struct{}{}
			if sib, ok := v.notes[sibTitle]; ok {
				siblings = append(siblings, sib)
			}
		}
	}
	return siblings
}

func (v *Vault) GetMentions(title string) []*Note {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.getMentionsLocked(title)
}

func (v *Vault) getMentionsLocked(title string) []*Note {
	targets, ok := v.mentions[title]
	if !ok {
		return nil
	}
	titles := make([]string, 0, len(targets))
	for t := range targets {
		titles = append(titles, t)
	}
	sort.Strings(titles)
	out := make([]*Note, 0, len(titles))
	for _, t := range titles {
		if n, ok := v.notes[t]; ok {
			out = append(out, n)
		}
	}
	return out
}

func (v *Vault) GetMentionedBy(title string) []*Note {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.getMentionedByLocked(title)
}

func (v *Vault) getMentionedByLocked(title string) []*Note {
	sources, ok := v.mentionedBy[title]
	if !ok {
		return nil
	}
	titles := make([]string, 0, len(sources))
	for t := range sources {
		titles = append(titles, t)
	}
	sort.Strings(titles)
	out := make([]*Note, 0, len(titles))
	for _, t := range titles {
		if n, ok := v.notes[t]; ok {
			out = append(out, n)
		}
	}
	return out
}

func (v *Vault) GetGraphContext(title string, depth int) GraphContext {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.getGraphContextLocked(title, depth)
}

func (v *Vault) getGraphContextLocked(title string, depth int) GraphContext {
	if depth <= 0 {
		depth = 1
	}
	return GraphContext{
		Ancestors:   v.getAncestorsLocked(title, depth),
		Descendants: v.getDescendantsLocked(title, depth),
		Siblings:    v.getSiblingsLocked(title),
		Mentions:    v.getMentionsLocked(title),
		MentionedBy: v.getMentionedByLocked(title),
	}
}

// ── Listing ─────────────────────────────────────────────────────────────────

func (v *Vault) ListTopics() []*Note {
	v.mu.RLock()
	defer v.mu.RUnlock()
	var out []*Note
	for _, n := range v.notes {
		if n.IsTopic {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out
}

type ListFilters struct {
	Topic  string
	Status string
	Tag    string
}

func (v *Vault) ListNotes(filters ListFilters) []*Note {
	v.mu.RLock()
	defer v.mu.RUnlock()
	var out []*Note
	for _, n := range v.notes {
		if isTemplatePath(n.RelativePath) {
			continue
		}
		if filters.Topic != "" && !v.isUnderTopicLocked(n.Title, filters.Topic) {
			continue
		}
		if filters.Status != "" && n.Frontmatter.Status != filters.Status {
			continue
		}
		if filters.Tag != "" && !containsString(n.Frontmatter.Tags, filters.Tag) {
			continue
		}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out
}

func isTemplatePath(rel string) bool {
	return rel == "templates" ||
		len(rel) >= len("templates/") && (rel[:len("templates/")] == "templates/" || rel[:len("templates\\")] == "templates\\")
}

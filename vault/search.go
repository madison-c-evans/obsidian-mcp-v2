package vault

import (
	"math"
	"regexp"
	"sort"
	"strings"
)

func (v *Vault) ResolveNote(query string) *Note {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.resolveNoteLocked(query)
}

func (v *Vault) resolveNoteLocked(query string) *Note {
	if n, ok := v.notes[query]; ok {
		return n
	}
	lower := strings.ToLower(query)
	for title, n := range v.notes {
		if strings.ToLower(title) == lower {
			return n
		}
	}
	var best *Note
	bestScore := 0.3
	for title, n := range v.notes {
		score := scoreTitleMatch(query, title)
		if score > bestScore {
			bestScore = score
			best = n
		}
	}
	return best
}

func (v *Vault) Search(query string, opts SearchOptions) []SearchResult {
	v.mu.RLock()
	defer v.mu.RUnlock()

	scope := opts.Scope
	if scope == "" {
		scope = "all"
	}
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 10
	}

	type candidate struct {
		note      *Note
		score     float64
		matchType string
	}
	var candidates []candidate

	for _, note := range v.notes {
		if strings.HasPrefix(note.RelativePath, "templates/") || strings.HasPrefix(note.RelativePath, "templates\\") {
			continue
		}
		if opts.Topic != "" && !v.isUnderTopicLocked(note.Title, opts.Topic) {
			continue
		}
		if opts.Status != "" && note.Frontmatter.Status != opts.Status {
			continue
		}
		if opts.Tag != "" && !containsString(note.Frontmatter.Tags, opts.Tag) {
			continue
		}

		var titleScore, contentScore, descScore float64
		if scope == "title" || scope == "all" {
			titleScore = scoreTitleMatch(query, note.Title)
		}
		if scope == "content" || scope == "all" {
			contentScore = scoreContentMatch(query, note.Content) * 0.6
			if note.Frontmatter.Description != "" {
				descScore = scoreContentMatch(query, note.Frontmatter.Description) * 0.9
			}
		}

		scored := []struct {
			score float64
			label string
		}{
			{titleScore, "title"},
			{descScore, "description"},
			{contentScore, "content"},
		}
		sort.Slice(scored, func(i, j int) bool {
			return scored[i].score > scored[j].score
		})
		top := scored[0]
		if top.score > 0.05 {
			candidates = append(candidates, candidate{note: note, score: top.score, matchType: top.label})
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})
	if len(candidates) > maxResults {
		candidates = candidates[:maxResults]
	}

	includeGraph := opts.IncludeGraph
	graphDepth := opts.GraphDepth
	if graphDepth <= 0 {
		graphDepth = 1
	}

	results := make([]SearchResult, len(candidates))
	for i, c := range candidates {
		res := SearchResult{
			Note:      c.note,
			Score:     c.score,
			MatchType: c.matchType,
		}
		if includeGraph {
			ctx := v.getGraphContextLocked(c.note.Title, graphDepth)
			res.GraphContext = &ctx
		}
		results[i] = res
	}
	return results
}

func scoreTitleMatch(query, title string) float64 {
	q := strings.ToLower(query)
	t := strings.ToLower(title)
	if t == q {
		return 1.0
	}
	if strings.Contains(t, q) {
		return 0.8
	}
	if strings.Contains(q, t) {
		return 0.6
	}
	splitRe := regexp.MustCompile(`[\s\-_&()]+`)
	qWords := filterShort(splitRe.Split(q, -1), 1)
	tWords := filterShort(splitRe.Split(t, -1), 1)
	if len(qWords) == 0 || len(tWords) == 0 {
		return 0
	}
	matched := 0
	for _, qw := range qWords {
		for _, tw := range tWords {
			if strings.Contains(tw, qw) || strings.Contains(qw, tw) {
				matched++
				break
			}
		}
	}
	if matched == 0 {
		return 0
	}
	return 0.4 * float64(matched) / float64(len(qWords))
}

func scoreContentMatch(query, content string) float64 {
	if content == "" {
		return 0
	}
	q := strings.ToLower(query)
	c := strings.ToLower(content)

	if !strings.Contains(c, q) {
		splitRe := regexp.MustCompile(`[\s\-_&()]+`)
		qWords := filterShort(splitRe.Split(q, -1), 2)
		if len(qWords) == 0 {
			return 0
		}
		hits := 0
		for _, w := range qWords {
			if strings.Contains(c, w) {
				hits++
			}
		}
		if hits == 0 {
			return 0
		}
		return 0.3 * float64(hits) / float64(len(qWords))
	}
	count := strings.Count(c, q)
	val := 0.4 + 0.1*math.Log2(float64(count)+1)
	if val > 0.8 {
		return 0.8
	}
	return val
}

func filterShort(words []string, minLen int) []string {
	out := make([]string, 0, len(words))
	for _, w := range words {
		if len(w) > minLen {
			out = append(out, w)
		}
	}
	return out
}

func (v *Vault) isUnderTopicLocked(noteTitle, topicQuery string) bool {
	topics, ok := v.topicsOf[noteTitle]
	if !ok || len(topics) == 0 {
		return false
	}
	tq := strings.ToLower(topicQuery)
	for t := range topics {
		if strings.Contains(strings.ToLower(t), tq) {
			return true
		}
	}
	return false
}

func (v *Vault) GetTopicsOf(title string) []*Note {
	v.mu.RLock()
	defer v.mu.RUnlock()
	topics, ok := v.topicsOf[title]
	if !ok {
		return nil
	}
	out := make([]*Note, 0, len(topics))
	for t := range topics {
		if n, ok := v.notes[t]; ok {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out
}

package vault

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"
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
			Excerpt:   buildExcerpt(query, c.note, c.matchType),
		}
		if includeGraph {
			ctx := v.getGraphContextLocked(c.note.Title, graphDepth)
			res.GraphContext = &ctx
		}
		results[i] = res
	}
	return results
}

const (
	excerptMaxChars = 240
	excerptMaxLines = 3
)

// buildExcerpt returns a short snippet that shows why a search result matched.
// For title-only matches we skip it: the title is already shown in the result
// header, so a duplicate is just noise.
func buildExcerpt(query string, note *Note, matchType string) string {
	terms := queryTerms(query)
	if len(terms) == 0 {
		return ""
	}
	switch matchType {
	case "content":
		if ex := extractExcerpt(note.Content, terms); ex != "" {
			return ex
		}
		// Fall back to description if the content didn't actually contain a
		// term (can happen when the content score came from a fuzzy partial).
		return extractExcerpt(note.Frontmatter.Description, terms)
	case "description":
		return extractExcerpt(note.Frontmatter.Description, terms)
	}
	return ""
}

// queryTerms returns lowercased search terms: the full query plus each
// space/punctuation-separated word longer than 2 chars. Order preserved so
// highlighting picks the longest match first (full query before sub-words).
func queryTerms(query string) []string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	splitRe := regexp.MustCompile(`[\s\-_&()]+`)
	words := filterShort(splitRe.Split(q, -1), 2)
	terms := make([]string, 0, len(words)+1)
	seen := map[string]struct{}{}
	add := func(t string) {
		if t == "" {
			return
		}
		if _, ok := seen[t]; ok {
			return
		}
		seen[t] = struct{}{}
		terms = append(terms, t)
	}
	add(q)
	for _, w := range words {
		add(w)
	}
	// Sort longest-first so highlighting wraps the most specific match.
	sort.SliceStable(terms, func(i, j int) bool { return len(terms[i]) > len(terms[j]) })
	return terms
}

func extractExcerpt(text string, terms []string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")

	matchIdx := -1
	for i, line := range lines {
		if lineMatchesAny(line, terms) {
			matchIdx = i
			break
		}
	}
	if matchIdx == -1 {
		return ""
	}

	picked := make([]string, 0, excerptMaxLines)
	addLine := func(idx int) {
		if idx < 0 || idx >= len(lines) || len(picked) >= excerptMaxLines {
			return
		}
		l := strings.TrimSpace(lines[idx])
		if l == "" {
			return
		}
		picked = append(picked, l)
	}
	addLine(matchIdx)
	for offset := 1; len(picked) < excerptMaxLines && (matchIdx-offset >= 0 || matchIdx+offset < len(lines)); offset++ {
		addLine(matchIdx + offset)
		if len(picked) >= excerptMaxLines {
			break
		}
		addLine(matchIdx - offset)
	}

	snippet := strings.Join(picked, "\n")
	snippet = stripBoldMarkers(snippet)
	snippet = trimAroundMatch(snippet, terms, excerptMaxChars)
	return highlightTerms(snippet, terms)
}

// stripBoldMarkers removes "**" emphasis runs from an excerpt snippet. The
// snippet is a lossy preview and highlightTerms injects its own "**" around
// matched terms; leaving the source's "**" in place lets the two interleave
// into malformed markdown (e.g. "**No **PKCE**.**") when a match lands inside an
// already-bold run. Dropping source bold from the preview keeps the highlighted
// output well-formed. Single-marker emphasis (*, _, __) can't collide with our
// "**" injection, so it's left intact.
func stripBoldMarkers(s string) string {
	return strings.ReplaceAll(s, "**", "")
}

func lineMatchesAny(line string, terms []string) bool {
	l := strings.ToLower(line)
	for _, t := range terms {
		if strings.Contains(l, t) {
			return true
		}
	}
	return false
}

// lowerRunes returns a per-rune lowercased copy. unicode.ToLower maps each rune
// to exactly one rune, so index alignment with the input slice is preserved.
// (strings.ToLower on a string can change byte length — İ→i, K(U+212A)→k,
// ẞ→ß — which would desync byte offsets between the lowercased and original
// text and could mis-place highlights or panic.)
func lowerRunes(rs []rune) []rune {
	out := make([]rune, len(rs))
	for i, r := range rs {
		out[i] = unicode.ToLower(r)
	}
	return out
}

// indexRunes returns the index of the first occurrence of sub within s, or -1.
func indexRunes(s, sub []rune) int {
	if len(sub) == 0 {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for j := range sub {
			if s[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// trimAroundMatch caps snippet length (in runes), centering the window on the
// first matched term occurrence so the matched text stays visible. Rune-based
// so it never slices through a multi-byte character.
func trimAroundMatch(snippet string, terms []string, maxChars int) string {
	runes := []rune(snippet)
	if len(runes) <= maxChars {
		return snippet
	}
	lower := lowerRunes(runes)
	pos := -1
	for _, t := range terms {
		if i := indexRunes(lower, []rune(t)); i >= 0 {
			pos = i
			break
		}
	}
	if pos < 0 {
		return string(runes[:maxChars]) + "…"
	}
	half := maxChars / 2
	start := pos - half
	if start < 0 {
		start = 0
	}
	end := start + maxChars
	if end > len(runes) {
		end = len(runes)
		start = end - maxChars
		if start < 0 {
			start = 0
		}
	}
	out := string(runes[start:end])
	if start > 0 {
		out = "…" + out
	}
	if end < len(runes) {
		out = out + "…"
	}
	return out
}

// highlightTerms wraps matched terms with **…** for visibility. Case is
// preserved in the original snippet. Longest term wins per overlap. Matching
// and span-marking operate on runes (see lowerRunes) so multi-byte characters
// can't desync the offsets.
func highlightTerms(snippet string, terms []string) string {
	if snippet == "" || len(terms) == 0 {
		return snippet
	}
	runes := []rune(snippet)
	lower := lowerRunes(runes)
	marked := make([]bool, len(runes))
	type span struct{ start, end int }
	var spans []span
	for _, t := range terms {
		if t == "" {
			continue
		}
		tr := []rune(t)
		start := 0
		for start+len(tr) <= len(lower) {
			i := indexRunes(lower[start:], tr)
			if i < 0 {
				break
			}
			abs := start + i
			end := abs + len(tr)
			overlap := false
			for k := abs; k < end; k++ {
				if marked[k] {
					overlap = true
					break
				}
			}
			if !overlap {
				for k := abs; k < end; k++ {
					marked[k] = true
				}
				spans = append(spans, span{abs, end})
			}
			start = end
		}
	}
	if len(spans) == 0 {
		return snippet
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	var b strings.Builder
	prev := 0
	for _, s := range spans {
		b.WriteString(string(runes[prev:s.start]))
		b.WriteString("**")
		b.WriteString(string(runes[s.start:s.end]))
		b.WriteString("**")
		prev = s.end
	}
	b.WriteString(string(runes[prev:]))
	return b.String()
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

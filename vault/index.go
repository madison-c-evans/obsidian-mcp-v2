package vault

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type Vault struct {
	mu          sync.RWMutex
	vaultPath   string
	notes       map[string]*Note
	children    map[string]map[string]struct{}
	mentions    map[string]map[string]struct{}
	mentionedBy map[string]map[string]struct{}
	topicsOf    map[string]map[string]struct{}
	tagIndex    map[string]map[string]struct{}
}

func New(vaultPath string) (*Vault, error) {
	abs, err := filepath.Abs(vaultPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("vault path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("vault path is not a directory: %s", abs)
	}
	return &Vault{vaultPath: abs}, nil
}

func (v *Vault) VaultPath() string {
	return v.vaultPath
}

func (v *Vault) BuildIndex() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.buildIndexLocked()
}

func (v *Vault) buildIndexLocked() error {
	v.notes = make(map[string]*Note)
	v.children = make(map[string]map[string]struct{})
	v.mentions = make(map[string]map[string]struct{})
	v.mentionedBy = make(map[string]map[string]struct{})
	v.topicsOf = make(map[string]map[string]struct{})
	v.tagIndex = make(map[string]map[string]struct{})

	files, err := v.scanMarkdownFiles()
	if err != nil {
		return err
	}
	for _, fp := range files {
		note, err := v.parseNoteFile(fp)
		if err != nil {
			continue
		}
		v.notes[note.Title] = note
	}
	v.buildGraphEdges()
	v.buildMentionEdges()
	v.buildTagIndex()
	return nil
}

func (v *Vault) scanMarkdownFiles() ([]string, error) {
	var results []string
	err := filepath.WalkDir(v.vaultPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == v.vaultPath {
				return nil
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "images" || name == "pdfs" {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".md") {
			results = append(results, path)
		}
		return nil
	})
	return results, err
}

func (v *Vault) parseNoteFile(filePath string) (*Note, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	yamlBlock, body := splitFrontmatter(string(raw))

	fm := Frontmatter{}
	if yamlBlock != "" {
		var data map[string]any
		if err := yaml.Unmarshal([]byte(yamlBlock), &data); err == nil {
			fm = extractFrontmatter(data)
		}
	}

	fileName := strings.TrimSuffix(filepath.Base(filePath), ".md")
	relPath, _ := filepath.Rel(v.vaultPath, filePath)

	isTopic := strings.HasPrefix(relPath, "topics"+string(filepath.Separator)) || strings.HasPrefix(relPath, "topics/") || containsString(fm.Tags, "topic")

	return &Note{
		Title:        fileName,
		FileName:     fileName,
		FilePath:     filePath,
		RelativePath: relPath,
		Frontmatter:  fm,
		Content:      strings.TrimSpace(body),
		IsTopic:      isTopic,
	}, nil
}

// splitFrontmatter returns the YAML block (without --- fences) and the body.
// If no frontmatter is present, returns ("", raw).
func splitFrontmatter(raw string) (string, string) {
	raw = strings.TrimPrefix(raw, "\uFEFF")
	lines := strings.Split(raw, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", raw
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			yamlBlock := strings.Join(lines[1:i], "\n")
			body := strings.Join(lines[i+1:], "\n")
			return yamlBlock, body
		}
	}
	return "", raw
}

func extractFrontmatter(data map[string]any) Frontmatter {
	return Frontmatter{
		Tags:          normalizeStringArray(data["tags"]),
		ExternalLinks: normalizeStringArray(data["external-links"]),
		Upstream:      parseUpstreamField(data["upstream"]),
		Date:          normalizeDate(data["date"]),
		Status:        anyToString(data["status"]),
		Description:   anyToString(data["description"]),
	}
}

var wikilinkRe = regexp.MustCompile(`^\[\[(.+?)\]\]$`)

func parseUpstreamField(val any) []string {
	if val == nil {
		return nil
	}
	var items []any
	switch v := val.(type) {
	case []any:
		items = v
	case []string:
		for _, s := range v {
			items = append(items, s)
		}
	default:
		items = []any{val}
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s := strings.TrimSpace(anyToString(item))
		if m := wikilinkRe.FindStringSubmatch(s); m != nil {
			s = strings.TrimSpace(m[1])
		}
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func normalizeStringArray(val any) []string {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s := anyToString(item)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		s := anyToString(val)
		if s == "" {
			return nil
		}
		return []string{s}
	}
}

func normalizeDate(val any) string {
	if val == nil {
		return ""
	}
	if t, ok := val.(time.Time); ok {
		return t.Format("2006-01-02")
	}
	return strings.TrimSpace(anyToString(val))
}

func anyToString(val any) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case time.Time:
		return v.Format("2006-01-02")
	default:
		return fmt.Sprint(val)
	}
}

func containsString(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

// Edge building (called under write lock by buildIndexLocked, or under
// caller-held lock by write operations).

func (v *Vault) buildGraphEdges() {
	v.children = make(map[string]map[string]struct{})
	for title, note := range v.notes {
		for _, parentTitle := range note.Frontmatter.Upstream {
			set, ok := v.children[parentTitle]
			if !ok {
				set = make(map[string]struct{})
				v.children[parentTitle] = set
			}
			set[title] = struct{}{}
		}
	}
	v.buildTopicClosure()
}

func (v *Vault) buildTopicClosure() {
	v.topicsOf = make(map[string]map[string]struct{})
	for startTitle := range v.notes {
		topics := make(map[string]struct{})
		visited := make(map[string]struct{})
		stack := []string{startTitle}
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if _, seen := visited[cur]; seen {
				continue
			}
			visited[cur] = struct{}{}
			note, ok := v.notes[cur]
			if !ok {
				continue
			}
			if note.IsTopic {
				topics[cur] = struct{}{}
			}
			for _, parent := range note.Frontmatter.Upstream {
				stack = append(stack, parent)
			}
		}
		v.topicsOf[startTitle] = topics
	}
}

var inlineWikiRe = regexp.MustCompile(`\[\[([^\]|]+)(?:\|[^\]]*)?\]\]`)

func (v *Vault) buildMentionEdges() {
	v.mentions = make(map[string]map[string]struct{})
	v.mentionedBy = make(map[string]map[string]struct{})

	for title, note := range v.notes {
		seen := make(map[string]struct{})
		for _, m := range inlineWikiRe.FindAllStringSubmatch(note.Content, -1) {
			target := strings.TrimSpace(m[1])
			if target == title {
				continue
			}
			if _, ok := v.notes[target]; !ok {
				continue
			}
			seen[target] = struct{}{}
		}
		if len(seen) == 0 {
			continue
		}
		v.mentions[title] = seen
		for target := range seen {
			set, ok := v.mentionedBy[target]
			if !ok {
				set = make(map[string]struct{})
				v.mentionedBy[target] = set
			}
			set[title] = struct{}{}
		}
	}
}

func (v *Vault) GetStats() Stats {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.getStatsLocked()
}

func (v *Vault) getStatsLocked() Stats {
	var stats Stats
	for _, n := range v.notes {
		if n.IsTopic {
			stats.TopicCount++
		} else {
			stats.NoteCount++
		}
	}
	for _, set := range v.children {
		stats.EdgeCount += len(set)
	}
	for _, set := range v.mentions {
		stats.MentionCount += len(set)
	}
	return stats
}

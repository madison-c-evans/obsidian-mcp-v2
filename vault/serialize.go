package vault

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// knownFrontmatterKeys are the fields serializeFrontmatter renders explicitly
// in canonical form. Any other key is preserved verbatim (see preserveExtras)
// rather than dropped.
var knownFrontmatterKeys = map[string]struct{}{
	"tags":           {},
	"external-links": {},
	"upstream":       {},
	"date":           {},
	"status":         {},
	"description":    {},
}

// serializeNote rebuilds the on-disk file from a raw frontmatter map plus body.
// Mirrors the format the TS version emits: bare empty keys (not `null`),
// double-quoted wikilinks, unquoted dates, fixed field order.
//
// Note: any frontmatter fields outside the known schema are dropped — this
// matches the TS behavior and keeps the format predictable for Obsidian.
func serializeNote(data map[string]any, content string) string {
	fm := serializeFrontmatter(data)
	if strings.TrimSpace(content) == "" {
		return fm + "\n"
	}
	body := content
	if !strings.HasPrefix(body, "\n") {
		body = "\n" + body
	}
	return fm + body + "\n"
}

func serializeFrontmatter(data map[string]any) string {
	var lines []string
	lines = append(lines, "---")

	tags := toStringSlice(data["tags"])
	if len(tags) > 0 {
		lines = append(lines, "tags:")
		for _, t := range tags {
			lines = append(lines, "  - "+t)
		}
	} else {
		lines = append(lines, "tags:")
	}

	extLinks := toStringSlice(data["external-links"])
	if len(extLinks) > 0 {
		lines = append(lines, "external-links:")
		for _, l := range extLinks {
			lines = append(lines, "  - "+l)
		}
	} else {
		lines = append(lines, "external-links:")
	}

	upstream := data["upstream"]
	switch v := upstream.(type) {
	case nil:
		lines = append(lines, "upstream:")
	case string:
		if v == "" {
			lines = append(lines, "upstream:")
		} else {
			lines = append(lines, "upstream: "+yamlDoubleQuote(v))
		}
	default:
		us := toStringSlice(upstream)
		switch len(us) {
		case 0:
			lines = append(lines, "upstream:")
		case 1:
			lines = append(lines, "upstream: "+yamlDoubleQuote(us[0]))
		default:
			lines = append(lines, "upstream:")
			for _, u := range us {
				lines = append(lines, "  - "+yamlDoubleQuote(u))
			}
		}
	}

	date := normalizeDate(data["date"])
	if date != "" {
		lines = append(lines, "date: "+date)
	} else {
		lines = append(lines, "date:")
	}

	status := anyToString(data["status"])
	lines = append(lines, "status: "+status)

	if _, ok := data["description"]; ok {
		desc := anyToString(data["description"])
		switch {
		case desc == "":
			lines = append(lines, "description:")
		case strings.ContainsAny(desc, ":#\"'") || strings.Contains(desc, "\n"):
			escaped := strings.ReplaceAll(desc, `\`, `\\`)
			escaped = strings.ReplaceAll(escaped, `"`, `\"`)
			lines = append(lines, fmt.Sprintf(`description: "%s"`, escaped))
		default:
			lines = append(lines, "description: "+desc)
		}
	}

	lines = append(lines, preserveExtras(data)...)

	lines = append(lines, "---")
	return strings.Join(lines, "\n")
}

// preserveExtras renders any frontmatter keys outside the known schema (e.g.
// Obsidian fields like aliases, cssclass, publish) so an edit doesn't silently
// drop them. Keys are emitted alphabetically, after the canonical fields, using
// the same 2-space indent as the hand-rolled known fields.
func preserveExtras(data map[string]any) []string {
	var keys []string
	for k := range data {
		if _, known := knownFrontmatterKeys[k]; !known {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)

	var out []string
	for _, k := range keys {
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(map[string]any{k: data[k]}); err != nil {
			enc.Close()
			continue
		}
		enc.Close()
		rendered := strings.TrimRight(buf.String(), "\n")
		if rendered != "" {
			out = append(out, rendered)
		}
	}
	return out
}

func yamlDoubleQuote(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch s := v.(type) {
	case []string:
		out := make([]string, 0, len(s))
		for _, x := range s {
			if x != "" {
				out = append(out, x)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(s))
		for _, item := range s {
			str := anyToString(item)
			if str != "" {
				out = append(out, str)
			}
		}
		return out
	case string:
		if s == "" {
			return nil
		}
		return []string{s}
	}
	return nil
}

// upstreamToRaw turns plain titles into the canonical raw form that
// serializeFrontmatter accepts: nil for empty, a single quoted wikilink for
// one, a list of wikilinks for many.
func upstreamToRaw(upstream []string) any {
	clean := make([]string, 0, len(upstream))
	for _, u := range upstream {
		u = strings.TrimSpace(u)
		if u != "" {
			clean = append(clean, u)
		}
	}
	if len(clean) == 0 {
		return nil
	}
	wikilinks := make([]string, len(clean))
	for i, u := range clean {
		wikilinks[i] = fmt.Sprintf("[[%s]]", u)
	}
	if len(wikilinks) == 1 {
		return wikilinks[0]
	}
	out := make([]any, len(wikilinks))
	for i, w := range wikilinks {
		out[i] = w
	}
	return out
}

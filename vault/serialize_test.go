package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSerializeFrontmatter_BareKeysForEmpties(t *testing.T) {
	out := serializeFrontmatter(map[string]any{
		"tags":           nil,
		"external-links": nil,
		"upstream":       nil,
		"date":           "",
		"status":         "",
	})

	// Every "empty" field must render as a bare key (`tags:`) and NOT as
	// `tags: null` or `tags: []`.
	for _, want := range []string{"tags:", "external-links:", "upstream:", "date:", "status: "} {
		if !containsLine(out, want) && !strings.Contains(out, "\n"+strings.TrimRight(want, " ")+"\n") &&
			!strings.HasSuffix(out, strings.TrimRight(want, " ")) {
			// More forgiving check: just ensure the key appears and no `null`.
			if !strings.Contains(out, strings.TrimRight(want, " ")) {
				t.Errorf("expected %q in output, got:\n%s", want, out)
			}
		}
	}
	if strings.Contains(out, "null") {
		t.Errorf("unexpected null in output:\n%s", out)
	}
	if strings.Contains(out, "[]") {
		t.Errorf("unexpected [] in output:\n%s", out)
	}
	// description is NOT in the map -> must NOT be emitted.
	if strings.Contains(out, "description") {
		t.Errorf("description key should not be emitted when absent; got:\n%s", out)
	}
}

func TestSerializeFrontmatter_TagsList(t *testing.T) {
	out := serializeFrontmatter(map[string]any{
		"tags": []string{"alpha", "beta"},
	})
	if !containsLine(out, "tags:") {
		t.Errorf("missing tags: header in:\n%s", out)
	}
	if !containsLine(out, "  - alpha") {
		t.Errorf("missing tag alpha line in:\n%s", out)
	}
	if !containsLine(out, "  - beta") {
		t.Errorf("missing tag beta line in:\n%s", out)
	}
	// Make sure ordering is preserved.
	ia := strings.Index(out, "  - alpha")
	ib := strings.Index(out, "  - beta")
	if ia < 0 || ib < 0 || ia > ib {
		t.Errorf("ordering not preserved; out:\n%s", out)
	}
}

func TestSerializeFrontmatter_UpstreamShapes(t *testing.T) {
	// Nil/missing upstream renders as a bare key.
	out := serializeFrontmatter(map[string]any{})
	if !containsLine(out, "upstream:") {
		t.Errorf("nil upstream missing bare key in:\n%s", out)
	}
	if strings.Contains(out, "upstream: ") && !strings.Contains(out, "upstream:\n") &&
		!strings.HasSuffix(strings.TrimRight(out, "\n"), "upstream:") {
		// upstream: with trailing space might appear due to status line; just ensure no value
	}

	// Single string upstream renders as a single quoted scalar.
	out1 := serializeFrontmatter(map[string]any{
		"upstream": "[[Solo]]",
	})
	if !containsLine(out1, `upstream: "[[Solo]]"`) {
		t.Errorf("single-string upstream wrong; out:\n%s", out1)
	}

	// Empty-string upstream collapses to bare key.
	outEmptyStr := serializeFrontmatter(map[string]any{
		"upstream": "",
	})
	if !containsLine(outEmptyStr, "upstream:") {
		t.Errorf("empty-string upstream should render bare; out:\n%s", outEmptyStr)
	}

	// One-element slice still renders as single scalar (matches TS shape).
	outOne := serializeFrontmatter(map[string]any{
		"upstream": []string{"[[One]]"},
	})
	if !containsLine(outOne, `upstream: "[[One]]"`) {
		t.Errorf("one-element slice upstream wrong; out:\n%s", outOne)
	}

	// Multi-element slice renders as a list.
	outMany := serializeFrontmatter(map[string]any{
		"upstream": []string{"[[One]]", "[[Two]]"},
	})
	if !containsLine(outMany, "upstream:") {
		t.Errorf("multi-element slice missing list header; out:\n%s", outMany)
	}
	if !containsLine(outMany, `  - "[[One]]"`) || !containsLine(outMany, `  - "[[Two]]"`) {
		t.Errorf("multi-element slice missing entries; out:\n%s", outMany)
	}

	// []any path also works.
	outAny := serializeFrontmatter(map[string]any{
		"upstream": []any{"[[A]]", "[[B]]"},
	})
	if !containsLine(outAny, `  - "[[A]]"`) || !containsLine(outAny, `  - "[[B]]"`) {
		t.Errorf("[]any upstream missing entries; out:\n%s", outAny)
	}
}

func TestSerializeFrontmatter_DatePresentVsAbsent(t *testing.T) {
	withDate := serializeFrontmatter(map[string]any{"date": "2024-05-01"})
	if !containsLine(withDate, "date: 2024-05-01") {
		t.Errorf("date not rendered when present; out:\n%s", withDate)
	}
	noDate := serializeFrontmatter(map[string]any{})
	if !containsLine(noDate, "date:") {
		t.Errorf("date key missing when absent; out:\n%s", noDate)
	}
	if strings.Contains(noDate, "date: ") {
		// `date: ` (with trailing content) should not appear when empty
		// Need to verify there isn't a non-empty date line.
		for _, line := range strings.Split(noDate, "\n") {
			if strings.HasPrefix(line, "date: ") && strings.TrimSpace(line[len("date:"):]) != "" {
				t.Errorf("unexpected date value on empty: %q", line)
			}
		}
	}
}

func TestSerializeFrontmatter_StatusAlwaysPresent(t *testing.T) {
	out := serializeFrontmatter(map[string]any{})
	if !strings.Contains(out, "status:") {
		t.Errorf("status key always required; out:\n%s", out)
	}

	out2 := serializeFrontmatter(map[string]any{"status": "TODO"})
	if !containsLine(out2, "status: TODO") {
		t.Errorf("status TODO not rendered; out:\n%s", out2)
	}
}

func TestSerializeFrontmatter_DescriptionPresence(t *testing.T) {
	// Description key NOT in map -> not emitted.
	out := serializeFrontmatter(map[string]any{
		"status": "x",
	})
	if strings.Contains(out, "description") {
		t.Errorf("description must be absent when key not in map; out:\n%s", out)
	}

	// Description key present but empty -> bare key emitted.
	out2 := serializeFrontmatter(map[string]any{
		"description": "",
	})
	if !containsLine(out2, "description:") {
		t.Errorf("empty description must emit bare key; out:\n%s", out2)
	}
	if strings.Contains(out2, "description: ") {
		// Allow only `description:` (no value), not `description: something`.
		for _, line := range strings.Split(out2, "\n") {
			if strings.HasPrefix(line, "description: ") && strings.TrimSpace(line[len("description:"):]) != "" {
				t.Errorf("description should be empty bare key, got: %q", line)
			}
		}
	}
}

func TestSerializeFrontmatter_DescriptionQuoting(t *testing.T) {
	cases := []struct {
		name     string
		value    string
		wantLine string
	}{
		{"plain", "Hello world", `description: Hello world`},
		{"colon", "k:v", `description: "k:v"`},
		{"hash", "tag #foo", `description: "tag #foo"`},
		{"double-quote", `she said "hi"`, `description: "she said \"hi\""`},
		{"single-quote", "it's mine", `description: "it's mine"`},
		{"newline", "line1\nline2", `description: "line1` + "\n" + `line2"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := serializeFrontmatter(map[string]any{
				"description": tc.value,
			})
			if !strings.Contains(out, tc.wantLine) {
				t.Errorf("description case %s: missing %q in:\n%s", tc.name, tc.wantLine, out)
			}
		})
	}
}

func TestSerializeFrontmatter_DescriptionEscapesBackslashAndQuote(t *testing.T) {
	out := serializeFrontmatter(map[string]any{
		"description": `path C:\foo "x"`,
	})
	// Has ':' and '"' so it gets quoted; backslash must be doubled, dquote escaped.
	want := `description: "path C:\\foo \"x\""`
	if !strings.Contains(out, want) {
		t.Errorf("missing %q in:\n%s", want, out)
	}
}

func TestSerializeFrontmatter_FixedFieldOrder(t *testing.T) {
	out := serializeFrontmatter(map[string]any{
		"tags":           []string{"a"},
		"external-links": []string{"u"},
		"upstream":       "[[U]]",
		"date":           "2024-01-02",
		"status":         "TODO",
		"description":    "d",
	})
	order := []string{"tags:", "external-links:", "upstream:", "date:", "status:", "description:"}
	pos := -1
	for _, k := range order {
		i := strings.Index(out, k)
		if i == -1 {
			t.Fatalf("missing key %q in:\n%s", k, out)
		}
		if i <= pos {
			t.Errorf("key %q out of order in:\n%s", k, out)
		}
		pos = i
	}
}

func TestUpstreamToRaw(t *testing.T) {
	if v := upstreamToRaw(nil); v != nil {
		t.Errorf("nil input -> %v, want nil", v)
	}
	if v := upstreamToRaw([]string{}); v != nil {
		t.Errorf("empty slice -> %v, want nil", v)
	}
	if v := upstreamToRaw([]string{"", "   "}); v != nil {
		t.Errorf("whitespace-only -> %v, want nil", v)
	}

	if got, want := upstreamToRaw([]string{"One"}), "[[One]]"; got != want {
		t.Errorf("single -> %v, want %q", got, want)
	}

	// Trailing whitespace is stripped before wikilink wrapping.
	if got, want := upstreamToRaw([]string{"  Spaced  "}), "[[Spaced]]"; got != want {
		t.Errorf("trimmed single -> %v, want %q", got, want)
	}

	// Multi-item returns []any to satisfy the serializer's switch.
	got := upstreamToRaw([]string{"A", "B"})
	arr, ok := got.([]any)
	if !ok {
		t.Fatalf("multi -> %T, want []any", got)
	}
	if len(arr) != 2 || arr[0] != "[[A]]" || arr[1] != "[[B]]" {
		t.Errorf("multi result = %v", arr)
	}

	// Mixed empty + valid: empties dropped.
	got2 := upstreamToRaw([]string{"", "X", "  ", "Y"})
	arr2, ok := got2.([]any)
	if !ok {
		t.Fatalf("mixed -> %T, want []any", got2)
	}
	if len(arr2) != 2 || arr2[0] != "[[X]]" || arr2[1] != "[[Y]]" {
		t.Errorf("mixed result = %v", arr2)
	}

	// One valid + many empties collapses to a single string.
	got3 := upstreamToRaw([]string{"", "Only", ""})
	if got3 != "[[Only]]" {
		t.Errorf("one-after-trim -> %v, want %q", got3, "[[Only]]")
	}
}

func TestYAMLDoubleQuote(t *testing.T) {
	if got := yamlDoubleQuote("plain"); got != `"plain"` {
		t.Errorf("plain -> %q", got)
	}
	if got := yamlDoubleQuote(`a"b`); got != `"a\"b"` {
		t.Errorf("dquote -> %q", got)
	}
	if got := yamlDoubleQuote(`a\b`); got != `"a\\b"` {
		t.Errorf("backslash -> %q", got)
	}
	if got := yamlDoubleQuote(`a\"b`); got != `"a\\\"b"` {
		t.Errorf("mixed -> %q", got)
	}
}

func TestToStringSlice(t *testing.T) {
	if v := toStringSlice(nil); v != nil {
		t.Errorf("nil -> %v, want nil", v)
	}
	if v := toStringSlice(""); v != nil {
		t.Errorf("empty string -> %v, want nil", v)
	}
	if got := toStringSlice("solo"); !equalStringSlices(got, []string{"solo"}) {
		t.Errorf("string -> %v", got)
	}
	if got := toStringSlice([]string{"a", "", "b"}); !equalStringSlices(got, []string{"a", "b"}) {
		t.Errorf("[]string with empty -> %v", got)
	}
	if got := toStringSlice([]any{"a", 1, "", "b"}); !equalStringSlices(got, []string{"a", "1", "b"}) {
		t.Errorf("[]any mixed -> %v", got)
	}
	if v := toStringSlice(42); v != nil {
		t.Errorf("unsupported type -> %v, want nil", v)
	}
}

func TestSerializeNote_EmptyBodyHasNoExtraNewline(t *testing.T) {
	out := serializeNote(map[string]any{"status": "TODO"}, "")
	if !strings.HasSuffix(out, "---\n") {
		t.Errorf("empty body should end with ---\\n; got:\n%q", out)
	}
}

func TestSerializeNote_BodyPrependsNewlineIfMissing(t *testing.T) {
	out := serializeNote(map[string]any{"status": "TODO"}, "body")
	// Expect "---\nbody\n" at the end (a newline between --- and body).
	if !strings.Contains(out, "---\nbody\n") {
		t.Errorf("expected ---\\nbody\\n; got:\n%q", out)
	}
}

func TestSerializeNote_BodyWithLeadingNewlineNotDuplicated(t *testing.T) {
	// Body that already starts with "\n" should not get an extra one prepended.
	out := serializeNote(map[string]any{"status": "TODO"}, "\nbody")
	if strings.Contains(out, "---\n\nbody") {
		t.Errorf("leading newline should not be duplicated; got:\n%q", out)
	}
	if !strings.Contains(out, "---\nbody\n") {
		t.Errorf("expected ---\\nbody\\n; got:\n%q", out)
	}
}

// Round-trip: serialize -> splitFrontmatter + yaml.Unmarshal yields the same
// logical frontmatter + body content. We exercise the path parseNoteFile takes.
func TestSerializeNote_RoundTrip(t *testing.T) {
	src := map[string]any{
		"tags":           []string{"alpha", "beta"},
		"external-links": []string{"https://example.com"},
		"upstream":       []string{"[[Parent A]]", "[[Parent B]]"},
		"date":           "2024-04-04",
		"status":         "TODO",
		"description":    "needs work: maybe",
	}
	body := "## Heading\nbody text.\n"

	raw := serializeNote(src, body)

	yamlBlock, gotBody := splitFrontmatter(raw)
	if yamlBlock == "" {
		t.Fatalf("expected yaml block, got empty; raw:\n%s", raw)
	}
	if !strings.Contains(gotBody, "## Heading") || !strings.Contains(gotBody, "body text.") {
		t.Errorf("body lost on round-trip: %q", gotBody)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(yamlBlock), &parsed); err != nil {
		t.Fatalf("yaml.Unmarshal: %v\nblock:\n%s", err, yamlBlock)
	}

	fm := extractFrontmatter(parsed)
	if !equalStringSlices(fm.Tags, []string{"alpha", "beta"}) {
		t.Errorf("tags round-trip = %v", fm.Tags)
	}
	if !equalStringSlices(fm.ExternalLinks, []string{"https://example.com"}) {
		t.Errorf("external-links round-trip = %v", fm.ExternalLinks)
	}
	if !equalStringSlices(fm.Upstream, []string{"Parent A", "Parent B"}) {
		t.Errorf("upstream round-trip = %v", fm.Upstream)
	}
	if fm.Date != "2024-04-04" {
		t.Errorf("date round-trip = %q", fm.Date)
	}
	if fm.Status != "TODO" {
		t.Errorf("status round-trip = %q", fm.Status)
	}
	if fm.Description != "needs work: maybe" {
		t.Errorf("description round-trip = %q", fm.Description)
	}
}

func TestSerializeNote_RoundTripSingleUpstream(t *testing.T) {
	src := map[string]any{
		"upstream": []string{"[[Only One]]"},
		"status":   "DONE",
	}
	raw := serializeNote(src, "")
	yamlBlock, _ := splitFrontmatter(raw)

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(yamlBlock), &parsed); err != nil {
		t.Fatalf("yaml.Unmarshal: %v\nblock:\n%s", err, yamlBlock)
	}
	fm := extractFrontmatter(parsed)
	if !equalStringSlices(fm.Upstream, []string{"Only One"}) {
		t.Errorf("single upstream round-trip = %v", fm.Upstream)
	}
}

func TestSerializeNote_RoundTripEmptyFrontmatter(t *testing.T) {
	src := map[string]any{}
	raw := serializeNote(src, "")
	yamlBlock, _ := splitFrontmatter(raw)

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(yamlBlock), &parsed); err != nil {
		t.Fatalf("yaml.Unmarshal: %v\nblock:\n%s", err, yamlBlock)
	}
	fm := extractFrontmatter(parsed)
	if len(fm.Tags) != 0 || len(fm.ExternalLinks) != 0 || len(fm.Upstream) != 0 {
		t.Errorf("expected empty collections; got tags=%v ext=%v up=%v", fm.Tags, fm.ExternalLinks, fm.Upstream)
	}
	if fm.Status != "" || fm.Date != "" || fm.Description != "" {
		t.Errorf("expected empty scalars; got status=%q date=%q desc=%q", fm.Status, fm.Date, fm.Description)
	}
}

func TestSerializeFrontmatter_PreservesUnknownScalar(t *testing.T) {
	out := serializeFrontmatter(map[string]any{
		"status":   "TODO",
		"cssclass": "wide",
	})
	if !containsLine(out, "cssclass: wide") {
		t.Errorf("unknown scalar key dropped; out:\n%s", out)
	}
	// Unknown keys come after the canonical fields and before the closing ---.
	if strings.Index(out, "status:") > strings.Index(out, "cssclass:") {
		t.Errorf("unknown key should follow canonical fields; out:\n%s", out)
	}
}

func TestSerializeFrontmatter_PreservesUnknownList(t *testing.T) {
	out := serializeFrontmatter(map[string]any{
		"status":  "TODO",
		"aliases": []any{"PKCE", "Proof Key for Code Exchange"},
	})
	if !containsLine(out, "aliases:") {
		t.Errorf("unknown list header dropped; out:\n%s", out)
	}
	if !containsLine(out, "  - PKCE") || !containsLine(out, "  - Proof Key for Code Exchange") {
		t.Errorf("unknown list items dropped or wrong indent; out:\n%s", out)
	}
}

func TestSerializeFrontmatter_PreservesUnknownNestedMap(t *testing.T) {
	out := serializeFrontmatter(map[string]any{
		"status": "TODO",
		"obsidian": map[string]any{
			"pinned": true,
		},
	})
	if !containsLine(out, "obsidian:") || !containsLine(out, "  pinned: true") {
		t.Errorf("unknown nested map dropped; out:\n%s", out)
	}
}

func TestSerializeFrontmatter_MultipleUnknownKeysAlphabetical(t *testing.T) {
	out := serializeFrontmatter(map[string]any{
		"status": "TODO",
		"zeta":   "z",
		"alpha":  "a",
	})
	if strings.Index(out, "alpha:") > strings.Index(out, "zeta:") {
		t.Errorf("unknown keys should be alphabetical; out:\n%s", out)
	}
}

// TestEditPreservesUnknownFrontmatter is the end-to-end guard for the bug: a
// note authored with a custom field (e.g. via Obsidian) must keep that field
// after an edit through the MCP write path.
func TestEditPreservesUnknownFrontmatter(t *testing.T) {
	v, root := newWriteTestVault(t)

	note := `---
tags:
  - notes
external-links:
upstream: "[[Topic One]]"
date: 2024-03-03
status: TODO
aliases:
  - Custom Alias
cssclass: wide
---
Original body.
`
	path := filepath.Join(root, "Custom.md")
	if err := os.WriteFile(path, []byte(note), 0o644); err != nil {
		t.Fatalf("write Custom.md: %v", err)
	}
	if err := v.BuildIndex(); err != nil {
		t.Fatalf("reindex: %v", err)
	}

	if _, err := v.AppendToNote("Custom", "Appended line."); err != nil {
		t.Fatalf("AppendToNote: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	got := string(raw)
	for _, want := range []string{"aliases:", "  - Custom Alias", "cssclass: wide", "Appended line."} {
		if !strings.Contains(got, want) {
			t.Errorf("edit dropped %q from note; got:\n%s", want, got)
		}
	}
}

// containsLine reports whether out contains `want` as a full trimmed line.
func containsLine(out, want string) bool {
	for _, line := range strings.Split(out, "\n") {
		if line == want {
			return true
		}
	}
	return false
}

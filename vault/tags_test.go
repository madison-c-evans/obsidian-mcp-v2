package vault

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeFixture(t *testing.T, dir, rel, body string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

func buildTestVault(t *testing.T) *Vault {
	t.Helper()
	dir := t.TempDir()

	writeFixture(t, dir, "A.md", `---
tags:
  - foo
  - bar
upstream:
---
A body
`)
	writeFixture(t, dir, "B.md", `---
tags:
  - foo
upstream: "[[Topic1]]"
---
B body
`)
	writeFixture(t, dir, "C.md", `---
tags:
  - foo
  - bar
upstream: "[[Topic1]]"
---
C body
`)
	writeFixture(t, dir, "topics/Topic1.md", `---
tags:
  - topic
  - foo
upstream:
---
Topic1 body
`)
	// Templates should be excluded from the index.
	writeFixture(t, dir, "templates/T.md", `---
tags:
  - templated
  - foo
upstream:
---
template body
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

func TestListTags_AllNotes(t *testing.T) {
	v := buildTestVault(t)

	got := v.ListTags("")
	want := []TagCount{
		{Tag: "foo", Count: 4},
		{Tag: "bar", Count: 2},
		{Tag: "topic", Count: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListTags(\"\") = %+v, want %+v", got, want)
	}
}

func TestListTags_TopicFilter(t *testing.T) {
	v := buildTestVault(t)

	got := v.ListTags("Topic1")
	// Under Topic1: B (foo), C (foo, bar), Topic1 itself (topic, foo).
	want := []TagCount{
		{Tag: "foo", Count: 3},
		{Tag: "bar", Count: 1},
		{Tag: "topic", Count: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListTags(\"Topic1\") = %+v, want %+v", got, want)
	}
}

func TestListTags_TopicFilterUnknown(t *testing.T) {
	v := buildTestVault(t)

	got := v.ListTags("NoSuchTopic")
	if len(got) != 0 {
		t.Fatalf("ListTags(\"NoSuchTopic\") = %+v, want empty", got)
	}
}

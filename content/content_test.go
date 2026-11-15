package content

import (
	"reflect"
	"testing"
)

func TestSplitFrontmatter(t *testing.T) {
	fm, body, has := SplitFrontmatter("\n---\ntitle: A\n---\nBody\n")
	if !has || fm != "title: A" || body != "Body\n" {
		t.Fatalf("got %q %q %v", fm, body, has)
	}
	if _, body, has := SplitFrontmatter("no fence"); has || body != "no fence" {
		t.Fatal("no fence")
	}
	if _, _, has := SplitFrontmatter("---\ntitle: A\n"); has {
		t.Fatal("unclosed fence must not count")
	}
	fm, body, _ = SplitFrontmatter("---\r\ntitle: A\r\n---\r\nB")
	if fm != "title: A\r" || body != "B" {
		t.Fatalf("crlf: %q %q", fm, body)
	}
}

func TestScalarTyping(t *testing.T) {
	m, err := ParseYAMLMap([]byte(`
n: 3
f: 2.5
hex: 0x10
b: true
noYes: yes
s: '007'
d: 2025-01-01
empty:
tilde: ~
list: [1, two]
nested:
  k: v
`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"n": float64(3), "f": 2.5, "hex": float64(16), "b": true, "noYes": "yes", "s": "007",
		"d": "2025-01-01", "empty": nil, "tilde": nil,
		"list":   []any{float64(1), "two"},
		"nested": map[string]any{"k": "v"},
	}
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("got %#v\nwant %#v", m, want)
	}
	if _, err := ParseYAMLMap([]byte("- a\n- b\n")); err == nil {
		t.Fatal("a list document must be rejected")
	}
	if m, err := ParseYAMLMap([]byte("")); err != nil || len(m) != 0 {
		t.Fatal("empty document is an empty map")
	}
}

func TestParseDocuments(t *testing.T) {
	d, err := Parse("src/content/settings/nav.yml", []byte("main:\n  - label: A\n"))
	if err != nil || !d.IsData || !d.HasFrontmatter || d.Body != "" {
		t.Fatalf("data doc: %+v %v", d, err)
	}
	d, err = Parse("src/content/pages/a.md", []byte("---\ntitle: A\n---\nHello"))
	if err != nil || d.IsData || !d.HasFrontmatter || d.Frontmatter["title"] != "A" || d.Body != "Hello" {
		t.Fatalf("md doc: %+v %v", d, err)
	}
	d, err = Parse("src/content/pages/b.md", []byte("just text"))
	if err != nil || d.HasFrontmatter || d.Body != "just text" {
		t.Fatalf("no frontmatter: %+v %v", d, err)
	}
	if _, err := Parse("x.md", []byte("---\ntitle: [\n---\n")); err == nil {
		t.Fatal("broken yaml must error")
	}
	if !IsContentFile("a.mdx") || IsContentFile("a.txt") || !IsDataFile("a.yaml") || IsDataFile("a.md") {
		t.Fatal("extensions")
	}
}

func TestSidecarPaths(t *testing.T) {
	if got := SidecarPath("src/content/pages/about.md"); got != ".norcube/cms/meta/src/content/pages/about.yml" {
		t.Fatalf("sidecar path: %q", got)
	}
	paths := ContentPathsForSidecar(".norcube/cms/meta/src/content/pages/about.yml")
	if len(paths) == 0 || paths[0] != "src/content/pages/about.md" {
		t.Fatalf("content paths: %v", paths)
	}
	if ContentPathsForSidecar("elsewhere/about.yml") != nil {
		t.Fatal("outside the meta dir")
	}
	sc, err := ParseSidecar([]byte("schemaVersion: 1\npage: abc\narrays:\n  blocks:\n    - id: x\n"))
	if err != nil || sc.Page != "abc" || len(sc.Arrays["blocks"]) != 1 {
		t.Fatalf("parse sidecar: %+v %v", sc, err)
	}
}

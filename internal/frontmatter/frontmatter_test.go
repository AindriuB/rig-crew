package frontmatter

import (
	"reflect"
	"strings"
	"testing"
)

func TestFrontmatter_RoundTrip(t *testing.T) {
	fields := []Field{{"name", "explorer"}, {"description", "Finds things"}}
	body := "Body line one.\n\nLine two.\n"
	got, gotBody, err := Parse(Format(fields, body))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, fields) || gotBody != body {
		t.Fatalf("got %v %q", got, gotBody)
	}
}

func TestFrontmatter_QuotesWhenNeeded(t *testing.T) {
	vals := []string{"", "a: b", "x #y", "- item", "[x]", "'q'", "say \"hi\"", " lead", "a\nb", "plain text", "end:"}
	var fields []Field
	for i, v := range vals {
		fields = append(fields, Field{string(rune('a' + i)), v})
	}
	out := string(Format(fields, ""))
	for _, want := range []string{"a: \"\"\n", "b: \"a: b\"\n", "c: \"x #y\"\n", "d: \"- item\"\n", "j: plain text\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
	got, _, err := Parse([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, fields) {
		t.Fatalf("round trip differs: %v", got)
	}
}

func TestFrontmatter_AcceptsCRLF(t *testing.T) {
	got, body, err := Parse([]byte("---\r\nname: x\r\ndescription: \"a: b\"\r\n---\r\nhello\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []Field{{"name", "x"}, {"description", "a: b"}}
	if !reflect.DeepEqual(got, want) || body != "hello\n" {
		t.Fatalf("got %v %q", got, body)
	}
}

func TestFrontmatter_RejectsMalformed(t *testing.T) {
	for _, in := range []string{"no front matter", "---\nname: x\n", "---\nnokey\n---\n", "---\na: \"bad\n---\n"} {
		if _, _, err := Parse([]byte(in)); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}

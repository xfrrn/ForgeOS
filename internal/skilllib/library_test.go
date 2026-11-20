package skilllib

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestPackageFilesAndReadScope(t *testing.T) {
	t.Setenv("FORGEOS_SKILLS_DIR", t.TempDir())
	doc := File{"SKILL.md", []byte("---\nname: guide\ndescription: Read the included notes\n---\nOpen references/data.txt.")}
	ref := File{"references/data.txt", []byte("first")}
	pkg, err := Publish([]File{doc, ref, {"assets/image.bin", []byte{0, 255}}})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(pkg.Entry)
	s, err := readSelected([]string{dir}, filepath.Join(dir, "references", "data.txt"), 1)
	if err != nil || s != "irst" {
		t.Fatal("skill reference unreadable", err)
	}
	other, err := Publish([]File{doc})
	if err != nil || other.Key == pkg.Key {
		t.Fatal("changed files did not create a new version", err)
	}
	if _, err = readSelected([]string{dir}, other.Entry, 0); err == nil {
		t.Fatal("read unassigned package")
	}
	if _, err = readSelected([]string{dir}, filepath.Dir(dir), 0); err == nil {
		t.Fatal("listed shared library")
	}
	for _, path := range []string{"../outside", "/bad", "references/../../secret", "references/data.txt:stream", "references/DATA.txt", "SKILL.md/nested"} {
		if _, err = Publish([]File{doc, ref, {path, nil}}); err == nil {
			t.Fatalf("accepted invalid path %q", path)
		}
	}
	query := bytes.NewBufferString("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}\n")
	var result bytes.Buffer
	if err = Serve(query, &result, filepath.Dir(dir), []string{pkg.Key}); err != nil || !bytes.Contains(result.Bytes(), []byte("\"read\"")) {
		t.Fatal("missing MCP read tool", err)
	}
}

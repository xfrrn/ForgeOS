package skilllib

import (
	"errors"
	"go.yaml.in/yaml/v3"
	"strings"
	"unicode"
	"unicode/utf8"
)

var ErrDocument = errors.New("SKILL.md 需要包含 name、description 的 YAML 文件头和非空正文，并采用 UTF-8 编码。")

func metadata(raw []byte) (string, string, error) {
	if len(raw) > 1<<20 || !utf8.Valid(raw) || strings.ContainsRune(string(raw), 0) {
		return "", "", ErrDocument
	}
	document := string(raw)
	doc := strings.TrimPrefix(strings.ReplaceAll(document, "\r\n", "\n"), "\uFEFF")
	rest, ok := strings.CutPrefix(doc, "---\n")
	header, body, closed := strings.Cut(rest, "\n---\n")
	var meta struct {
		Name        string
		Description string
	}
	if !ok || !closed || yaml.Unmarshal([]byte(header), &meta) != nil {
		return "", "", ErrDocument
	}
	name, description, body := strings.TrimSpace(meta.Name), strings.TrimSpace(meta.Description), strings.TrimSpace(body)
	if name == "" || len(name) > 256 || strings.IndexFunc(name, unicode.IsControl) >= 0 || description == "" || len(description) > 4096 || body == "" || strings.ContainsRune(description, 31) {
		return "", "", ErrDocument
	}
	return name, description, nil
}

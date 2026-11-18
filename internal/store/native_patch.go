package store

import (
	"io/fs"
	"strings"
	"unicode"
	"unicode/utf8"

	"forgeos/internal/validation"
)

type NativePatchFile struct {
	Path   string `json:"path"`
	Op     string `json:"op"`
	Diff   string `json:"diff"`
	MoveTo string `json:"move_to,omitempty"`
	Before string `json:"before_sha256,omitempty"`
}

func ValidPatchPath(path string) bool {
	if path == "." || len(path) > 1024 || !fs.ValidPath(path) || !ValidApprovalCommand(path) || strings.ContainsAny(path, "\\:") {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if strings.EqualFold(part, ".git") || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false
		}
	}
	return true
}

func ValidNativePatch(files []NativePatchFile, secrets []string) bool {
	if len(files) == 0 || len(files) > 32 {
		return false
	}
	var size int
	seen := map[string]bool{}
	for _, f := range files {
		switch f.Op {
		case "add":
			if f.Before != "" || f.MoveTo != "" {
				return false
			}
		case "update", "delete":
			if len(f.Before) != 64 || strings.Trim(f.Before, "abcdef0123456789") != "" || (f.MoveTo != "" && f.Op != "update") {
				return false
			}
		default:
			return false
		}
		for _, path := range []string{f.Path, f.MoveTo} {
			if path == "" && f.Path != "" {
				continue
			}
			key := strings.ToLower(path)
			if !ValidPatchPath(path) || seen[key] {
				return false
			}
			seen[key] = true
		}
		size += len(f.Diff) + len(f.Path) + len(f.MoveTo)
		if size > 32<<10 || !utf8.ValidString(f.Diff) {
			return false
		}
		text := strings.ReplaceAll(f.Path+"\n"+f.MoveTo+"\n"+f.Diff+"\n", "\r\n", "\n")
		if strings.IndexFunc(text, func(r rune) bool {
			return unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r)
		}) >= 0 ||
			validation.RedactedLog([]byte(text), secrets) != text {
			return false
		}
	}
	return true
}

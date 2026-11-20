package skilllib

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Serve exposes one read-only MCP tool scoped to immutable packages chosen by the controller.
func Serve(in io.Reader, out io.Writer, root string, keys []string) error {
	if !filepath.IsAbs(root) || len(keys) == 0 {
		return ErrPackage
	}
	var dirs []string
	for _, key := range keys {
		if _, err := Dir(key); err != nil {
			return err
		}
		d, err := filepath.EvalSymlinks(filepath.Join(root, key))
		if err != nil {
			return err
		}
		dirs = append(dirs, d)
	}
	enc := json.NewEncoder(out)
	scan := bufio.NewScanner(in)
	scan.Buffer(make([]byte, 4096), 1<<20)
	for scan.Scan() {
		var r struct {
			ID     json.RawMessage
			Method string
			Params struct {
				ProtocolVersion string
				Name            string
				Arguments       struct {
					Path   string
					Offset int64
				}
			}
		}
		if err := json.Unmarshal(scan.Bytes(), &r); err != nil {
			return err
		}
		if len(r.ID) == 0 {
			continue
		}
		response := map[string]any{"jsonrpc": "2.0", "id": r.ID}
		var result any
		switch r.Method {
		case "initialize":
			version := r.Params.ProtocolVersion
			switch version {
			case "2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25":
			default:
				version = "2025-06-18"
			}
			result = map[string]any{"protocolVersion": version, "serverInfo": map[string]string{"name": "forgeos_skills", "version": "1.0"}, "capabilities": map[string]any{"tools": map[string]any{}}}
		case "ping":
			result = map[string]any{}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "read", "description": "Read a selected ForgeOS skill file by absolute path, or list a directory in its package. Reads up to 64 KiB starting at a byte offset.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"path": map[string]string{"type": "string"}, "offset": map[string]any{"type": "integer", "minimum": 0}}, "required": []string{"path"}}, "annotations": map[string]any{"readOnlyHint": true, "destructiveHint": false, "openWorldHint": false}}}}
		case "tools/call":
			if r.Params.Name != "read" {
				response["error"] = map[string]any{"code": -32602, "message": "unknown tool"}
				break
			}
			text, err := readSelected(dirs, r.Params.Arguments.Path, r.Params.Arguments.Offset)
			if err != nil {
				text = err.Error()
			}
			result = map[string]any{"isError": err != nil, "content": []any{map[string]string{"type": "text", "text": text}}}
		default:
			response["error"] = map[string]any{"code": -32601, "message": "method not found"}
		}
		if result != nil {
			response["result"] = result
		}
		if err := enc.Encode(response); err != nil {
			return err
		}
	}
	return scan.Err()
}
func readSelected(dirs []string, path string, offset int64) (string, error) {
	denied := errors.New("select a file or directory inside an assigned Skill")
	if !filepath.IsAbs(path) || offset < 0 {
		return "", denied
	}
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", denied
	}
	allowed := false
	for _, dir := range dirs {
		rel, err := filepath.Rel(dir, path)
		if err == nil && (rel == "." || validPath(filepath.ToSlash(rel))) {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", denied
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", denied
	}
	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return "", err
		}
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return strings.Join(names, "\n"), nil
	}
	if !info.Mode().IsRegular() {
		return "", denied
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return "", err
	}
	raw, err := io.ReadAll(io.LimitReader(f, 64<<10))
	if err != nil {
		return "", err
	}
	if !utf8.Valid(raw) {
		return strings.ToValidUTF8(string(raw), "�"), nil
	}
	return string(raw), nil
}

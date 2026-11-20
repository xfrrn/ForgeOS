// Package skilllib stores immutable, content-addressed Skill directories.
package skilllib

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

const MaxBytes = 32 << 20
const MaxFiles = 1024

type File struct {
	Path string
	Data []byte
}
type Package struct {
	Name, Description, Key, Entry string
	Count                         int
}

var ErrPackage = errors.New("请选择包含根目录 SKILL.md 的 Skill 文件夹。文件名应能跨平台使用，整个目录最多 1024 个文件、32 MiB。")

func Root() (string, error) {
	if root := os.Getenv("FORGEOS_SKILLS_DIR"); root != "" {
		if !filepath.IsAbs(root) {
			return "", errors.New("FORGEOS_SKILLS_DIR must be absolute")
		}
		return filepath.Clean(root), nil
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "ForgeOS", "skills"), nil
}
func Dir(key string) (string, error) {
	b, err := hex.DecodeString(key)
	if err != nil || len(b) != sha256.Size || key != strings.ToLower(key) {
		return "", ErrPackage
	}
	root, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, key), nil
}

func validPath(path string) bool {
	if !fs.ValidPath(path) || path == "." || !filepath.IsLocal(filepath.FromSlash(path)) || strings.ContainsAny(path, `\:*?"<>|`) || strings.IndexFunc(path, unicode.IsControl) >= 0 {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || len(part) > 255 {
			return false
		}
		base, _, _ := strings.Cut(strings.ToUpper(part), ".")
		switch base {
		case "CON", "NUL", "PRN", "AUX", "CONIN$", "CONOUT$":
			return false
		}
		if len(base) == 4 && (base[:3] == "COM" || base[:3] == "LPT") && base[3] >= '0' && base[3] <= '9' {
			return false
		}
	}
	return true
}

func Publish(files []File) (Package, error) {
	var pkg Package
	if len(files) == 0 || len(files) > MaxFiles {
		return pkg, ErrPackage
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	hash := sha256.New()
	var total int
	names := map[string]string{}
	dirs := map[string]bool{}
	for _, f := range files {
		total += len(f.Data)
		if total > MaxBytes || !validPath(f.Path) {
			return pkg, ErrPackage
		}
		parts := strings.Split(f.Path, "/")
		for i := range parts {
			name := strings.Join(parts[:i+1], "/")
			key := strings.ToLower(name)
			isDir := i+1 < len(parts)
			if prior, exists := names[key]; exists && (prior != name || !dirs[key] || !isDir) {
				return pkg, ErrPackage
			}
			names[key], dirs[key] = name, isDir
		}
		for _, data := range [][]byte{[]byte(f.Path), f.Data} {
			_ = binary.Write(hash, binary.BigEndian, uint64(len(data)))
			hash.Write(data)
		}
		if f.Path == "SKILL.md" {
			var err error
			pkg.Name, pkg.Description, err = metadata(f.Data)
			if err != nil {
				return pkg, err
			}
		}
	}
	if pkg.Name == "" {
		return pkg, ErrPackage
	}
	pkg.Key, pkg.Count = hex.EncodeToString(hash.Sum(nil)), len(files)
	dir, err := Dir(pkg.Key)
	if err != nil {
		return pkg, err
	}
	pkg.Entry = filepath.Join(dir, "SKILL.md")
	root := filepath.Dir(dir)
	if err = os.MkdirAll(root, 0700); err != nil {
		return pkg, err
	}
	stage, err := os.MkdirTemp(root, ".import-")
	if err != nil {
		return pkg, err
	}
	defer os.RemoveAll(stage)
	for _, f := range files {
		dest := filepath.Join(stage, filepath.FromSlash(f.Path))
		if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return pkg, err
		}
		if err = os.WriteFile(dest, f.Data, 0600); err != nil {
			return pkg, err
		}
	}
	if err = os.Rename(stage, dir); err != nil {
		// A repeat import or a concurrent publisher may have installed the same package.
		for _, f := range files {
			data, readErr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f.Path)))
			if readErr != nil || !bytes.Equal(data, f.Data) {
				return pkg, fmt.Errorf("publish skill package: %w", err)
			}
		}
	}
	return pkg, nil
}

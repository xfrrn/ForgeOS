package localrepo

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const rubyString = `(?:'[^'\\\r\n]*'|"[^"\\\r\n]*")`
const rubyAtom = `(?:` + rubyString + `|:[a-zA-Z_][a-zA-Z0-9_]*|true|false|nil)`
const rubyValue = `(?:` + rubyAtom + `|\[\s*(?:` + rubyAtom + `(?:\s*,\s*` + rubyAtom + `)*)?\s*\])`
const rubyArgument = `(?:(?:[a-zA-Z_]\w*:\s*|:[a-zA-Z_]\w*\s*=>\s*))?` + rubyValue
const rubyArguments = rubyArgument + `(?:\s*,\s*` + rubyArgument + `)*`

// Match entire literal calls. Unknown Ruby can change how all subsequent lines are parsed.
var rubyPlainArgs = regexp.MustCompile(`^(?:` + rubyArguments + `)?$`)
var rubyGemName = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
var rubyLockedDep = regexp.MustCompile(`^  ([a-zA-Z0-9._-]+)(?: \([^()]+\))?!?$`)

// Strip a comment only outside a complete literal. Escapes and interpolation are left unresolved.
func rubyStatement(line string) (string, bool) {
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			} else if c == '\\' || (quote == '"' && c == '#' && i+1 < len(line) && strings.ContainsRune("{@$", rune(line[i+1]))) {
				return "", false
			}
		} else if c == '#' {
			return strings.TrimSpace(line[:i]), true
		} else if c == '\'' || c == '"' {
			quote = c
		}
	}
	return strings.TrimSpace(line), quote == 0
}

func (f *InventoryFile) readGemfile(data []byte, deps map[string]bool) {
	depth := 0
	unresolved := 0
	for _, raw := range strings.Split(string(data), "\n") {
		line, ok := rubyStatement(strings.TrimSuffix(raw, "\r"))
		if !ok {
			f.Problem = "dynamic_ruby"
			return
		}
		if line == "" {
			continue
		}
		if line == "end" {
			depth--
			if depth < 0 {
				f.Problem = "invalid_manifest"
				return
			}
			continue
		}
		// Accept both gem "name" and gem("name") without interpreting Ruby.
		nameEnd := strings.IndexAny(line, " (\t")
		name, args := line, ""
		if nameEnd >= 0 {
			name, args = line[:nameEnd], strings.TrimSpace(line[nameEnd:])
		}
		block := strings.HasSuffix(args, " do")
		if block {
			args = strings.TrimSpace(strings.TrimSuffix(args, " do"))
		}
		if strings.HasPrefix(args, "(") && strings.HasSuffix(args, ")") {
			args = strings.TrimSpace(args[1 : len(args)-1])
		}
		if !rubyPlainArgs.MatchString(args) {
			f.Problem = "dynamic_ruby"
			return
		}
		switch name {
		case "gem":
			if block || len(args) < 2 || (args[0] != '\'' && args[0] != '"') {
				f.Problem = "dynamic_ruby"
				return
			}
			end := strings.IndexByte(args[1:], args[0])
			if end < 0 || !rubyGemName.MatchString(args[1:1+end]) {
				f.Problem = "invalid_manifest"
				return
			}
			deps[args[1:1+end]] = true
		case "gemspec", "eval_gemfile":
			if block {
				f.Problem = "dynamic_ruby"
				return
			}
			unresolved++
		case "source", "group", "platforms", "platform", "git", "path", "ruby":
			if args == "" {
				f.Problem = "invalid_manifest"
				return
			}
		default:
			f.Problem = "dynamic_ruby"
			return
		}
		if block {
			depth++
		}
	}
	if depth != 0 {
		f.Problem = "invalid_manifest"
		return
	}
	f.UnresolvedInputs = unresolved
}

func (f *InventoryFile) readRubyLock(data []byte, deps map[string]bool) {
	found, reading := false, false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "DEPENDENCIES" {
			if found {
				f.Problem = "invalid_manifest"
				return
			}
			found, reading = true, true
			continue
		}
		if line == "" {
			continue
		}
		if line[0] != ' ' && line[0] != '\t' {
			reading = false
		}
		if !reading {
			continue
		}
		dep := rubyLockedDep.FindStringSubmatch(line)
		if dep == nil {
			f.Problem = "invalid_manifest"
			return
		}
		deps[dep[1]] = true
	}
	if !found {
		f.Problem = "invalid_manifest"
	}
}

func (f *InventoryFile) readRuby(data []byte, deps map[string]bool) {
	if !utf8.Valid(data) {
		f.Problem = "invalid_manifest"
		return
	}
	if f.Kind == "ruby-lock" {
		f.readRubyLock(data, deps)
	} else {
		f.readGemfile(data, deps)
	}
}

package localrepo

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

var gradleConfiguration = regexp.MustCompile(`^(?:api|implementation|compileOnly(?:Api)?|runtimeOnly|annotationProcessor|[a-z][a-zA-Z0-9]*(?:Api|Implementation|CompileOnly(?:Api)?|RuntimeOnly|AnnotationProcessor)|kapt(?:[A-Z]\w*)?|ksp(?:[A-Z]\w*)?)$`)
var gradleIdentifier = regexp.MustCompile(`^[a-zA-Z_]\w*$`)
var gradleAccessor = regexp.MustCompile(`^libs(?:\.[a-zA-Z_]\w*)+$`)
var gradleProjectPath = regexp.MustCompile(`^(?::[a-zA-Z0-9_.-]+)+$`)
var gradleSimpleInterpolation = regexp.MustCompile(`^[a-zA-Z_]\w*(?:\.\w+)*$`)

// Tokenize literals and comments before looking for blocks. Unrecognized string
// syntax makes the whole file unresolved rather than exposing its contents as declarations.
func gradleTokens(s string, kotlin bool) ([]string, bool) {
	if !utf8.ValidString(s) {
		return nil, false
	}
	var tokens []string
	for i := 0; i < len(s); {
		start := i
		c := s[i]
		i++
		switch {
		case c == ' ' || c == '\t' || c == '\r':
			continue
		case strings.HasPrefix(s[start:], "//"):
			if n := strings.IndexByte(s[i:], '\n'); n >= 0 {
				i += n
			} else {
				i = len(s)
			}
			continue
		case strings.HasPrefix(s[start:], "/*"):
			i++
			depth := 1
			for i < len(s) && depth > 0 {
				if strings.HasPrefix(s[i:], "*/") {
					i += 2
					depth--
				} else if kotlin && strings.HasPrefix(s[i:], "/*") {
					i += 2
					depth++
				} else {
					i++
				}
			}
			if depth != 0 {
				return nil, false
			}
			continue
		case c == 96 && kotlin:
			end := strings.IndexByte(s[i:], 96)
			if end < 0 || strings.ContainsAny(s[i:i+end], "\r\n") {
				return nil, false
			}
			tokens = append(tokens, "<quoted identifier>")
			i += end + 1
			continue
		case (c == '/' && !kotlin) || c == 96:
			return nil, false
		case c == '"' || c == '\'':
			delimiter := s[start:i]
			triple := strings.HasPrefix(s[start:], strings.Repeat(delimiter, 3))
			if triple {
				delimiter = strings.Repeat(delimiter, 3)
				i += 2
			}
			for {
				if i >= len(s) {
					return nil, false
				}
				if strings.HasPrefix(s[i:], delimiter) {
					i += len(delimiter)
					break
				}
				if s[i] == '\\' && !(kotlin && triple) {
					i += 2
					continue
				}
				if c == '"' && strings.HasPrefix(s[i:], "${") {
					n := strings.IndexByte(s[i+2:], '}')
					if n < 0 || !gradleSimpleInterpolation.MatchString(s[i+2:i+2+n]) {
						return nil, false
					}
					i += n + 3
					continue
				}
				i++
			}
			if triple {
				tokens = append(tokens, "<multiline>")
				continue
			}
		case gradleWordByte(c):
			for i < len(s) && gradleWordByte(s[i]) {
				i++
			}
		}
		tokens = append(tokens, s[start:i])
	}
	return tokens, true
}

func gradleWordByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '.' || c == '$'
}

func gradleLiteral(token string) string {
	if len(token) < 2 || (token[0] != '"' && token[0] != '\'') || strings.ContainsAny(token, "\\\r\n") {
		return ""
	}
	return token[1 : len(token)-1]
}
func gradleArgs(parts []string) []string {
	if len(parts) >= 2 && parts[0] == "(" && parts[len(parts)-1] == ")" {
		return parts[1 : len(parts)-1]
	}
	return parts
}

func gradleModule(parts []string) string {
	if len(parts) == 1 {
		if gradleAccessor.MatchString(parts[0]) {
			return parts[0]
		}
		fields := strings.Split(gradleLiteral(parts[0]), ":")
		if len(fields) >= 2 && len(fields) <= 4 && mavenName.MatchString(fields[0]) && mavenName.MatchString(fields[1]) {
			return fields[0] + ":" + fields[1]
		}
		return ""
	}
	// Groovy map notation and Kotlin named arguments use the same fields.
	values := map[string]string{}
	for len(parts) >= 3 {
		key := parts[0]
		if (key != "group" && key != "name" && key != "version" && key != "classifier" && key != "ext") || (parts[1] != ":" && parts[1] != "=") {
			return ""
		}
		if _, exists := values[key]; exists {
			return ""
		}
		values[key] = gradleLiteral(parts[2])
		parts = parts[3:]
		if len(parts) == 0 {
			break
		}
		if parts[0] != "," || len(parts) == 1 {
			return ""
		}
		parts = parts[1:]
	}
	if len(parts) == 0 && mavenName.MatchString(values["group"]) && mavenName.MatchString(values["name"]) {
		return values["group"] + ":" + values["name"]
	}
	return ""
}

func gradleDeclaration(stmt []string) (string, string) {
	scope := stmt[0]
	args := gradleArgs(stmt[1:])
	if scope == "add" && len(args) >= 3 && args[1] == "," {
		scope = gradleLiteral(args[0])
		args = args[2:]
		if !gradleIdentifier.MatchString(scope) {
			return "", ""
		}
	} else if !gradleConfiguration.MatchString(scope) {
		return "", ""
	}
	if len(args) > 1 {
		switch args[0] {
		case "platform", "enforcedPlatform", "testFixtures":
			if mod := gradleModule(gradleArgs(args[1:])); mod != "" {
				return scope, args[0] + "(" + mod + ")"
			}
			return "", ""
		case "project":
			a := gradleArgs(args[1:])
			if len(a) == 1 {
				p := gradleLiteral(a[0])
				if gradleProjectPath.MatchString(p) {
					return scope, "project(" + p + ")"
				}
			}
			return "", ""
		}
	}
	return scope, gradleModule(args)
}

// A control header on its own line still governs the following statement.
func gradleControlHeader(stmt []string) bool {
	if len(stmt) == 1 {
		return stmt[0] == "else" || stmt[0] == "do"
	}
	if len(stmt) < 3 || stmt[1] != "(" {
		return false
	}
	switch stmt[0] {
	case "if", "for", "while", "when", "switch":
	default:
		return false
	}
	level := 0
	for i, token := range stmt[1:] {
		switch token {
		case "(":
			level++
		case ")":
			level--
			if level == 0 {
				return i == len(stmt)-2
			}
		}
	}
	return false
}

func (f *InventoryFile) readGradleScript(data []byte) {
	tokens, ok := gradleTokens(string(data), strings.HasSuffix(f.Path, ".kts"))
	if !ok {
		f.Problem = "dynamic_gradle"
		return
	}
	configs := map[string][]string{}
	var stmt []string
	depth, parens, unresolved := 0, 0, 0
	active := false
	record := func() {
		if active && depth == 1 && len(stmt) > 0 {
			scope, dep := gradleDeclaration(stmt)
			if dep == "" {
				unresolved++
			} else {
				configs[scope] = append(configs[scope], dep)
			}
		}
		stmt = nil
	}
	for _, tok := range tokens {
		switch tok {
		case "{":
			dependencies := len(stmt) == 1 && stmt[0] == "dependencies"
			if depth == 0 && !dependencies && slices.Contains(stmt, "dependencies") {
				unresolved++
			}
			record()
			if dependencies {
				if depth == 0 {
					active = true
				} else {
					unresolved++
				}
			}
			depth++
		case "}":
			record()
			depth--
			if depth < 0 {
				f.Problem = "invalid_manifest"
				return
			}
			if depth == 0 {
				active = false
			}
		case "\n", ";":
			if parens == 0 && (tok == ";" || !gradleControlHeader(stmt)) {
				record()
			}
		default:
			switch tok {
			case "(", "[":
				parens++
			case ")", "]":
				parens--
			}
			if parens < 0 {
				f.Problem = "invalid_manifest"
				return
			}
			stmt = append(stmt, tok)
		}
	}
	if depth != 0 || parens != 0 {
		f.Problem = "invalid_manifest"
		return
	}
	for scope, items := range configs {
		slices.Sort(items)
		configs[scope] = slices.Compact(items)
	}
	f.GradleConfigurations, f.UnresolvedInputs = configs, unresolved
}

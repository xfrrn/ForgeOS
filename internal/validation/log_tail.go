package validation

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const LiveLogWindow = 8 << 10

var sensitiveLine = regexp.MustCompile(`(?i)(authorization|proxy-authorization|cookie|password|passwd|secret|token|api[-_ ]?key|private[-_ ]?key|bearer\s+|://[^/\s]+:[^/\s]+@|gh[pousr]_[a-z0-9]+|github_pat_|sk-[a-z0-9_-]+|AKIA[A-Z0-9]{16}|eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+|-----BEGIN [A-Z ]*CREDENTIAL)`)

func RedactedLog(data []byte, secrets []string) string {
	// Structured paths may quote known credentials; match their escaped forms too.
	matches := make([]string, 0, len(secrets)*2)
	for _, secret := range secrets {
		matches = append(matches, secret)
		quoted := strconv.Quote(secret)
		if quoted[1:len(quoted)-1] != secret {
			matches = append(matches, quoted[1:len(quoted)-1])
		}
	}
	// Only publish complete lines. Classify them before cutting the display window.
	end := bytes.LastIndexByte(data, '\n')
	if end < 0 {
		return ""
	}
	var text strings.Builder
	privateKey := false
	for pos := 0; pos <= end; {
		n := bytes.IndexByte(data[pos:], '\n')
		raw := data[pos : pos+n]
		pos += n + 1
		line := strings.TrimSuffix(string(raw), "\r")
		if strings.Contains(line, "-----BEGIN ") && strings.Contains(line, "PRIVATE KEY-----") {
			privateKey = true
		}
		hidden := privateKey || !utf8.ValidString(line) || strings.IndexFunc(line, func(r rune) bool { return unicode.IsControl(r) && r != '\t' }) >= 0 || sensitiveLine.MatchString(line)
		if !hidden {
			for _, secret := range matches {
				if secret != "" && strings.Contains(line, secret) {
					hidden = true
					break
				}
			}
		}
		if strings.Contains(line, "-----END ") && strings.Contains(line, "PRIVATE KEY-----") {
			privateKey = false
		}
		if hidden {
			text.WriteString("[redacted]")
		} else {
			text.WriteString(line)
		}
		text.WriteByte('\n')
	}
	return text.String()
}

func DisplayLogTail(data []byte, secrets []string) string {
	s := RedactedLog(data, secrets)
	if len(s) > LiveLogWindow {
		s = s[len(s)-LiveLogWindow:]
		for !utf8.RuneStart(s[0]) {
			s = s[1:]
		}
	}
	return s
}

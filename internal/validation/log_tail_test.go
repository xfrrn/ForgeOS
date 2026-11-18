package validation

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRedactedLogHidesQuotedKnownCredentials(t *testing.T) {
	for _, secret := range []string{"embedded\\separator", "embedded\"quote", "embedded\nline"} {
		raw := "add " + strconv.Quote("docs/"+secret+".md") + "\n"
		if got := RedactedLog([]byte(raw), []string{secret}); got != "[redacted]\n" {
			t.Fatal("quoted credential exposed")
		}
	}
}

func TestLiveLogTail(t *testing.T) {
	data := "compiling\r\nAuthorization: Bearer example\n单元测试\n-----BEGIN PRIVATE KEY-----\nprivate body\n-----END PRIVATE KEY-----\ncustom-value\nclean\npartial"
	got := DisplayLogTail([]byte(data), []string{"custom-value"})
	want := "compiling\n[redacted]\n单元测试\n[redacted]\n[redacted]\n[redacted]\n[redacted]\nclean\n"
	if got != want {
		t.Fatalf("tail = %q", got)
	}
	long := DisplayLogTail([]byte(strings.Repeat("文", LiveLogWindow)+"\nend\n"), nil)
	if len(long) > LiveLogWindow || !utf8.ValidString(long) || !strings.HasSuffix(long, "\nend\n") {
		t.Fatal("invalid display window")
	}
	if got := DisplayLogTail([]byte("a\x00b\nunfinished"), nil); got != "[redacted]\n" {
		t.Fatal(got)
	}
	if got := DisplayLogTail([]byte("line without newline"), nil); got != "" {
		t.Fatal(got)
	}
}

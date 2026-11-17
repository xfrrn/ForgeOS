package localrepo

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDocumentPartsPreserveByteOffsets(t *testing.T) {
	// A multi-byte rune straddles 64 KiB, then CRLF and a long final line cross two more windows.
	body := strings.Repeat("a", (64<<10)-1) + "界" + strings.Repeat("乙", 21000) + "\r\n" + strings.Repeat("Ω", 48000)
	var joined strings.Builder
	count := 0
	out, err := splitDocument(strings.NewReader(body), func(part int, at int64, text string) error {
		count++
		if part != count || at != int64(joined.Len()) || !utf8.ValidString(text) || len(text) > 64<<10 {
			t.Fatal(part, at, len(text))
		}
		joined.WriteString(text)
		return nil
	})
	if err != nil || out.Problem != "" || joined.String() != body || out.Count != count || out.Bytes != int64(len(body)) || out.Digest != fmt.Sprintf("%x", sha256.Sum256([]byte(body))) {
		t.Fatal(out, err)
	}
}

package main

import "testing"

func TestChatPreviewSecretAcrossChunks(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"answer sk-t", "answer "}, {"answer sk-test", "answer [已隐藏]"}, {"answer sk-test hello", "answer [已隐藏] hello"},
	} {
		if got := redactChatPreview(tc.input, []string{"sk-test"}); got != tc.want {
			t.Errorf("unexpected preview %q", got)
		}
	}
}

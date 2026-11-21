package chatkernel

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestAnswerPrefix(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{`{"act`, ""},
		{`{"action":{"answer":"hidden"},"citations":["answer"],"answer":"你`, "你"},
		{`{"answer":"一\n二\"三\\`, "一\n二\"三\\"},
		{`{"answer":"一\`, "一"},
		{`{"answer":"x\u4f`, "x"},
		{`{"answer":"x\ud83d\ude`, "x"},
		{`{"answer":"x\ud83d\ude03","citations":[]}`, "x😃"},
		{`{"answer":null}`, ""},
	} {
		if got := answerPrefix(tc.raw); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestChatCompletionStream(t *testing.T) {
	var wire strings.Builder
	send := func(delta obj, finish string) {
		b, _ := json.Marshal(obj{"choices": []obj{{"index": 0, "delta": delta, "finish_reason": finish}}})
		fmt.Fprintf(&wire, "data: %s\r\n\r\n", b)
	}
	send(obj{"content": `{"answer":"first`}, "")
	send(obj{"content": ` second","citations":[],"action":null}`}, "stop")
	wire.WriteString("data: [DONE]\n\n")
	var seen []string
	reply, err := readCompletionStream(strings.NewReader(wire.String()), func(s string) { seen = append(seen, s) })
	if err != nil || len(seen) == 0 || seen[len(seen)-1] != "first second" || reply.Choices[0].Finish != "stop" {
		t.Fatalf("preview=%q err=%v", seen, err)
	}
	wire.Reset()
	send(obj{"tool_calls": []obj{{"index": 0, "id": "abc", "function": obj{"name": "read", "arguments": `{"p":`}}}}, "")
	send(obj{"tool_calls": []obj{{"index": 0, "function": obj{"arguments": `1}`}}}}, "tool_calls")
	reply, err = readCompletionStream(strings.NewReader(wire.String()), func(string) { t.Fatal("tool arguments shown in preview") })
	if err != nil {
		t.Fatal(err)
	}
	call := reply.Choices[0].Message.Calls[0]
	if call.ID != "abc" || call.Function.Name != "read" || call.Function.Arguments != `{"p":1}` {
		t.Fatalf("incomplete call: %+v", call)
	}
	wire.Reset()
	send(obj{"content": `{"answer":"partial`}, "")
	if _, err = readCompletionStream(strings.NewReader(wire.String()), nil); err == nil {
		t.Fatal("accepted interrupted completion")
	}
}

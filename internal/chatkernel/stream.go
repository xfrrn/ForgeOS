package chatkernel

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
)

type streamChunk struct {
	Error   json.RawMessage
	Usage   *tokenUsage
	Choices []struct {
		Index  int
		Finish string `json:"finish_reason"`
		Delta  struct {
			Content string
			Calls   []struct {
				Index int
				functionCall
			} `json:"tool_calls"`
		}
	}
}

// The provider streams the structured JSON document, including tool arguments and
// action proposals. Only the decoded top-level answer string belongs in the UI.
func answerPrefix(document string) string {
	dec := json.NewDecoder(strings.NewReader(document))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return ""
	}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return ""
		}
		if key != "answer" {
			var skip json.RawMessage
			if dec.Decode(&skip) != nil {
				return ""
			}
			continue
		}
		rest := strings.TrimLeft(document[dec.InputOffset():], " \n\r\t")
		if !strings.HasPrefix(rest, ":") {
			return ""
		}
		value := strings.TrimLeft(rest[1:], " \n\r\t")
		if len(value) == 0 || value[0] != '"' {
			return ""
		}
		end := 1
		for end < len(value) && value[end] != '"' {
			if value[end] != '\\' {
				end++
				continue
			}
			if end+1 >= len(value) {
				break
			}
			if value[end+1] != 'u' {
				end += 2
				continue
			}
			if end+6 > len(value) {
				break
			}
			code, err := strconv.ParseUint(value[end+2:end+6], 16, 16)
			if err != nil {
				return ""
			}
			if code >= 0xd800 && code <= 0xdbff {
				if end+12 > len(value) {
					break
				}
				if strings.HasPrefix(value[end+6:], `\u`) {
					end += 12
					continue
				}
			}
			end += 6
		}
		var answer string
		if json.Unmarshal([]byte(value[:end]+`"`), &answer) != nil {
			return ""
		}
		return answer
	}
	return ""
}

// Coalesce bursts but also deliver the last chunk when a provider pauses mid-answer.
// The network reader can keep consuming tool and model frames while a preview is sent.
func answerPreviews(publish func(string)) (chan string, func()) {
	if publish == nil {
		return nil, func() {}
	}
	updates := make(chan string, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		var last string
		for doc := range updates {
			answer := answerPrefix(doc)
			if answer != "" && answer != last {
				publish(answer)
				last = answer
			}
			time.Sleep(80 * time.Millisecond)
		}
	}()
	return updates, func() { close(updates); <-done }
}

func readCompletionStream(r io.Reader, preview func(string)) (completion, error) {
	reply := completion{Choices: []completionChoice{{}}}
	choice := &reply.Choices[0]
	var content strings.Builder
	var event bytes.Buffer
	updates, finishPreviews := answerPreviews(preview)
	defer finishPreviews()
	// Bound framing overhead as well as the unwrapped completion.
	scan := bufio.NewScanner(io.LimitReader(r, 8<<20))
	scan.Buffer(make([]byte, 4096), 2<<20)
	consume := func() (bool, error) {
		data := bytes.TrimSpace(event.Bytes())
		defer event.Reset()
		if len(data) == 0 {
			return false, nil
		}
		if bytes.Equal(data, []byte("[DONE]")) {
			return true, nil
		}
		var chunk streamChunk
		if json.Unmarshal(data, &chunk) != nil || (len(chunk.Error) > 0 && string(chunk.Error) != "null") {
			return false, errors.New("invalid project model stream")
		}
		if chunk.Usage != nil {
			reply.Usage = *chunk.Usage
		}
		for _, part := range chunk.Choices {
			if part.Index != 0 {
				return false, errors.New("unexpected completion choice")
			}
			if part.Finish != "" {
				choice.Finish = part.Finish
			}
			content.WriteString(part.Delta.Content)
			if content.Len() > 2<<20 {
				return false, errors.New("project model response too long")
			}
			for _, piece := range part.Delta.Calls {
				if piece.Index < 0 || piece.Index > 63 {
					return false, errors.New("invalid streamed tool index")
				}
				for len(choice.Message.Calls) <= piece.Index {
					choice.Message.Calls = append(choice.Message.Calls, functionCall{})
				}
				call := &choice.Message.Calls[piece.Index]
				call.ID += piece.ID
				call.Function.Name += piece.Function.Name
				call.Function.Arguments += piece.Function.Arguments
				if len(call.Function.Arguments) > 256<<10 {
					return false, errors.New("project tool arguments too long")
				}
			}
			if updates != nil && len(choice.Message.Calls) == 0 && part.Delta.Content != "" && content.Len() <= 64<<10 {
				select {
				case <-updates:
				default:
				}
				updates <- content.String()
			}
		}
		return false, nil
	}
	for scan.Scan() {
		line := scan.Text()
		if line == "" {
			done, err := consume()
			if err != nil {
				return reply, err
			}
			if done {
				break
			}
		} else if data, ok := strings.CutPrefix(line, "data:"); ok {
			event.WriteString(strings.TrimPrefix(data, " "))
			event.WriteByte('\n')
			if event.Len() > 2<<20 {
				return reply, errors.New("project model event too long")
			}
		}
	}
	if err := scan.Err(); err != nil {
		return reply, errors.New("project model stream interrupted")
	}
	if event.Len() > 0 {
		if _, err := consume(); err != nil {
			return reply, err
		}
	}
	choice.Message.Content = content.String()
	if choice.Finish == "" {
		return reply, errors.New("incomplete project model stream")
	}
	return reply, nil
}

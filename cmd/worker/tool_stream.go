package main

import (
	"strings"

	"forgeos/internal/validation"
)

// Keep incomplete lines in memory: a later chunk may identify a secret on that line.
type toolStream struct {
	pending    string
	text       string
	archive    string
	discard    bool
	revision   int64
	privateKey bool
}

func (p *processLog) commandOutput(id, delta string) error {
	if !p.toolStream || delta == "" {
		return nil
	}
	call, ok := p.tools.pending[id]
	if !ok || call.tool != "command_execution" {
		return nil
	}
	if call.stream == nil {
		if _, err := p.redactOutput(p.toolOutput, "tool", ""); err != nil {
			return err
		}
		call.stream = &toolStream{privateKey: p.toolOutput.privateKey}
		p.tools.pending[id] = call
	}
	before := call.stream.revision
	if err := call.stream.append(p, delta, false); err != nil {
		return err
	}
	if call.stream.revision == before {
		// A crash can lose a split key delimiter. Persist uncertainty without its raw bytes.
		tx, err := p.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err = p.saveOutputState(tx, "tool", p.toolKeyOpen()); err != nil {
			return err
		}
		return tx.Commit()
	}
	return p.recordTool(id, call.tool, "output", "")
}

func (s *toolStream) append(p *processLog, delta string, final bool) error {
	var archive strings.Builder
	defer func() { s.archive += archive.String() }()
	p.toolOutput.privateKey = p.toolOutput.privateKey || s.privateKey
	defer func() { s.privateKey = p.toolOutput.privateKey }()
	for delta != "" || final && s.pending != "" {
		part, rest, newline := strings.Cut(delta, "\n")
		delta = rest
		if !s.discard {
			if len(s.pending)+len(part) > 64<<10 {
				// An oversized line may hide a key boundary. Mask until a later explicit END.
				if _, err := p.redactOutput(p.toolOutput, "tool", ""); err != nil {
					return err
				}
				p.toolOutput.privateKey = true
				s.revision++
				s.text = validation.DisplayLogTail([]byte(s.text+"[redacted]\n"), nil)
				archive.WriteString("[redacted]\n")
				s.pending, s.discard = "", true
			} else {
				s.pending += part
			}
		}
		if newline || final {
			if !s.discard {
				safe, err := p.redactOutput(p.toolOutput, "tool", strings.TrimSuffix(s.pending, "\r"))
				if err != nil {
					return err
				}
				s.text = validation.DisplayLogTail([]byte(s.text+safe), nil)
				archive.WriteString(safe)
				s.revision++
			}
			s.pending, s.discard = "", false
		}
	}
	return nil
}

func (p *processLog) toolKeyOpen() bool {
	if p.toolOutput.privateKey {
		return true
	}
	for _, call := range p.tools.pending {
		if call.stream != nil && (call.stream.privateKey || call.stream.pending != "") {
			return true
		}
	}
	return false
}

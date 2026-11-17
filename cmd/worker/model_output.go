package main

import (
	"encoding/json"
	"os"
	"slices"
	"strings"

	"forgeos/internal/store"
	"forgeos/internal/validation"
)

type modelOutput struct {
	store.ModelOutput
	secrets    []string
	credential func() (string, error)
	privateKey bool
}

func outputSecrets(lease string) []string {
	values := []string{lease}
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		if secretVariable.MatchString(key) && value != "" {
			values = append(values, value)
		}
	}
	return values
}

func (p *processLog) modelMessage(text string) error {
	if p.output == nil || text == "" {
		return nil
	}
	o := p.output
	token, err := o.credential()
	if err != nil {
		return err
	}
	if token != "" && !slices.Contains(o.secrets, token) {
		o.secrets = append(o.secrets, token)
	}
	// Preserve private-key redaction across separate public messages; never journal the raw text.
	if o.privateKey {
		text = "-----BEGIN PRIVATE KEY-----\n" + text
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "-----BEGIN ") && strings.Contains(line, "PRIVATE KEY-----") {
			o.privateKey = true
		}
		if strings.Contains(line, "-----END ") && strings.Contains(line, "PRIVATE KEY-----") {
			o.privateKey = false
		}
	}
	safe := validation.DisplayLogTail([]byte(text+"\n"), o.secrets)
	o.Text = validation.DisplayLogTail([]byte(o.Text+safe), o.secrets)
	o.Sequence++
	_, err = p.db.Exec(`INSERT INTO harness_model_output(instance,sequence,text) VALUES(?,?,?)
	 ON CONFLICT(instance) DO UPDATE SET sequence=excluded.sequence,text=excluded.text`, p.id, o.Sequence, o.Text)
	if err == nil {
		p.notify()
	}
	return err
}

func (p *processLog) claudeActivity(kind string, raw []byte) error {
	if kind == "assistant" && p.output != nil {
		var event struct {
			Message struct{ Content []struct{ Type, Text string } }
		}
		if err := json.Unmarshal(raw, &event); err != nil {
			return err
		}
		for _, block := range event.Message.Content {
			if block.Type == "text" {
				if err := p.modelMessage(block.Text); err != nil {
					return err
				}
			}
		}
	}
	return p.claudeTool(kind, raw)
}

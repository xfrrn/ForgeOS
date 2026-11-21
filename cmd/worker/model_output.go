package main

import (
	"database/sql"
	"encoding/json"
	"errors"
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
	loaded     bool
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
	safe, err := p.redactOutput(o, "model", text)
	if err != nil {
		return err
	}
	o.Text = validation.DisplayLogTail([]byte(o.Text+safe), o.secrets)
	o.Sequence++
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO harness_model_output(instance,sequence,text) VALUES(?,?,?)
	 ON CONFLICT(instance) DO UPDATE SET sequence=excluded.sequence,text=excluded.text`, p.id, o.Sequence, o.Text)
	if err == nil {
		err = p.archiveOutput(tx, "model", 0, "", "message", safe)
	}
	if err == nil {
		err = p.saveOutputState(tx, "model", o.privateKey)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err == nil {
		p.notify()
	}
	return err
}

func (p *processLog) redactOutput(o *modelOutput, stream, text string) (string, error) {
	if !o.loaded {
		err := p.db.QueryRow("SELECT private_key FROM harness_output_redaction WHERE lease_hash=? AND stream=?", p.lease[:], stream).Scan(&o.privateKey)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		o.loaded = true
	}
	return o.redact(text)
}

func (p *processLog) saveOutputState(tx *sql.Tx, stream string, open bool) error {
	_, err := tx.Exec(`INSERT INTO harness_output_redaction(lease_hash,stream,private_key) VALUES(?,?,?)
	 ON CONFLICT(lease_hash,stream) DO UPDATE SET private_key=excluded.private_key`, p.lease[:], stream, open)
	return err
}

func (o *modelOutput) redact(text string) (string, error) {
	if o.credential != nil {
		token, err := o.credential()
		if err != nil {
			return "", err
		}
		if token != "" && !slices.Contains(o.secrets, token) {
			o.secrets = append(o.secrets, token)
		}
	}
	// Preserve private-key redaction across output records; never journal the raw text.
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
	return validation.RedactedLog([]byte(text+"\n"), o.secrets), nil
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

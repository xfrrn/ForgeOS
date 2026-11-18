package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os/exec"
	"sync/atomic"
	"time"
)

// A stream belongs to one CLI process and carries exactly one user turn.
type claudeControl struct {
	input        io.WriteCloser
	active       atomic.Bool
	requested    atomic.Bool
	acknowledged bool
	ended        bool
}

const claudeInterruptID = "forgeos-interrupt"

func (c *claudeControl) response(raw []byte) error {
	var response struct {
		Response struct {
			Subtype string
			ID      string `json:"request_id"`
		}
	}
	if !c.requested.Load() || c.acknowledged || json.Unmarshal(raw, &response) != nil ||
		response.Response.ID != claudeInterruptID || response.Response.Subtype != "success" {
		return errors.New("unexpected Claude interrupt response")
	}
	c.acknowledged = true
	if c.ended {
		return c.input.Close()
	}
	return nil
}

func (c *claudeControl) end() {
	c.active.Store(false)
	c.ended = true
	if !c.requested.Load() || c.acknowledged {
		_ = c.input.Close()
	}
}

func (c *claudeControl) watch(ctx context.Context, cmd *exec.Cmd, output io.Closer) func() {
	finished, stopped := make(chan struct{}), make(chan struct{})
	force := func() {
		_ = cmd.Cancel()
		_ = c.input.Close()
		_ = output.Close()
	}
	stop := context.AfterFunc(ctx, func() {
		defer close(stopped)
		killed := make(chan struct{})
		timer := time.AfterFunc(5*time.Second, func() {
			force()
			close(killed)
		})
		defer func() {
			if !timer.Stop() {
				<-killed
			}
		}()
		if !c.active.Load() {
			force()
			return
		}
		c.requested.Store(true)
		err := json.NewEncoder(c.input).Encode(map[string]any{"type": "control_request", "request_id": claudeInterruptID,
			"request": map[string]string{"subtype": "interrupt"}})
		if err != nil {
			force()
			return
		}
		<-finished
	})
	return func() {
		close(finished)
		if !stop() {
			<-stopped
		}
		if c.acknowledged && c.ended {
			log.Printf("Claude process %d acknowledged native interrupt and reported its terminal result", cmd.Process.Pid)
		}
	}
}

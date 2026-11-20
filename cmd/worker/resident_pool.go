package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"forgeos/internal/store"
)

// The supervisor owns claims and recovery discovery. Residents share the node identity and journal,
// so the control plane reserves task, model and CI capacity once for the entire host.
type residentPool struct {
	initial int
	policy  atomic.Pointer[store.FixedLimits]
	online  atomic.Int32
	mu      sync.Mutex
	members map[int]*residentMember
}
type residentMember struct {
	client   *rpc.Client
	stop     context.CancelFunc
	busy     int
	draining bool
}

func (p *residentPool) supervise(ctx context.Context, journal string) {
	var children sync.WaitGroup
	defer children.Wait()
	for ctx.Err() == nil {
		target := p.policy.Load().Workers
		p.mu.Lock()
		for id, m := range p.members {
			if id >= target && m.busy == 0 && !m.draining {
				m.draining = true
				m.stop()
			}
		}
		// Stagger startup and failed-start retries. A retiring process keeps its slot until Wait returns.
		if len(p.members) < target {
			id := 0
			for p.members[id] != nil {
				id++
			}
			child, stop := context.WithCancel(ctx)
			m := &residentMember{stop: stop}
			p.members[id] = m
			children.Add(1)
			go func() {
				defer children.Done()
				defer stop()
				if err := p.runMember(child, journal, m); err != nil && child.Err() == nil {
					log.Printf("resident Worker %d exited: %v", id+1, err)
				}
				p.mu.Lock()
				delete(p.members, id)
				p.mu.Unlock()
			}()
		}
		p.mu.Unlock()
		wait(ctx)
	}
}

type rpcPipes struct {
	io.Reader
	io.WriteCloser
}

func (p *residentPool) runMember(ctx context.Context, journal string, m *residentMember) error {
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, bin, "resident-work", journal)
	cmd.Stderr = os.Stderr
	input, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer input.Close()
	output, writer, err := os.Pipe()
	if err != nil {
		return err
	}
	defer output.Close()
	cmd.Stdout = writer
	cmd.Cancel = input.Close
	cmd.WaitDelay = 15 * time.Second
	err = cmd.Start()
	writer.Close()
	if err != nil {
		return err
	}
	client := jsonrpc.NewClient(rpcPipes{output, input})
	defer client.Close()
	// Fail a wedged initialization without publishing the child as available.
	timeout := time.AfterFunc(time.Minute, func() { _ = cmd.Process.Kill() })
	var ready bool
	err = client.Call("Worker.Ready", true, &ready)
	timeout.Stop()
	if err != nil || !ready {
		input.Close()
		_ = cmd.Wait()
		return errors.New("resident Worker initialization failed")
	}
	p.mu.Lock()
	m.client = client
	p.online.Add(1)
	p.mu.Unlock()
	log.Printf("resident Worker ready pid=%d", cmd.Process.Pid)
	err = cmd.Wait()
	p.mu.Lock()
	m.client = nil
	p.online.Add(-1)
	p.mu.Unlock()
	return err
}

func (p *residentPool) execute(ctx context.Context, work poolWork) error {
	for ctx.Err() == nil {
		p.mu.Lock()
		var selected *residentMember
		for id, m := range p.members {
			if id < p.policy.Load().Workers && !m.draining && m.client != nil && (selected == nil || m.busy < selected.busy) {
				selected = m
			}
		}
		if selected != nil {
			selected.busy++
			client := selected.client
			p.mu.Unlock()
			var complete bool
			err := client.Call("Worker.Execute", work, &complete)
			p.mu.Lock()
			selected.busy--
			p.mu.Unlock()
			return err
		}
		p.mu.Unlock()
		wait(ctx)
	}
	return ctx.Err()
}

type ResidentJob poolWork
type residentExecutor struct {
	ctx    context.Context
	worker *worker
}

func (r *residentExecutor) Ready(_ bool, ready *bool) error { *ready = true; return nil }
func (r *residentExecutor) Execute(job ResidentJob, complete *bool) error {
	if job.Follow <= 0 && (job.Job.ExecutionID <= 0 || job.Job.LeaseToken == "") {
		return errors.New("missing resident assignment")
	}
	err := poolWork(job).run(r.ctx, r.worker)
	*complete = err == nil
	return err
}

type parentInput struct {
	reader io.Reader
	cancel context.CancelFunc
}

func (p parentInput) Read(buf []byte) (int, error) {
	n, err := p.reader.Read(buf)
	if err != nil {
		p.cancel()
	}
	return n, err
}

func serveResident(parent context.Context, journal string) error {
	w, err := fromEnv("")
	if err != nil {
		return err
	}
	w.journal, err = openExistingJournal(journal, true)
	if err != nil {
		return err
	}
	defer w.journal.Close()
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	server := rpc.NewServer()
	if err := server.RegisterName("Worker", &residentExecutor{ctx, w}); err != nil {
		return err
	}
	server.ServeCodec(jsonrpc.NewServerCodec(rpcPipes{parentInput{os.Stdin, cancel}, os.Stdout}))
	return nil
}

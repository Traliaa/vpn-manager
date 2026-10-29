// Package managedprocess owns and reaps one long-lived child, never unrelated processes.
package managedprocess

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type child struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error // Read only after done closes.
}

type Process struct {
	mu    sync.Mutex
	child *child
}

func (p *Process) Running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return running(p.child)
}

func running(c *child) bool {
	if c == nil {
		return false
	}
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

// Start detaches service lifetime from the caller's request. The context bounds
// readiness only. Cancellation or timeout before readiness always reaps the child.
func (p *Process) Start(ctx context.Context, binary string, args []string, ready func() bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if running(p.child) {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c := &child{cmd: exec.Command(binary, args...), done: make(chan struct{})}
	// Output is intentionally not returned: clients may echo key material on errors.
	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("start service process: %w", err)
	}
	p.child = c
	go func() { c.err = c.cmd.Wait(); close(c.done) }()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	settled := time.NewTimer(200 * time.Millisecond)
	defer settled.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	stable := false
	for {
		select {
		case <-c.done:
			return fmt.Errorf("service process exited during startup: %v", c.err)
		case <-ctx.Done():
			_ = stop(c)
			return ctx.Err()
		case <-deadline.C:
			_ = stop(c)
			return fmt.Errorf("service process readiness timed out")
		case <-settled.C:
			stable = true
		case <-ticker.C:
		}
		if stable && (ready == nil || ready()) {
			return nil
		}
	}
}

func (p *Process) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Cleanup must also work when the request that started it has been cancelled.
	return stop(p.child)
}

func stop(c *child) error {
	if !running(c) {
		return nil
	}
	if err := c.cmd.Process.Signal(syscall.SIGTERM); err != nil && running(c) {
		_ = c.cmd.Process.Kill()
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-c.done:
		return nil
	case <-timer.C:
		if err := c.cmd.Process.Kill(); err != nil && running(c) {
			return err
		}
		<-c.done
		return nil
	}
}

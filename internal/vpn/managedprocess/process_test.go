package managedprocess

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func script(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "child")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestStartRejectsChildThatExits(t *testing.T) {
	var p Process
	if err := p.Start(context.Background(), script(t, "exit 7"), nil, nil); err == nil {
		t.Fatal("reported exited child as ready")
	}
	if p.Running() {
		t.Fatal("exited child reported running")
	}
}
func TestRequestCancellationDoesNotKillOwnedChild(t *testing.T) {
	var p Process
	ctx, cancel := context.WithCancel(context.Background())
	if err := p.Start(ctx, script(t, "exec sleep 60"), nil, nil); err != nil {
		t.Fatal(err)
	}
	defer p.Stop(context.Background())
	cancel()
	if !p.Running() {
		t.Fatal("request cancellation killed service")
	}
	if err := p.Start(context.Background(), "/missing", nil, nil); err != nil {
		t.Fatalf("repeated start not idempotent: %v", err)
	}
	if err := p.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.Running() {
		t.Fatal("child survived stop")
	}
	if err := p.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func TestReadinessCancellationCleansUp(t *testing.T) {
	var p Process
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := p.Start(ctx, script(t, "exec sleep 60"), nil, func() bool { return false }); err == nil {
		t.Fatal("unready child accepted")
	}
	if p.Running() {
		t.Fatal("unready child leaked")
	}
}

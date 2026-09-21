package platform

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunSubprocessSucceedsWhenAGrandchildOutlivesTheChild(t *testing.T) {
	script := filepath.Join(t.TempDir(), "leaky")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho done\nsleep 5 &\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	out, err := RunSubprocess(t.Context(), script)
	if err != nil || string(out) != "done\n" {
		t.Fatalf("RunSubprocess = (%q, %v), want the child's own exit status", out, err)
	}
}

func TestRunSubprocessReturnsWhenAGrandchildHoldsThePipe(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "holder")
	mark := filepath.Join(dir, "spawned")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 5 &\n: > \"$SUBPROCESS_TEST_MARK\"\nwait\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("SUBPROCESS_TEST_MARK", mark)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := RunSubprocess(ctx, script)
		done <- err
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(mark); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the grandchild was never spawned")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	start := time.Now()
	if err := <-done; err == nil {
		t.Fatal("canceled subprocess returned no error")
	}
	if elapsed := time.Since(start); elapsed > subprocessWaitDelay+time.Second {
		t.Fatalf("RunSubprocess took %s after the cancel waiting on the grandchild's pipe", elapsed)
	}
}

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const (
	pidFile     = "/run/cocoon-net.pid"
	pidFilePerm = 0o644
	pidDirPerm  = 0o755
)

// acquirePIDFile writes the current PID to /run/cocoon-net.pid and fails
// if another instance is already running.
func acquirePIDFile() error {
	if err := checkExistingPID(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(pidFile), pidDirPerm); err != nil {
		return fmt.Errorf("create pid dir: %w", err)
	}
	return os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), pidFilePerm)
}

// checkExistingPID returns nil when it's safe to (re)write the PID file:
// no file, corrupt file, or stale file (process dead). It returns an
// error only if another live cocoon-net daemon still owns the PID.
func checkExistingPID() error {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return nil // no PID file, safe to proceed
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return nil // corrupt PID file, overwrite
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	if proc.Signal(syscall.Signal(0)) == nil {
		return fmt.Errorf("another cocoon-net daemon is running (pid %d)", pid)
	}
	return nil // stale PID, process dead
}

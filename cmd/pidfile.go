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

func acquirePIDFile() error {
	if err := checkExistingPID(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(pidFile), pidDirPerm); err != nil {
		return fmt.Errorf("create pid dir: %w", err)
	}
	return os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), pidFilePerm)
}

// A missing, corrupt, or stale (process dead) PID file is safe to overwrite.
func checkExistingPID() error {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return nil
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return nil
	}
	proc, _ := os.FindProcess(pid)
	if proc.Signal(syscall.Signal(0)) == nil {
		return fmt.Errorf("another cocoon-net daemon is running (pid %d)", pid)
	}
	return nil
}

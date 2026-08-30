package cmd

import (
	"errors"
	"fmt"
	"io/fs"
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
	if err := os.MkdirAll(filepath.Dir(pidFile), pidDirPerm); err != nil {
		return fmt.Errorf("create pid dir: %w", err)
	}
	tmp := fmt.Sprintf("%s.%d", pidFile, os.Getpid())
	if err := os.WriteFile(tmp, []byte(strconv.Itoa(os.Getpid())), pidFilePerm); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}
	defer func() { _ = os.Remove(tmp) }()
	for range 2 {
		err := os.Link(tmp, pidFile)
		if err == nil {
			return nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("create pid file: %w", err)
		}
		if err := checkExistingPID(); err != nil {
			return err
		}
		_ = os.Remove(pidFile)
	}
	return fmt.Errorf("create pid file: %w", fs.ErrExist)
}

// a missing, corrupt, or stale (process dead) PID file is safe to overwrite
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

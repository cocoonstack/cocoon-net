package dhcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	leasePathPrefix          = "/v1/leases/"
	controlReadHeaderTimeout = 5 * time.Second
	controlShutdownTimeout   = 5 * time.Second
)

type controlServer struct {
	server     *http.Server
	listener   net.Listener
	socketPath string
}

func newControlServer(socketPath string, leases *Server) (*controlServer, error) {
	if socketPath == "" {
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o750); err != nil {
		return nil, fmt.Errorf("create socket directory: %w", err)
	}
	if info, err := os.Lstat(socketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("refusing to replace non-socket path %s", socketPath)
		}
		if removeErr := os.Remove(socketPath); removeErr != nil {
			return nil, fmt.Errorf("remove stale socket: %w", removeErr)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect socket path: %w", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", socketPath, err)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(socketPath)
		return nil, fmt.Errorf("chmod socket: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc(leasePathPrefix, leases.handleLeaseControl)
	return &controlServer{
		server: &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: controlReadHeaderTimeout,
		},
		listener:   listener,
		socketPath: socketPath,
	}, nil
}

func (s *controlServer) serve() <-chan error {
	errCh := make(chan error, 1)
	go func() {
		err := s.server.Serve(s.listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()
	return errCh
}

func (s *controlServer) close() {
	ctx, cancel := context.WithTimeout(context.Background(), controlShutdownTimeout)
	defer cancel()
	_ = s.server.Shutdown(ctx)
	_ = os.Remove(s.socketPath)
}

func (s *Server) handleLeaseControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rawMAC, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, leasePathPrefix))
	if err != nil || rawMAC == "" || strings.Contains(rawMAC, "/") {
		http.Error(w, "invalid MAC address", http.StatusBadRequest)
		return
	}
	mac, err := net.ParseMAC(rawMAC)
	if err != nil {
		http.Error(w, "invalid MAC address", http.StatusBadRequest)
		return
	}
	if _, err := s.ReleaseLease(r.Context(), mac); err != nil {
		http.Error(w, "lease release failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

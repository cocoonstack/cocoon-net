package dhcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLeaseControlRejectsInvalidRequests(t *testing.T) {
	srv := New(Config{}, nil)

	for _, tt := range []struct {
		name   string
		method string
		path   string
		status int
	}{
		{name: "method", method: http.MethodGet, path: leasePathPrefix + "aa:bb:cc:dd:ee:ff", status: http.StatusMethodNotAllowed},
		{name: "missing mac", method: http.MethodDelete, path: leasePathPrefix, status: http.StatusBadRequest},
		{name: "invalid mac", method: http.MethodDelete, path: leasePathPrefix + "not-a-mac", status: http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			res := httptest.NewRecorder()
			srv.handleLeaseControl(res, req)
			if res.Code != tt.status {
				t.Errorf("status = %d, want %d", res.Code, tt.status)
			}
		})
	}
}

func TestLeaseControlMissingLeaseIsIdempotent(t *testing.T) {
	srv := New(Config{}, nil)
	req := httptest.NewRequest(http.MethodDelete, leasePathPrefix+"aa:bb:cc:dd:ee:ff", nil)
	res := httptest.NewRecorder()

	srv.handleLeaseControl(res, req)

	if res.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", res.Code, http.StatusNoContent)
	}
}

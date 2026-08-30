package dhcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLeaseControlRoutes(t *testing.T) {
	srv := New(Config{}, nil)
	mux := newControlMux(srv)

	for _, tt := range []struct {
		name   string
		method string
		path   string
		status int
	}{
		{name: "method", method: http.MethodGet, path: "/v1/leases/aa:bb:cc:dd:ee:ff", status: http.StatusMethodNotAllowed},
		{name: "missing mac", method: http.MethodDelete, path: "/v1/leases/", status: http.StatusNotFound},
		{name: "invalid mac", method: http.MethodDelete, path: "/v1/leases/not-a-mac", status: http.StatusBadRequest},
		{name: "escaped mac", method: http.MethodDelete, path: "/v1/leases/aa%3Abb%3Acc%3Add%3Aee%3Aff", status: http.StatusNoContent},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			res := httptest.NewRecorder()
			mux.ServeHTTP(res, req)
			if res.Code != tt.status {
				t.Errorf("status = %d, want %d", res.Code, tt.status)
			}
		})
	}
}

func TestLeaseControlMissingLeaseIsIdempotent(t *testing.T) {
	srv := New(Config{}, nil)
	req := httptest.NewRequest(http.MethodDelete, "/v1/leases/aa:bb:cc:dd:ee:ff", nil)
	res := httptest.NewRecorder()

	newControlMux(srv).ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", res.Code, http.StatusNoContent)
	}
}

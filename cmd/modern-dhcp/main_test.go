package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"modern-dhcp/pkg/models"
)

func TestRunLeaseReleaseSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		expectedPath := "/api/v1/tenants/tenant-1/leases/lease-123/release"
		if r.URL.Path != expectedPath {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && err != io.EOF {
			t.Fatalf("unexpected body decode error: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(models.Lease{ID: "lease-123", TenantID: "tenant-1", IPAddress: "10.0.0.5", State: "RELEASED"})
	}))
	defer ts.Close()

	err := run([]string{"leases", "release", "--tenant", "tenant-1", "--lease", "lease-123", "--url", ts.URL})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
}

func TestRunLeaseReleaseError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer ts.Close()

	err := run([]string{"leases", "release", "--tenant", "tenant-1", "--lease", "missing", "--url", ts.URL})
	if err == nil {
		t.Fatalf("expected error but got nil")
	}
}

package ops

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestTransferManagerExport(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	opts := ImportExportOptions{
		Enabled:     true,
		StoragePath: tmp,
		ObjectStore: "s3://example-bucket/import-export",
		Formats:     []string{"csv", "json"},
	}
	mgr, err := newTransferManager(opts, zap.NewNop())
	if err != nil {
		t.Fatalf("newTransferManager: %v", err)
	}
	defer mgr.Close()

	job, err := mgr.StartTransfer(context.Background(), TransferRequest{
		Kind:        TransferKindExport,
		Resource:    "leases",
		Format:      "csv",
		RequestedBy: "ops-admin",
		Metadata: map[string]string{
			"tenant": "tenant-acme",
		},
	})
	if err != nil {
		t.Fatalf("StartTransfer export: %v", err)
	}

	waitForTransferStatus(t, mgr, job.ID, TransferStatusSucceeded)

	stored, err := mgr.Get(job.ID)
	if err != nil {
		t.Fatalf("Get job: %v", err)
	}
	if stored.ArtifactPath == "" {
		t.Fatalf("expected artifact path")
	}
	if _, err := os.Stat(stored.ArtifactPath); err != nil {
		t.Fatalf("artifact missing: %v", err)
	}
	if stored.TargetURI == "" {
		t.Fatalf("expected target URI")
	}
	if stored.SizeBytes == 0 {
		t.Fatalf("expected non-zero artifact size")
	}
}

func TestTransferManagerImport(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	sourceFile := filepath.Join(tmp, "import.csv")
	data := []byte("id,name\n1,alpha\n")
	if err := os.WriteFile(sourceFile, data, 0o600); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	opts := ImportExportOptions{
		Enabled:     true,
		StoragePath: tmp,
		Formats:     []string{"csv"},
		MaxFileSize: int64(len(data) + 10),
	}
	mgr, err := newTransferManager(opts, zap.NewNop())
	if err != nil {
		t.Fatalf("newTransferManager: %v", err)
	}
	defer mgr.Close()

	job, err := mgr.StartTransfer(context.Background(), TransferRequest{
		Kind:        TransferKindImport,
		Resource:    "pools",
		RequestedBy: "ops-admin",
		SourceURI:   sourceFile,
	})
	if err != nil {
		t.Fatalf("StartTransfer import: %v", err)
	}

	waitForTransferStatus(t, mgr, job.ID, TransferStatusSucceeded)

	stored, err := mgr.Get(job.ID)
	if err != nil {
		t.Fatalf("Get job: %v", err)
	}
	if stored.ArtifactPath == "" {
		t.Fatalf("expected artifact path")
	}
	if stored.ArtifactPath == sourceFile {
		t.Fatalf("artifact should differ from source path")
	}
	if _, err := os.Stat(stored.ArtifactPath); err != nil {
		t.Fatalf("copied artifact missing: %v", err)
	}
	if stored.SizeBytes == 0 {
		t.Fatalf("expected size bytes to be recorded")
	}
}

func waitForTransferStatus(t *testing.T, mgr *transferManager, jobID string, desired TransferStatus) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, err := mgr.Get(jobID)
		if err == nil && job.Status == desired {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	job, err := mgr.Get(jobID)
	if err != nil {
		t.Fatalf("job not found: %v", err)
	}
	t.Fatalf("expected status %s, got %s (%s)", desired, job.Status, job.Error)
}

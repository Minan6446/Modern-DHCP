package reporting

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"time"

	"modern-dhcp/pkg/models"
)

// Renderer renders structured reports into deliverable artifacts.
type Renderer interface {
	RenderLeaseHistory(ctx context.Context, tenantID string, leases []models.Lease, format Format) (Artifact, error)
}

// CSVRenderer renders reports as CSV payloads.
type CSVRenderer struct{}

// NewCSVRenderer constructs a renderer that emits CSV artifacts.
func NewCSVRenderer() *CSVRenderer {
	return &CSVRenderer{}
}

// RenderLeaseHistory implements Renderer for lease history exports.
func (CSVRenderer) RenderLeaseHistory(ctx context.Context, tenantID string, leases []models.Lease, format Format) (Artifact, error) {
	if format == FormatPDF {
		return Artifact{}, fmt.Errorf("renderer: format %s not supported", format)
	}
	buf := bytes.NewBuffer(nil)
	writer := csv.NewWriter(buf)
	header := []string{"leaseId", "tenantId", "poolId", "ipAddress", "identifier", "clientId", "state", "securityState", "updatedAt", "expiresAt"}
	if err := writer.Write(header); err != nil {
		return Artifact{}, err
	}
	for _, lease := range leases {
		record := []string{
			lease.ID,
			lease.TenantID,
			lease.PoolID,
			lease.IPAddress,
			lease.HardwareAddr,
			lease.ClientID,
			lease.State,
			lease.SecurityState,
			lease.UpdatedAt.Format(time.RFC3339),
			lease.ExpiresAt.Format(time.RFC3339),
		}
		if err := writer.Write(record); err != nil {
			return Artifact{}, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return Artifact{}, err
	}
	now := time.Now().UTC()
	artifact := Artifact{
		Name:        fmt.Sprintf("lease-history-%s-%s.csv", tenantID, now.Format("20060102-150405")),
		Format:      FormatCSV,
		ContentType: "text/csv",
		Data:        buf.Bytes(),
		Size:        buf.Len(),
		GeneratedAt: now,
	}
	return artifact, nil
}

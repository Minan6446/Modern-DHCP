package observability

import (
	"errors"
	"time"

	"modern-dhcp/internal/metrics"
	"modern-dhcp/internal/pool"
	"modern-dhcp/pkg/models"
	"modern-dhcp/pkg/telemetry"
)

// SelectorDimensions encapsulates label-ready selector metadata fields.
type SelectorDimensions struct {
	Scope     string
	VLAN      string
	Interface string
	SSID      string
	Location  string
}

// DimensionsFromResolution derives the label set from the resolved pool (if any).
func DimensionsFromResolution(request pool.MetadataSelector, resolved *models.AddressPool) SelectorDimensions {
	dims := SelectorDimensions{
		Scope:     telemetry.ScopeLabel(""),
		VLAN:      telemetry.VLANLabel(request.VLANID),
		Interface: telemetry.StringLabel(request.InterfaceID),
		SSID:      telemetry.StringLabel(request.SSID),
		Location:  telemetry.StringLabel(request.Location),
	}
	if resolved != nil {
		dims.Scope = telemetry.ScopeLabel(resolved.Scope)
		dims.VLAN = telemetry.OptionalVLANLabel(resolved.VLANID)
		dims.Interface = telemetry.OptionalStringLabel(resolved.InterfaceID)
		dims.SSID = telemetry.OptionalStringLabel(resolved.SSID)
		dims.Location = telemetry.OptionalStringLabel(resolved.Location)
	}
	return dims
}

// MetadataSnapshotLabels returns the ordered label values for the metadata gauge.
func MetadataSnapshotLabels(tenantID string, poolObj *models.AddressPool) []string {
	return []string{
		tenantID,
		poolObj.ID,
		telemetry.ScopeLabel(poolObj.Scope),
		telemetry.OptionalVLANLabel(poolObj.VLANID),
		telemetry.OptionalStringLabel(poolObj.InterfaceID),
		telemetry.OptionalStringLabel(poolObj.SSID),
		telemetry.OptionalStringLabel(poolObj.Location),
	}
}

// ClassifyResolveError maps resolve errors to the configured Prometheus label values.
func ClassifyResolveError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, pool.ErrPoolNotFound):
		return "not_found"
	case errors.Is(err, pool.ErrInvalidScope), errors.Is(err, pool.ErrInvalidCIDR), errors.Is(err, pool.ErrInvalidRange):
		return "validation"
	default:
		return "repository"
	}
}

// ObserveSelectorMetrics updates all selector-related collectors.
func ObserveSelectorMetrics(collector *metrics.Collector, tenantID string, selector pool.MetadataSelector, resolved *models.AddressPool, err error, started time.Time) {
	if collector == nil {
		return
	}
	dims := DimensionsFromResolution(selector, resolved)
	resolvedOK := err == nil && resolved != nil
	resultLabel := telemetry.ResultLabel(resolvedOK)
	collector.PoolSelectorResolutions.WithLabelValues(
		tenantID,
		dims.Scope,
		dims.VLAN,
		dims.Interface,
		dims.SSID,
		dims.Location,
		resultLabel,
	).Inc()
	collector.PoolSelectorLatency.WithLabelValues(tenantID, resultLabel).Observe(time.Since(started).Seconds())
	if err != nil {
		collector.PoolSelectorErrors.WithLabelValues(tenantID, ClassifyResolveError(err)).Inc()
	}
	if resolved != nil {
		collector.PoolMetadataSnapshot.WithLabelValues(MetadataSnapshotLabels(tenantID, resolved)...).Set(1)
	}
}

package pool

import (
	"context"
	"testing"

	"modern-dhcp/pkg/models"
)

func TestResolveMetadataVLANRequired(t *testing.T) {
	svc := &Service{}
	if _, err := svc.resolveMetadata(scopeVLAN, PoolCreateRequest{}, nil); err != ErrVLANRequired {
		t.Fatalf("expected ErrVLANRequired, got %v", err)
	}
}

func TestResolveMetadataVLANExplicitValue(t *testing.T) {
	svc := &Service{}
	parentVLAN := 100
	parent := &models.AddressPool{VLANID: &parentVLAN}
	childVLAN := 200
	meta, err := svc.resolveMetadata(scopeVLAN, PoolCreateRequest{VLANID: &childVLAN}, parent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.vlanID == nil || *meta.vlanID != childVLAN {
		t.Fatalf("expected explicit vlan %d, got %#v", childVLAN, meta.vlanID)
	}
}

func TestResolveMetadataPortRequirements(t *testing.T) {
	svc := &Service{}
	parentVLAN := 10
	parent := &models.AddressPool{VLANID: &parentVLAN}

	if _, err := svc.resolveMetadata(scopePort, PoolCreateRequest{}, parent); err != ErrEndpointMetadata {
		t.Fatalf("expected ErrEndpointMetadata, got %v", err)
	}

	iface := " Gi1/0/1 "
	meta, err := svc.resolveMetadata(scopePort, PoolCreateRequest{InterfaceID: &iface}, parent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.vlanID == nil || *meta.vlanID != parentVLAN {
		t.Fatalf("expected vlan inheritance %d, got %#v", parentVLAN, meta.vlanID)
	}
	if meta.interfaceID == nil || *meta.interfaceID != "Gi1/0/1" {
		t.Fatalf("expected trimmed interface id, got %#v", meta.interfaceID)
	}
}

func TestResolveMetadataPortRejectsVLANOverride(t *testing.T) {
	svc := &Service{}
	parentVLAN := 20
	parent := &models.AddressPool{VLANID: &parentVLAN}
	override := 21
	iface := "xe-0/0/1"
	if _, err := svc.resolveMetadata(scopePort, PoolCreateRequest{VLANID: &override, InterfaceID: &iface}, parent); err != ErrParentVLANMismatch {
		t.Fatalf("expected ErrParentVLANMismatch, got %v", err)
	}
}

func TestResolvePoolPrefersInterface(t *testing.T) {
	ifacePool := models.AddressPool{ID: "iface"}
	repo := &stubRepo{interfaceResult: ifacePool}
	svc := &Service{repo: repo}
	pool, err := svc.ResolvePool(context.Background(), "tenant", MetadataSelector{InterfaceID: " Gi1/0/1 "})
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if pool == nil || pool.ID != "iface" {
		t.Fatalf("expected iface pool, got %#v", pool)
	}
}

func TestResolvePoolFallsBackToVLAN(t *testing.T) {
	vlanPool := models.AddressPool{ID: "vlan"}
	repo := &stubRepo{vlanResult: vlanPool}
	svc := &Service{repo: repo}
	pool, err := svc.ResolvePool(context.Background(), "tenant", MetadataSelector{VLANID: 200})
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if pool == nil || pool.ID != "vlan" {
		t.Fatalf("expected vlan pool, got %#v", pool)
	}
}

func TestResolvePoolNotFound(t *testing.T) {
	repo := &stubRepo{}
	svc := &Service{repo: repo}
	if _, err := svc.ResolvePool(context.Background(), "tenant", MetadataSelector{VLANID: 10}); err != ErrPoolNotFound {
		t.Fatalf("expected ErrPoolNotFound, got %v", err)
	}
}

type stubRepo struct {
	interfaceResult models.AddressPool
	vlanResult      models.AddressPool
}

func (s *stubRepo) ListPools(ctx context.Context, tenantID string, limit, offset int) ([]models.AddressPool, error) {
	return nil, nil
}

func (s *stubRepo) CountPools(ctx context.Context, tenantID string) (int, error) {
	return 0, nil
}

func (s *stubRepo) GetPool(ctx context.Context, tenantID, poolID string) (*models.AddressPool, error) {
	return nil, nil
}

func (s *stubRepo) InsertPool(ctx context.Context, pool *models.AddressPool) error { return nil }

func (s *stubRepo) UpdatePool(ctx context.Context, pool *models.AddressPool) error { return nil }

func (s *stubRepo) DeletePool(ctx context.Context, tenantID, poolID string) error { return nil }

func (s *stubRepo) FindPools(ctx context.Context, tenantID string, filter MetadataFilter) ([]models.AddressPool, error) {
	if filter.InterfaceID != nil && *filter.InterfaceID == "Gi1/0/1" && s.interfaceResult.ID != "" {
		return []models.AddressPool{s.interfaceResult}, nil
	}
	if filter.VLANID != nil && *filter.VLANID == 200 && s.vlanResult.ID != "" {
		return []models.AddressPool{s.vlanResult}, nil
	}
	return nil, nil
}

func (s *stubRepo) ListBindings(ctx context.Context, tenantID string, limit, offset int) ([]models.StaticBinding, error) {
	return nil, nil
}

func (s *stubRepo) GetBinding(ctx context.Context, tenantID, bindingID string) (*models.StaticBinding, error) {
	return nil, nil
}

func (s *stubRepo) InsertBinding(ctx context.Context, binding *models.StaticBinding) error {
	return nil
}

func (s *stubRepo) UpdateBinding(ctx context.Context, binding *models.StaticBinding) error {
	return nil
}

func (s *stubRepo) DeleteBinding(ctx context.Context, tenantID, bindingID string) error { return nil }

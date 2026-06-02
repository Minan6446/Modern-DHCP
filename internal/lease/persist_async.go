package lease

import (
	"context"

	"go.uber.org/zap"

	"modern-dhcp/pkg/models"
)

func (s *Service) shouldAsyncPersistLease() bool {
	return s.bitmapEnabled() && s.replication == nil
}

func (s *Service) persistLeaseUpdate(ctx context.Context, leaseRecord *models.Lease, poolObj *models.AddressPool) error {
	if leaseRecord == nil {
		return nil
	}
	if !s.shouldAsyncPersistLease() {
		if err := s.repo.UpdateLease(ctx, leaseRecord); err != nil {
			return err
		}
		if err := s.confirmReplication(ctx, leaseRecord); err != nil {
			return err
		}
		return s.confirmSyncAck(ctx, leaseRecord)
	}
	copyLease := *leaseRecord
	go func(leaseCopy models.Lease) {
		if err := s.repo.UpdateLease(context.Background(), &leaseCopy); err != nil {
			if s.logger != nil {
				s.logger.Error("async lease update failed", zap.String("leaseId", leaseCopy.ID), zap.Error(err))
			}
		}
	}(copyLease)
	_ = poolObj
	return nil
}

func (s *Service) persistLeaseCreate(ctx context.Context, leaseRecord *models.Lease, poolObj *models.AddressPool) error {
	if leaseRecord == nil {
		return nil
	}
	if !s.shouldAsyncPersistLease() {
		if err := s.repo.CreateLease(ctx, leaseRecord); err != nil {
			return err
		}
		if err := s.confirmReplication(ctx, leaseRecord); err != nil {
			return err
		}
		return s.confirmSyncAck(ctx, leaseRecord)
	}
	copyLease := *leaseRecord
	go func(leaseCopy models.Lease, poolSnapshot *models.AddressPool) {
		if err := s.repo.CreateLease(context.Background(), &leaseCopy); err != nil {
			if s.logger != nil {
				s.logger.Error("async lease create failed", zap.String("leaseId", leaseCopy.ID), zap.Error(err))
			}
			if poolSnapshot != nil {
				_ = s.bitmapSetByIP(context.Background(), poolSnapshot, leaseCopy.IPAddress, true)
			}
		}
	}(copyLease, poolObj)
	return nil
}

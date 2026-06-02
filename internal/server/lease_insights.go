package server

import (
	"context"
	"encoding/binary"
	"math"
	"net/netip"
	"sort"
	"strings"
	"time"

	"net/http"

	"github.com/labstack/echo/v4"

	"modern-dhcp/internal/lease"
	"modern-dhcp/internal/pool"
	"modern-dhcp/pkg/models"
)

type dimensionCountResponse struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

type leaseRiskPoolResponse struct {
	PoolID        string  `json:"poolId"`
	PoolName      string  `json:"poolName"`
	TenantID      string  `json:"tenantId"`
	Utilization   float64 `json:"utilization"`
	ActiveLeases  int64   `json:"activeLeases"`
	ExhaustionETA string  `json:"exhaustionEta,omitempty"`
}

type leaseInsightsResponse struct {
	GeneratedAt           string                   `json:"generatedAt"`
	PressureIndex         float64                  `json:"pressureIndex"`
	RenewalRate           float64                  `json:"renewalRate"`
	AvgLeaseDurationHours float64                  `json:"avgLeaseDurationHours"`
	TodayNewLeases        int                      `json:"todayNewLeases"`
	ConflictIPs           int                      `json:"conflictIps"`
	ExpiringNext24h       int                      `json:"expiringNext24h"`
	Declines24h           int                      `json:"declines24h"`
	StateBreakdown        map[string]int64         `json:"stateBreakdown"`
	VendorMix             []dimensionCountResponse `json:"vendorMix"`
	PoolsAtRisk           []leaseRiskPoolResponse  `json:"poolsAtRisk"`
}

func (s *HTTPServer) handleLeaseInsights(leaseSvc *lease.Service, poolSvc *pool.Service) echo.HandlerFunc {
	if leaseSvc == nil {
		return func(c echo.Context) error {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "lease service disabled")
		}
	}
	return func(c echo.Context) error {
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			tenantID = systemTenantID
		}
		ctx := c.Request().Context()
		poolScope := s.poolScopeRef(c).WithTenantOverride(tenantID)
		leaseScope := s.leaseScopeRef(c).WithTenantOverride(tenantID)

		activeCount, err := leaseSvc.CountActiveLeases(ctx, leaseScope)
		if err != nil {
			if isMissingTableErr(err) {
				return c.JSON(http.StatusOK, leaseInsightsResponse{
					GeneratedAt:           time.Now().UTC().Format(time.RFC3339),
					PressureIndex:         0,
					RenewalRate:           0,
					AvgLeaseDurationHours: 0,
					TodayNewLeases:        0,
					ConflictIPs:           0,
					ExpiringNext24h:       0,
					Declines24h:           0,
					StateBreakdown:        map[string]int64{},
					VendorMix:             []dimensionCountResponse{},
					PoolsAtRisk:           []leaseRiskPoolResponse{},
				})
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}

		utcNow := time.Now().UTC()
		startOfDay := time.Date(utcNow.Year(), utcNow.Month(), utcNow.Day(), 0, 0, 0, 0, time.UTC)
		newLeasesToday, err := leaseSvc.CountLeasesCreatedSince(ctx, leaseScope, startOfDay)
		if err != nil {
			if isMissingTableErr(err) {
				newLeasesToday = 0
			} else {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}
		conflictSince := utcNow.Add(-24 * time.Hour)
		conflictIPs, err := leaseSvc.CountConflictLeasesSince(ctx, leaseScope, conflictSince)
		if err != nil {
			if isMissingTableErr(err) {
				conflictIPs = 0
			} else {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}
		avgLeaseHours, err := leaseSvc.AverageLeaseDurationHours(ctx, leaseScope)
		if err != nil {
			if isMissingTableErr(err) {
				avgLeaseHours = 0
			} else {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}

		stateBreakdown := map[string]int64{
			"ACTIVE":    int64(activeCount),
			"EXPIRED":   0,
			"RECLAIMED": 0,
		}

		vendorMix := make([]dimensionCountResponse, 0)
		if mix, err := leaseSvc.CountActiveLeasesByDeviceType(ctx, leaseScope); err == nil && len(mix) > 0 {
			vendorMix = make([]dimensionCountResponse, 0, len(mix))
			for key, count := range mix {
				normalized := strings.TrimSpace(key)
				if normalized == "" {
					normalized = "unknown"
				}
				vendorMix = append(vendorMix, dimensionCountResponse{Key: normalized, Count: count})
			}
			sort.SliceStable(vendorMix, func(i, j int) bool {
				if vendorMix[i].Count == vendorMix[j].Count {
					return vendorMix[i].Key < vendorMix[j].Key
				}
				return vendorMix[i].Count > vendorMix[j].Count
			})
		}

		poolsAtRisk := make([]leaseRiskPoolResponse, 0)
		maxUtilization := 0.0
		if poolSvc != nil {
			pools, err := listAllPools(ctx, poolSvc, poolScope, 200)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			if len(pools) > 0 {
				ids := make([]string, 0, len(pools))
				for _, poolObj := range pools {
					ids = append(ids, poolObj.ID)
				}
				counts, err := leaseSvc.CountActiveLeasesByPool(ctx, leaseScope, ids)
				if err != nil {
					return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
				}
				for _, poolObj := range pools {
					capacity := poolCapacity(poolObj)
					active := counts[poolObj.ID]
					utilization := 0.0
					if capacity > 0 {
						utilization = (float64(active) / float64(capacity)) * 100.0
					}
					if utilization > maxUtilization {
						maxUtilization = utilization
					}
					if utilization >= 80.0 {
						poolsAtRisk = append(poolsAtRisk, leaseRiskPoolResponse{
							PoolID:       poolObj.ID,
							PoolName:     poolObj.Name,
							TenantID:     tenantID,
							Utilization:  round(utilization, 1),
							ActiveLeases: active,
						})
					}
				}
				if len(poolsAtRisk) > 1 {
					sort.SliceStable(poolsAtRisk, func(i, j int) bool {
						if poolsAtRisk[i].Utilization == poolsAtRisk[j].Utilization {
							return poolsAtRisk[i].PoolName < poolsAtRisk[j].PoolName
						}
						return poolsAtRisk[i].Utilization > poolsAtRisk[j].Utilization
					})
					if len(poolsAtRisk) > 5 {
						poolsAtRisk = poolsAtRisk[:5]
					}
				}
			}
		}

		response := leaseInsightsResponse{
			GeneratedAt:           time.Now().UTC().Format(time.RFC3339),
			PressureIndex:         round(maxUtilization, 1),
			RenewalRate:           0,
			AvgLeaseDurationHours: round(avgLeaseHours, 2),
			TodayNewLeases:        newLeasesToday,
			ConflictIPs:           conflictIPs,
			ExpiringNext24h:       0,
			Declines24h:           0,
			StateBreakdown:        stateBreakdown,
			VendorMix:             vendorMix,
			PoolsAtRisk:           poolsAtRisk,
		}
		return c.JSON(http.StatusOK, response)
	}
}

func listAllPools(ctx context.Context, poolSvc *pool.Service, scopeRef pool.ResourceScope, pageSize int) ([]models.AddressPool, error) {
	if poolSvc == nil {
		return nil, nil
	}
	if pageSize <= 0 {
		pageSize = 200
	}
	offset := 0
	pools := make([]models.AddressPool, 0)
	for {
		batch, err := poolSvc.ListPools(ctx, scopeRef, pageSize, offset)
		if err != nil {
			return nil, err
		}
		pools = append(pools, batch...)
		if len(batch) < pageSize {
			break
		}
		offset += len(batch)
		if offset >= 2000 {
			break
		}
	}
	return pools, nil
}

func poolCapacity(pool models.AddressPool) int64 {
	start, errStart := netip.ParseAddr(pool.RangeStart)
	end, errEnd := netip.ParseAddr(pool.RangeEnd)
	if errStart != nil || errEnd != nil {
		return 0
	}
	if start.Compare(end) > 0 {
		return 0
	}
	if start.Is4() && end.Is4() {
		sBytes := start.As4()
		eBytes := end.As4()
		s := binary.BigEndian.Uint32(sBytes[:])
		e := binary.BigEndian.Uint32(eBytes[:])
		if e < s {
			return 0
		}
		return int64(e - s + 1)
	}
	return 0
}

func round(value float64, precision int) float64 {
	if precision <= 0 {
		return math.Round(value)
	}
	factor := math.Pow(10, float64(precision))
	return math.Round(value*factor) / factor
}

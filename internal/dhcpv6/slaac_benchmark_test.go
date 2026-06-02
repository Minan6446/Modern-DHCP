package dhcpv6

import (
	"net/netip"
	"strconv"
	"sync/atomic"
	"testing"
)

func resetSLAACHashCacheForBenchmark() {
	slaacHashCache.Range(func(key, _ any) bool {
		slaacHashCache.Delete(key)
		return true
	})
}

func BenchmarkDeriveSLAACAddress_CacheHit(b *testing.B) {
	resetSLAACHashCacheForBenchmark()
	prefix := netip.MustParsePrefix("2001:db8:1::/64")
	seed := "duid-0001|aa:bb:cc:dd:ee:ff|12345"
	if _, err := deriveSLAACAddress(prefix, seed); err != nil {
		b.Fatalf("warmup failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := deriveSLAACAddress(prefix, seed); err != nil {
			b.Fatalf("derive failed: %v", err)
		}
	}
}

func BenchmarkDeriveSLAACAddress_CacheMiss(b *testing.B) {
	resetSLAACHashCacheForBenchmark()
	prefix := netip.MustParsePrefix("2001:db8:1::/64")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		seed := "duid-" + strconv.Itoa(i) + "|aa:bb:cc:dd:ee:ff|12345"
		if _, err := deriveSLAACAddress(prefix, seed); err != nil {
			b.Fatalf("derive failed: %v", err)
		}
	}
}

func BenchmarkDeriveSLAACAddress_ParallelCacheHit(b *testing.B) {
	resetSLAACHashCacheForBenchmark()
	prefix := netip.MustParsePrefix("2001:db8:1::/64")
	seed := "duid-0001|aa:bb:cc:dd:ee:ff|12345"
	if _, err := deriveSLAACAddress(prefix, seed); err != nil {
		b.Fatalf("warmup failed: %v", err)
	}

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := deriveSLAACAddress(prefix, seed); err != nil {
				b.Fatalf("derive failed: %v", err)
			}
		}
	})
}

func BenchmarkDeriveSLAACAddress_ParallelCacheMiss(b *testing.B) {
	resetSLAACHashCacheForBenchmark()
	prefix := netip.MustParsePrefix("2001:db8:1::/64")
	var counter uint64

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			n := atomic.AddUint64(&counter, 1)
			seed := "duid-" + strconv.FormatUint(n, 10) + "|aa:bb:cc:dd:ee:ff|12345"
			if _, err := deriveSLAACAddress(prefix, seed); err != nil {
				b.Fatalf("derive failed: %v", err)
			}
		}
	})
}

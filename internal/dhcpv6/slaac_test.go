package dhcpv6

import (
	"net/netip"
	"strings"
	"testing"
)

func TestIsValidSLAACAddr(t *testing.T) {
	prefix := netip.MustParsePrefix("2001:db8:1::/64")
	tests := []struct {
		name  string
		addr  netip.Addr
		valid bool
	}{
		{
			name:  "valid host id",
			addr:  netip.MustParseAddr("2001:db8:1::1234"),
			valid: true,
		},
		{
			name:  "all zero host id rejected",
			addr:  netip.MustParseAddr("2001:db8:1::"),
			valid: false,
		},
		{
			name:  "all ones host id rejected",
			addr:  netip.MustParseAddr("2001:db8:1::ffff:ffff:ffff:ffff"),
			valid: false,
		},
		{
			name:  "outside prefix rejected",
			addr:  netip.MustParseAddr("2001:db8:2::1"),
			valid: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValidSLAACAddr(tc.addr, prefix); got != tc.valid {
				t.Fatalf("IsValidSLAACAddr()=%v, want %v", got, tc.valid)
			}
		})
	}
}

func TestIsValidSLAACAddr_LinkLocalPrefixRules(t *testing.T) {
	prefix := netip.MustParsePrefix("fe80::/10")

	if !IsValidSLAACAddr(netip.MustParseAddr("fe80::1"), prefix) {
		t.Fatalf("expected fe80::1 to be valid for fe80::/10")
	}

	if IsValidSLAACAddr(netip.MustParseAddr("febf::1"), prefix) {
		t.Fatalf("expected febf::1 to be rejected for fe80::/10 canonical SLAAC form")
	}
}

func TestDeriveSLAACAddress(t *testing.T) {
	tests := []struct {
		name     string
		prefix   netip.Prefix
		seed     string
		wantErr  string
		wantBits int
	}{
		{
			name:     "prefix bits 64",
			prefix:   netip.MustParsePrefix("2001:db8:1::/64"),
			seed:     "duid|aa:bb:cc:dd:ee:ff|123",
			wantBits: 64,
		},
		{
			name:     "prefix bits 120",
			prefix:   netip.MustParsePrefix("2001:db8:1::/120"),
			seed:     "duid|aa:bb:cc:dd:ee:ff|123",
			wantBits: 120,
		},
		{
			name:     "prefix bits 0",
			prefix:   netip.MustParsePrefix("::/0"),
			seed:     "duid|aa:bb:cc:dd:ee:ff|123",
			wantBits: 0,
		},
		{
			name:    "empty seed",
			prefix:  netip.MustParsePrefix("2001:db8:1::/64"),
			seed:    "",
			wantErr: "slaac seed is empty",
		},
		{
			name:    "invalid non-ipv6 prefix",
			prefix:  netip.MustParsePrefix("192.168.1.0/24"),
			seed:    "seed",
			wantErr: "invalid IPv6 prefix",
		},
		{
			name:    "host bits zero",
			prefix:  netip.MustParsePrefix("2001:db8::1/128"),
			seed:    "seed",
			wantErr: "invalid prefix bits",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			addr, err := deriveSLAACAddress(tc.prefix, tc.seed)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !addr.IsValid() {
				t.Fatalf("derived address is invalid")
			}
			if tc.prefix.Bits() != tc.wantBits {
				t.Fatalf("test setup mismatch: got bits=%d, want=%d", tc.prefix.Bits(), tc.wantBits)
			}
			if !tc.prefix.Masked().Contains(addr) {
				t.Fatalf("derived address %q not in prefix %q", addr.String(), tc.prefix.String())
			}
			if !IsValidSLAACAddr(addr, tc.prefix) {
				t.Fatalf("derived address must satisfy SLAAC validation")
			}
		})
	}
}

func TestIsValidSLAACAddr_RejectsAllZeroAndAllOnesHostPart(t *testing.T) {
	prefix := netip.MustParsePrefix("2001:db8:2::/64")

	allZero := netip.MustParseAddr("2001:db8:2::")
	if IsValidSLAACAddr(allZero, prefix) {
		t.Fatalf("expected all-zero host part to be rejected")
	}

	allOnes := netip.MustParseAddr("2001:db8:2::ffff:ffff:ffff:ffff")
	if IsValidSLAACAddr(allOnes, prefix) {
		t.Fatalf("expected all-ones host part to be rejected")
	}
}

func TestDeriveSLAACAddress_UniqueAcrossSeeds(t *testing.T) {
	prefix := netip.MustParsePrefix("2001:db8:9::/64")
	seedA := "duid-a|aa:bb:cc:dd:ee:11|10"
	seedB := "duid-b|aa:bb:cc:dd:ee:22|11"

	addrA, err := deriveSLAACAddress(prefix, seedA)
	if err != nil {
		t.Fatalf("derive with seedA failed: %v", err)
	}
	addrB, err := deriveSLAACAddress(prefix, seedB)
	if err != nil {
		t.Fatalf("derive with seedB failed: %v", err)
	}
	if addrA == addrB {
		t.Fatalf("expected different seeds to derive different addresses, got %q", addrA.String())
	}
}

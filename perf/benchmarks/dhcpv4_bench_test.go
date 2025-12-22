package benchmarks

import (
	"net"
	"testing"

	"modern-dhcp/internal/dhcpv4"
)

var sampleDiscover []byte

func init() {
	msg := &dhcpv4.Message{
		Op:    1,
		HType: 1,
		HLen:  6,
		XID:   0xdeadbeef,
		CHAddr: []byte{
			0x00, 0x0c, 0x29, 0xab, 0xcd, 0xef,
		},
		GIAddr: net.IPv4(10, 0, 0, 1),
		CIAddr: net.IPv4zero,
		YIAddr: net.IPv4zero,
	}
	msg.Options = map[byte][]byte{
		dhcpv4.OptionDHCPMessageType:      {dhcpv4.MessageTypeDiscover},
		dhcpv4.OptionClientIdentifier:     {0x01, 0x00, 0x0c, 0x29, 0xab, 0xcd, 0xef},
		dhcpv4.OptionParameterRequestList: {1, 3, 6, 15, 28, 51},
	}
	payload, err := msg.MarshalBinary()
	if err != nil {
		panic(err)
	}
	sampleDiscover = payload
}

func BenchmarkDHCPv4Parse(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := dhcpv4.ParseMessage(sampleDiscover); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDHCPv4Marshal(b *testing.B) {
	msg, err := dhcpv4.ParseMessage(sampleDiscover)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := msg.MarshalBinary(); err != nil {
			b.Fatal(err)
		}
	}
}

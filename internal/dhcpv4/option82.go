package dhcpv4

import (
	"context"
	"net"

	"modern-dhcp/internal/dhcpv4/option"

	"go.uber.org/zap"
)

type Option82Policy = option.Option82Policy

type dhcpv4Option82Parsed struct {
	CircuitID    string
	RemoteID     string
	SubscriberID string
}

type dhcpv4Option82Validator struct {
	delegate option.Option82Validator
}

func dhcpv4Option82NewValidator(policy Option82Policy) *dhcpv4Option82Validator {
	return &dhcpv4Option82Validator{delegate: option.NewOption82Validator(policy)}
}

func (v *dhcpv4Option82Validator) dhcpv4Option82Validate(pkt Packet) error {
	if v == nil || v.delegate == nil {
		return nil
	}
	return v.delegate.Validate(pkt.Option82Present, pkt.Option82Error, pkt.Option82CircuitID, pkt.Option82RemoteID)
}

// dhcpv4Option82Parse decodes RFC3046 (DHCP Relay Agent Information Option) TLVs.
// RFC3046 defines sub-option 1 (Circuit ID), 2 (Remote ID), and 6 (Subscriber ID).
func dhcpv4Option82Parse(data []byte) (dhcpv4Option82Parsed, error) {
	parsed, err := option.ParseOption82(data)
	if err != nil {
		return dhcpv4Option82Parsed{}, err
	}
	return dhcpv4Option82Parsed(parsed), nil
}

func dhcpv4Option82SanitizeASCII(value []byte) string {
	return option.SanitizeASCII(value)
}

func (s *Server) dhcpv4Option82ValidateOrReject(ctx context.Context, msg *Message, pkt Packet, addr *net.UDPAddr) bool {
	if s.option82Validator == nil {
		return true
	}
	if err := s.option82Validator.dhcpv4Option82Validate(pkt); err != nil {
		if s.logger != nil {
			dhcpv4Logger(ctx, s.logger).Error("dhcpv4 option82 validation failed",
				zap.Error(err),
				zap.Uint32("xid", pkt.XID),
				zap.String("circuitId", pkt.Option82CircuitID),
				zap.String("remoteId", pkt.Option82RemoteID),
				zap.String("subscriberId", pkt.Option82SubscriberID),
			)
		}
		s.sendNak(ctx, msg, addr, err.Error())
		return false
	}
	return true
}

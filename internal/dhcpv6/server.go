package dhcpv6

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"net/netip"
	"sync"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/lease"
	"modern-dhcp/internal/relay"
)

// Options configures the DHCPv6 UDP server.
type Options struct {
	BindAddress       string
	Port              int
	TenantID          string
	ServerID          []byte
	ReadTimeout       time.Duration
	PreferredLifetime time.Duration
	ValidLifetime     time.Duration
	Partitioner       *relay.Partitioner
}

// Server handles DHCPv6 UDP traffic and delegates to the handler.
type Server struct {
	opts        Options
	handler     *Handler
	logger      *zap.Logger
	serverID    []byte
	partitioner *relay.Partitioner

	mu   sync.Mutex
	conn *net.UDPConn
}

// NewServer creates a DHCPv6 server instance.
func NewServer(opts Options, handler *Handler, logger *zap.Logger) *Server {
	if opts.Port == 0 {
		opts.Port = 547
	}
	if opts.ReadTimeout == 0 {
		opts.ReadTimeout = 5 * time.Second
	}
	serverID := opts.ServerID
	if len(serverID) == 0 {
		serverID = generateServerDUID()
	}
	return &Server{opts: opts, handler: handler, logger: logger, serverID: serverID, partitioner: opts.Partitioner}
}

// ListenAndServe starts processing DHCPv6 packets until the context is canceled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	if s.handler == nil {
		return errors.New("dhcpv6: handler is nil")
	}

	addr := &net.UDPAddr{IP: net.ParseIP(s.opts.BindAddress), Port: s.opts.Port}
	if addr.IP == nil {
		addr.IP = net.IPv6unspecified
	}

	conn, err := net.ListenUDP("udp6", addr)
	if err != nil {
		return fmt.Errorf("dhcpv6: listen failed: %w", err)
	}

	s.logger.Info("dhcpv6 listener started", zap.String("addr", addr.String()))
	s.mu.Lock()
	s.conn = conn
	s.mu.Unlock()

	buf := make([]byte, 2048)
	for {
		if err := conn.SetReadDeadline(time.Now().Add(s.opts.ReadTimeout)); err != nil {
			s.logger.Warn("dhcpv6 read deadline failed", zap.Error(err))
		}
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				if ctx.Err() != nil {
					return nil
				}
				continue
			}
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("dhcpv6: read error: %w", err)
		}
		payload := make([]byte, n)
		copy(payload, buf[:n])
		go s.processDatagram(ctx, payload, remote)
	}
}

// Shutdown terminates the UDP listener.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return nil
	}
	done := make(chan struct{})
	go func(c *net.UDPConn) {
		_ = c.Close()
		close(done)
	}(s.conn)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		s.logger.Info("dhcpv6 listener stopped")
		return nil
	}
}

func (s *Server) processDatagram(ctx context.Context, data []byte, addr *net.UDPAddr) {
	msg, err := ParseMessage(data)
	if err != nil {
		s.logger.Debug("dhcpv6 parse failed", zap.Error(err))
		return
	}

	pkt, err := packetFromMessage(msg)
	if err != nil {
		s.logger.Debug("dhcpv6 packet conversion failed", zap.Error(err))
		return
	}
	if !s.shouldHandleRelay(pkt) {
		return
	}

	var result *lease.Result
	switch msg.MessageType {
	case MessageTypeInformationReq:
		// stateless reply
	case MessageTypeRelease:
		if err := s.handler.HandleRelease(ctx, s.opts.TenantID, pkt); err != nil {
			s.logger.Warn("dhcpv6 release handling failed", zap.Error(err))
			return
		}
	case MessageTypeDecline:
		if err := s.handler.HandleDecline(ctx, s.opts.TenantID, pkt); err != nil {
			s.logger.Warn("dhcpv6 decline handling failed", zap.Error(err))
			return
		}
	default:
		var handleErr error
		result, handleErr = s.handler.HandleRequest(ctx, s.opts.TenantID, pkt)
		if handleErr != nil {
			s.logger.Warn("dhcpv6 handler error", zap.Error(handleErr))
			return
		}
	}

	replyType := responseType(msg.MessageType)
	if replyType == 0 {
		return
	}

	resp, err := s.buildResponse(msg, pkt, result, replyType, msg.MessageType)
	if err != nil {
		s.logger.Warn("dhcpv6 response build failed", zap.Error(err))
		return
	}

	s.sendResponse(resp, addr)
}

func responseType(msgType byte) byte {
	switch msgType {
	case MessageTypeSolicit:
		return MessageTypeAdvertise
	case MessageTypeRequest, MessageTypeRenew, MessageTypeRebind, MessageTypeConfirm, MessageTypeInformationReq, MessageTypeRelease, MessageTypeDecline:
		return MessageTypeReply
	default:
		return 0
	}
}

func (s *Server) buildResponse(req *Message, pkt Packet, result *lease.Result, msgType byte, reqType byte) ([]byte, error) {
	options := make([]byte, 0)
	if clientID := req.Option(OptionClientID); clientID != nil {
		options = appendOption(options, OptionClientID, clientID)
	}
	options = appendOption(options, OptionServerID, s.serverID)
	if msgType == MessageTypeAdvertise {
		options = appendOption(options, OptionPreference, []byte{0x40})
	}

	if reqType == MessageTypeInformationReq {
		txID := req.TransactionID & 0xFFFFFF
		payload := buildBaseMessage(msgType, txID, options)
		if len(req.RelayHops) == 0 {
			return payload, nil
		}
		return wrapRelayResponse(req.RelayHops, payload)
	}

	needsLease := reqType != MessageTypeRelease && reqType != MessageTypeDecline
	if needsLease {
		if result == nil || result.Lease == nil {
			return nil, errors.New("dhcpv6: missing lease result")
		}
		ip := net.ParseIP(result.Lease.IPAddress)
		if ip == nil || ip.To16() == nil || ip.To4() != nil {
			return nil, errors.New("dhcpv6: invalid IPv6 lease IP")
		}

		iaid := pkt.IAID
		if iaid == 0 {
			if raw := req.Option(OptionIANA); raw != nil {
				iaid, _ = parseIANA(raw)
			}
		}

		preferred := secondsValue(result.Profile.DefaultDuration, s.opts.PreferredLifetime, time.Hour)
		valid := secondsValue(result.Profile.MaxDuration, s.opts.ValidLifetime, time.Duration(preferred)*2*time.Second)
		if valid < preferred {
			valid = preferred
		}
		t1 := secondsValue(result.Profile.RenewalTime, 0, time.Duration(preferred/2)*time.Second)
		t2 := secondsValue(result.Profile.RebindingTime, 0, time.Duration((preferred*3)/4)*time.Second)
		iaPayload := encodeIANA(iaid, t1, t2, ip, preferred, valid)
		options = appendOption(options, OptionIANA, iaPayload)

		if len(result.PrefixDelegations) > 0 {
			for i := range result.PrefixDelegations {
				pd := result.PrefixDelegations[i]
				pdPayload, err := encodeIAPDOption(&pd, t1, t2, preferred, valid)
				if err != nil {
					return nil, err
				}
				options = appendOption(options, OptionIAPD, pdPayload)
			}
		}
	}

	txID := req.TransactionID & 0xFFFFFF
	payload := buildBaseMessage(msgType, txID, options)
	if len(req.RelayHops) == 0 {
		return payload, nil
	}
	return wrapRelayResponse(req.RelayHops, payload)
}

func (s *Server) sendResponse(payload []byte, addr *net.UDPAddr) {
	if len(payload) == 0 {
		return
	}
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		s.logger.Warn("dhcpv6 connection unavailable")
		return
	}
	if _, err := conn.WriteToUDP(payload, addr); err != nil {
		s.logger.Warn("dhcpv6 send failed", zap.Error(err))
	}
}

func buildBaseMessage(msgType byte, txID uint32, options []byte) []byte {
	buf := make([]byte, 4)
	buf[0] = msgType
	buf[1] = byte((txID >> 16) & 0xFF)
	buf[2] = byte((txID >> 8) & 0xFF)
	buf[3] = byte(txID & 0xFF)
	return append(buf, options...)
}

func encodeIANA(iaid uint32, t1, t2 uint32, addr net.IP, preferred, valid uint32) []byte {
	body := make([]byte, 12)
	binary.BigEndian.PutUint32(body[0:4], iaid)
	binary.BigEndian.PutUint32(body[4:8], t1)
	binary.BigEndian.PutUint32(body[8:12], t2)
	iaAddr := encodeIAAddr(addr, preferred, valid)
	return append(body, iaAddr...)
}

func encodeIAPDOption(pd *lease.PrefixDelegation, t1, t2, defaultPreferred, defaultValid uint32) ([]byte, error) {
	if pd == nil {
		return nil, errors.New("dhcpv6: missing prefix delegation info")
	}
	prefix, err := netip.ParsePrefix(pd.Prefix)
	if err != nil {
		return nil, fmt.Errorf("dhcpv6: invalid delegated prefix: %w", err)
	}
	if !prefix.Addr().Is6() {
		return nil, errors.New("dhcpv6: delegated prefix must be IPv6")
	}
	pdPreferred := clampSeconds(pd.PreferredLifetime)
	if pdPreferred == 0 {
		pdPreferred = defaultPreferred
	}
	pdValid := clampSeconds(pd.ValidLifetime)
	if pdValid == 0 {
		pdValid = defaultValid
	}
	if pdValid < pdPreferred {
		pdValid = pdPreferred
	}
	body := make([]byte, 12)
	binary.BigEndian.PutUint32(body[0:4], pd.IAPDID)
	binary.BigEndian.PutUint32(body[4:8], t1)
	binary.BigEndian.PutUint32(body[8:12], t2)
	iaPrefix, err := encodeIAPrefix(prefix, pd.PrefixLength, pdPreferred, pdValid)
	if err != nil {
		return nil, err
	}
	return append(body, iaPrefix...), nil
}

func encodeIAPrefix(prefix netip.Prefix, length byte, preferred, valid uint32) ([]byte, error) {
	if !prefix.Addr().Is6() {
		return nil, errors.New("dhcpv6: ia prefix requires IPv6")
	}
	maskBits := int(length)
	if maskBits <= 0 {
		maskBits = prefix.Bits()
	}
	if maskBits > 128 {
		maskBits = 128
	}
	masked := netip.PrefixFrom(prefix.Masked().Addr(), maskBits).Masked()
	payload := make([]byte, 25)
	binary.BigEndian.PutUint32(payload[0:4], preferred)
	binary.BigEndian.PutUint32(payload[4:8], valid)
	payload[8] = byte(maskBits)
	addr := masked.Masked().Addr()
	if !addr.Is6() {
		return nil, errors.New("dhcpv6: ia prefix addr invalid")
	}
	bytes := addr.As16()
	copy(payload[9:25], bytes[:])
	return appendOption(nil, OptionIAPrefix, payload), nil
}

func encodeIAAddr(addr net.IP, preferred, valid uint32) []byte {
	ip := addr.To16()
	if ip == nil {
		ip = make([]byte, 16)
	}
	payload := make([]byte, 24)
	copy(payload[0:16], ip)
	binary.BigEndian.PutUint32(payload[16:20], preferred)
	binary.BigEndian.PutUint32(payload[20:24], valid)
	return appendOption(nil, OptionIAAddr, payload)
}

func appendOption(buf []byte, code uint16, data []byte) []byte {
	option := make([]byte, 4+len(data))
	binary.BigEndian.PutUint16(option[0:2], code)
	binary.BigEndian.PutUint16(option[2:4], uint16(len(data)))
	copy(option[4:], data)
	return append(buf, option...)
}

func wrapRelayResponse(hops []RelayHop, payload []byte) ([]byte, error) {
	outer := payload
	for i := len(hops) - 1; i >= 0; i-- {
		hop := hops[i]
		header := make([]byte, 34)
		header[0] = MessageTypeRelayReply
		header[1] = hop.HopCount
		copy(header[2:18], padIPv6(hop.LinkAddr))
		copy(header[18:34], padIPv6(hop.PeerAddr))
		options := appendOption(nil, OptionRelayMsg, outer)
		for code, values := range hop.Options {
			if code == OptionRelayMsg {
				continue
			}
			for _, v := range values {
				optionVal := make([]byte, len(v))
				copy(optionVal, v)
				options = appendOption(options, code, optionVal)
			}
		}
		outer = append(header, options...)
	}
	return outer, nil
}

func padIPv6(ip net.IP) []byte {
	out := make([]byte, 16)
	if ip == nil {
		return out
	}
	copy(out, ip.To16())
	return out
}

func (s *Server) shouldHandleRelay(pkt Packet) bool {
	if s == nil || s.partitioner == nil {
		return true
	}
	meta := relay.BuildMetadata(s.opts.TenantID, pkt.LinkAddr, pkt.RelayInfo, parseRelayVLAN(pkt.RelayInfo), nil)
	if s.partitioner.ShouldHandle(meta) {
		return true
	}
	s.logger.Debug("dhcpv6 relay partition drop", zap.String("relayId", meta.RelayID))
	return false
}

func generateServerDUID() []byte {
	duid := make([]byte, 14)
	binary.BigEndian.PutUint16(duid[0:2], 1) // DUID-LLT
	binary.BigEndian.PutUint16(duid[2:4], 1) // Ethernet
	binary.BigEndian.PutUint32(duid[4:8], uint32(time.Now().Unix()))
	if _, err := rand.Read(duid[8:]); err != nil {
		copy(duid[8:], []byte{0, 1, 2, 3, 4, 5})
	}
	return duid
}

func secondsValue(value time.Duration, override time.Duration, fallback time.Duration) uint32 {
	if override > 0 {
		return clampSeconds(override)
	}
	if value > 0 {
		return clampSeconds(value)
	}
	if fallback > 0 {
		return clampSeconds(fallback)
	}
	return 0
}

func clampSeconds(d time.Duration) uint32 {
	if d <= 0 {
		return 0
	}
	secs := d / time.Second
	if secs <= 0 {
		secs = 1
	}
	if secs > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(secs)
}

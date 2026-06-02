package dhcpv6

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/juju/ratelimit"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"

	dhcpv6fsm "modern-dhcp/internal/dhcpv6/leasefsm"
	"modern-dhcp/internal/lease"
	"modern-dhcp/internal/relay"
	"modern-dhcp/pkg/models"
)

// Options configures the DHCPv6 UDP server.
type Options struct {
	BindAddress       string
	Port              int
	TenantID          string
	ServerID          []byte
	RateLimitPPS      int
	RelayWhitelist    []string
	ClusterServerIDs  []string
	ReadTimeout       time.Duration
	PreferredLifetime time.Duration
	ValidLifetime     time.Duration
	Partitioner       *relay.Partitioner
}

type packetMiddleware func(ctx context.Context, msg *Message, pkt Packet, remote *net.UDPAddr) bool

type rogueAdvertiseEvent struct {
	Remote   string
	ServerID string
}

// Server handles DHCPv6 UDP traffic and delegates to the handler.
type Server struct {
	opts        Options
	handler     *Handler
	logger      *zap.Logger
	serverID    []byte
	partitioner *relay.Partitioner
	relayAllow  map[string]struct{}
	rateLimiter *dhcpv6IdentityRateLimiter
	middleware  []packetMiddleware
	rogueAllow  map[string]struct{}
	rogueEvents chan rogueAdvertiseEvent

	initMu               sync.Mutex
	securityInitialized  bool
	rogueDetectorStarted bool

	mu   sync.Mutex
	conn *net.UDPConn

	fsmMu     sync.Mutex
	leaseFSMs map[string]dhcpv6fsm.State
}

var (
	dhcpv6PacketRateLimitedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dhcpv6_packet_rate_limited_total",
		Help: "Count of DHCPv6 packets dropped due to per-identity rate limiting",
	})
	dhcpv6RogueServerDetectedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dhcpv6_rogue_server_detected_total",
		Help: "Count of rogue DHCPv6 server Advertise packets detected",
	})
)

// NewServer creates a DHCPv6 server instance.
func NewServer(opts Options, handler *Handler, logger *zap.Logger) *Server {
	if opts.Port == 0 {
		opts.Port = 547
	}
	if opts.ReadTimeout == 0 {
		opts.ReadTimeout = 5 * time.Second
	}
	if opts.RateLimitPPS <= 0 {
		opts.RateLimitPPS = 10
	}
	serverID := opts.ServerID
	if len(serverID) == 0 {
		serverID = generateServerDUID()
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	srv := &Server{opts: opts, handler: handler, logger: logger, serverID: serverID, partitioner: opts.Partitioner, relayAllow: normalizeRelayWhitelist(opts.RelayWhitelist), leaseFSMs: make(map[string]dhcpv6fsm.State)}
	srv.initializeSecurityPipelines(nil)
	return srv
}

// ListenAndServe starts processing DHCPv6 packets until the context is canceled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	if s.handler == nil {
		return errors.New("dhcpv6: handler is nil")
	}
	s.initializeSecurityPipelines(ctx)

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
	s.initializeSecurityPipelines(nil)
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
	if len(msg.RelayHops) > 0 {
		hopChain := relayHopChain(msg.RelayHops)
		if s.logger != nil {
			s.logger.Debug("dhcpv6 relay hop chain", zap.Int("hopCount", len(msg.RelayHops)), zap.Any("relayHopChain", hopChain))
		}
		if !s.isRelaySourceAllowed(pkt.LinkAddr) {
			if s.logger != nil {
				s.logger.Error("dhcpv6 relay source rejected", zap.String("linkAddr", ipString(pkt.LinkAddr)), zap.String("peerAddr", ipString(pkt.PeerAddr)), zap.String("remoteAddr", ipString(addr.IP)), zap.Any("relayHopChain", hopChain))
			}
			return
		}
	}
	if !s.shouldHandleRelay(pkt) {
		return
	}
	if !s.applyMiddleware(ctx, msg, pkt, addr) {
		return
	}
	clientKey := leaseFSMClientKey(pkt)
	if err := s.applyFSMPreTransition(clientKey, msg.MessageType); err != nil {
		s.logger.Warn("dhcpv6 lease fsm pre-transition rejected", zap.String("clientKey", clientKey), zap.Uint8("messageType", msg.MessageType), zap.Error(err))
		return
	}

	var result *lease.Result
	switch msg.MessageType {
	case MessageTypeSolicit:
		var handleErr error
		result, handleErr = s.handler.HandleSolicit(ctx, s.opts.TenantID, pkt)
		if handleErr != nil {
			if s.trySendNACK(msg, pkt, msg.MessageType, handleErr, addr) {
				return
			}
			s.logger.Warn("dhcpv6 solicit handling failed", zap.Error(handleErr))
			return
		}
	case MessageTypeInformationReq:
		// stateless reply
	case MessageTypeConfirm:
		var handleErr error
		result, handleErr = s.handler.HandleConfirm(ctx, s.opts.TenantID, pkt)
		if handleErr != nil {
			if s.trySendNACK(msg, pkt, msg.MessageType, handleErr, addr) {
				return
			}
			s.logger.Warn("dhcpv6 confirm handling failed", zap.Error(handleErr))
			return
		}
	case MessageTypeRenew:
		var handleErr error
		result, handleErr = s.handler.HandleRenew(ctx, s.opts.TenantID, pkt)
		if handleErr != nil {
			if s.trySendNACK(msg, pkt, msg.MessageType, handleErr, addr) {
				return
			}
			s.logger.Warn("dhcpv6 renew handling failed", zap.Error(handleErr))
			return
		}
	case MessageTypeRebind:
		var handleErr error
		result, handleErr = s.handler.HandleRebind(ctx, s.opts.TenantID, pkt)
		if handleErr != nil {
			if s.trySendNACK(msg, pkt, msg.MessageType, handleErr, addr) {
				return
			}
			s.logger.Warn("dhcpv6 rebind handling failed", zap.Error(handleErr))
			return
		}
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
			if s.trySendNACK(msg, pkt, msg.MessageType, handleErr, addr) {
				return
			}
			s.logger.Warn("dhcpv6 handler error", zap.Error(handleErr))
			return
		}
	}
	if err := s.applyFSMSuccessTransition(clientKey, msg.MessageType); err != nil {
		s.logger.Warn("dhcpv6 lease fsm success transition rejected", zap.String("clientKey", clientKey), zap.Uint8("messageType", msg.MessageType), zap.Error(err))
		return
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

func (s *Server) trySendNACK(req *Message, pkt Packet, reqType byte, err error, addr *net.UDPAddr) bool {
	if err == nil {
		return false
	}
	var nackErr *NACKError
	if !errors.As(err, &nackErr) {
		return false
	}
	respType := responseType(reqType)
	if respType == 0 {
		return true
	}
	result := zeroLifetimeConfirmResult(pkt)
	resp, buildErr := s.buildResponse(req, pkt, result, respType, reqType)
	if buildErr != nil {
		s.logger.Warn("dhcpv6 nack response build failed", zap.Error(buildErr))
		return true
	}
	s.logger.Info("dhcpv6 nack sent", zap.String("reason", nackErr.Error()))
	s.sendResponse(resp, addr)
	return true
}

func (s *Server) initializeSecurityPipelines(ctx context.Context) {
	if s == nil {
		return
	}
	s.initMu.Lock()
	defer s.initMu.Unlock()
	if !s.securityInitialized {
		s.rateLimiter = dhcpv6NewIdentityRateLimiter(s.opts.RateLimitPPS)
		s.rogueAllow = normalizeServerIDSet(s.opts.ClusterServerIDs, s.serverID)
		s.rogueEvents = make(chan rogueAdvertiseEvent, 128)
		s.middleware = []packetMiddleware{
			s.rateLimitMiddleware(),
			s.rogueDetectMiddleware(),
		}
		s.securityInitialized = true
	}
	if ctx != nil && !s.rogueDetectorStarted {
		s.rogueDetectorStarted = true
		go s.runRogueDetector(ctx)
	}
}

func (s *Server) applyMiddleware(ctx context.Context, msg *Message, pkt Packet, remote *net.UDPAddr) bool {
	for i := range s.middleware {
		if !s.middleware[i](ctx, msg, pkt, remote) {
			return false
		}
	}
	return true
}

func (s *Server) rateLimitMiddleware() packetMiddleware {
	return func(_ context.Context, msg *Message, pkt Packet, remote *net.UDPAddr) bool {
		identity := packetIdentityKey(pkt)
		if identity == "" {
			identity = messageClientIDKey(msg)
		}
		if identity == "" || s.rateLimiter.allow(identity) {
			return true
		}
		dhcpv6PacketRateLimitedTotal.Inc()
		if s.logger != nil {
			s.logger.Warn("dhcpv6 packet rate limit exceeded; dropping packet",
				zap.String("identity", identity),
				zap.String("remote", udpAddrString(remote)),
				zap.Uint8("messageType", msg.MessageType),
			)
		}
		return false
	}
}

func (s *Server) rogueDetectMiddleware() packetMiddleware {
	return func(_ context.Context, msg *Message, _ Packet, remote *net.UDPAddr) bool {
		if msg == nil || msg.MessageType != MessageTypeAdvertise {
			return true
		}
		serverID := canonicalServerID(msg.Option(OptionServerID))
		if serverID == "" {
			return false
		}
		if _, ok := s.rogueAllow[serverID]; ok {
			return false
		}
		dhcpv6RogueServerDetectedTotal.Inc()
		evt := rogueAdvertiseEvent{Remote: udpAddrString(remote), ServerID: serverID}
		if s.rogueEvents != nil {
			select {
			case s.rogueEvents <- evt:
			default:
			}
		}
		if s.logger != nil {
			s.logger.Error("rogue dhcpv6 advertise detected", zap.String("remote", evt.Remote), zap.String("serverId", evt.ServerID))
		}
		return false
	}
}

func (s *Server) runRogueDetector(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-s.rogueEvents:
			if s.logger != nil {
				s.logger.Debug("dhcpv6 rogue detector event", zap.String("remote", evt.Remote), zap.String("serverId", evt.ServerID))
			}
		}
	}
}

func canonicalServerID(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	return strings.ToLower(hex.EncodeToString(raw))
}

func normalizeServerIDSet(clusterServerIDs []string, selfServerID []byte) map[string]struct{} {
	out := make(map[string]struct{}, len(clusterServerIDs)+1)
	if self := canonicalServerID(selfServerID); self != "" {
		out[self] = struct{}{}
	}
	for _, raw := range clusterServerIDs {
		trimmed := strings.TrimSpace(strings.ToLower(raw))
		if trimmed == "" {
			continue
		}
		if decoded, err := hex.DecodeString(trimmed); err == nil {
			if normalized := canonicalServerID(decoded); normalized != "" {
				out[normalized] = struct{}{}
			}
			continue
		}
		out[trimmed] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type dhcpv6IdentityRateLimiter struct {
	pps     int64
	buckets map[string]*ratelimit.Bucket
	mu      sync.Mutex
}

func dhcpv6NewIdentityRateLimiter(pps int) *dhcpv6IdentityRateLimiter {
	if pps <= 0 {
		pps = 10
	}
	return &dhcpv6IdentityRateLimiter{pps: int64(pps), buckets: make(map[string]*ratelimit.Bucket, 256)}
}

func (l *dhcpv6IdentityRateLimiter) allow(key string) bool {
	if l == nil {
		return true
	}
	key = strings.TrimSpace(strings.ToLower(key))
	if key == "" {
		return true
	}
	l.mu.Lock()
	bucket, ok := l.buckets[key]
	if !ok {
		bucket = ratelimit.NewBucketWithRate(float64(l.pps), l.pps)
		l.buckets[key] = bucket
	}
	l.mu.Unlock()
	return bucket.TakeAvailable(1) == 1
}

func packetIdentityKey(pkt Packet) string {
	if duid := strings.TrimSpace(strings.ToLower(pkt.DUID)); duid != "" {
		return "duid:" + duid
	}
	if len(pkt.ClientMAC) > 0 {
		return "mac:" + strings.ToLower(pkt.ClientMAC.String())
	}
	return ""
}

func messageClientIDKey(msg *Message) string {
	if msg == nil {
		return ""
	}
	clientID := msg.Option(OptionClientID)
	if len(clientID) == 0 {
		return ""
	}
	return "duid:" + strings.ToLower(hex.EncodeToString(clientID))
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

	// RFC 3646 section 3: DNS Recursive Name Server option.
	if dnsPayload := encodeIPv6AddressListOption(s.responseDNSServers(result, pkt)); len(dnsPayload) > 0 {
		options = appendOption(options, OptionDNSRecursiveNameServer, dnsPayload)
	}
	// RFC 3646 section 4: Domain Search List option.
	if domainPayload, err := encodeDomainSearchListOption(s.responseDomainSearchList(result, pkt)); err != nil {
		s.logger.Warn("dhcpv6 domain search list encode failed", zap.Error(err))
	} else if len(domainPayload) > 0 {
		options = appendOption(options, OptionDomainSearchList, domainPayload)
	}
	// RFC 4704 section 4.1: Client FQDN option.
	if fqdnPayload, err := encodeFQDNOption(s.responseFQDN(result, pkt)); err != nil {
		s.logger.Warn("dhcpv6 fqdn encode failed", zap.Error(err))
	} else if len(fqdnPayload) > 0 {
		options = appendOption(options, OptionFQDN, fqdnPayload)
	}
	// RFC 5908 section 4: NTP Server option.
	if ntpPayload := encodeNTPServerOption(s.responseNTPServers(result, pkt)); len(ntpPayload) > 0 {
		options = appendOption(options, OptionNTPServer, ntpPayload)
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
		if reqType == MessageTypeConfirm && result.Profile.DefaultDuration <= 0 && result.Profile.MaxDuration <= 0 {
			preferred = 0
			valid = 0
		}
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
				pdPayload, err := encodeIAPDOption(&pd, result.Pool, t1, t2, preferred, valid)
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

func encodeIAPDOption(pd *lease.PrefixDelegation, poolObj *models.AddressPool, t1, t2, defaultPreferred, defaultValid uint32) ([]byte, error) {
	if pd == nil {
		return nil, errors.New("dhcpv6: missing prefix delegation info")
	}
	prefixValue := strings.TrimSpace(pd.Prefix)
	if prefixValue == "" && poolObj != nil {
		prefixValue = strings.TrimSpace(poolObj.CIDR)
	}
	prefix, err := netip.ParsePrefix(prefixValue)
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

func (s *Server) isRelaySourceAllowed(linkAddr net.IP) bool {
	if len(s.relayAllow) == 0 {
		return true
	}
	key := normalizeIPv6Address(linkAddr)
	if key == "" {
		return false
	}
	_, ok := s.relayAllow[key]
	return ok
}

func normalizeRelayWhitelist(raw []string) map[string]struct{} {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(raw))
	for _, value := range raw {
		if normalized := normalizeIPv6Address(net.ParseIP(strings.TrimSpace(value))); normalized != "" {
			out[normalized] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeIPv6Address(ip net.IP) string {
	if ip == nil {
		return ""
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok || !addr.Is6() {
		return ""
	}
	return addr.Unmap().String()
}

func ipString(ip net.IP) string {
	if normalized := normalizeIPv6Address(ip); normalized != "" {
		return normalized
	}
	if ip == nil {
		return ""
	}
	return strings.TrimSpace(ip.String())
}

func udpAddrString(addr *net.UDPAddr) string {
	if addr == nil {
		return ""
	}
	return strings.TrimSpace(addr.String())
}

func relayHopChain(hops []RelayHop) []map[string]any {
	if len(hops) == 0 {
		return nil
	}
	chain := make([]map[string]any, 0, len(hops))
	for idx := range hops {
		entry := map[string]any{
			"index":    idx,
			"hopCount": hops[idx].HopCount,
			"linkAddr": ipString(hops[idx].LinkAddr),
			"peerAddr": ipString(hops[idx].PeerAddr),
		}
		if attrs := relayHopAttributes(hops[idx]); len(attrs) > 0 {
			entry["attributes"] = attrs
		}
		chain = append(chain, entry)
	}
	return chain
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

func leaseFSMClientKey(pkt Packet) string {
	if id := strings.TrimSpace(pkt.DUID); id != "" {
		return "duid:" + id
	}
	if len(pkt.ClientMAC) > 0 {
		return "mac:" + strings.ToLower(pkt.ClientMAC.String())
	}
	return ""
}

func (s *Server) applyFSMPreTransition(clientKey string, msgType byte) error {
	if clientKey == "" {
		return nil
	}
	switch msgType {
	case MessageTypeRenew:
		return s.transitionLeaseState(clientKey, dhcpv6fsm.LeaseStateRenewing)
	case MessageTypeRebind:
		return s.transitionLeaseState(clientKey, dhcpv6fsm.LeaseStateRebinding)
	default:
		return nil
	}
}

func (s *Server) applyFSMSuccessTransition(clientKey string, msgType byte) error {
	if clientKey == "" {
		return nil
	}
	switch msgType {
	case MessageTypeSolicit:
		return s.transitionLeaseState(clientKey, dhcpv6fsm.LeaseStateOffered)
	case MessageTypeRequest, MessageTypeConfirm, MessageTypeRenew, MessageTypeRebind:
		return s.transitionLeaseState(clientKey, dhcpv6fsm.LeaseStateBound)
	case MessageTypeDecline:
		return s.transitionLeaseState(clientKey, dhcpv6fsm.LeaseStateExpired)
	case MessageTypeRelease:
		return s.transitionLeaseState(clientKey, dhcpv6fsm.LeaseStateReleased)
	default:
		return nil
	}
}

func (s *Server) transitionLeaseState(clientKey string, target dhcpv6fsm.State) error {
	s.fsmMu.Lock()
	defer s.fsmMu.Unlock()
	current := s.leaseFSMs[clientKey]
	if current == "" {
		current = dhcpv6fsm.LeaseStateInit
	}
	if err := dhcpv6fsm.ValidateTransition(current, target); err != nil {
		return err
	}
	if current == target {
		return nil
	}
	s.leaseFSMs[clientKey] = target
	if s.logger != nil {
		s.logger.Debug("dhcpv6 lease fsm state changed", zap.String("clientKey", clientKey), zap.String("from", string(current)), zap.String("to", string(target)))
	}
	return nil
}

func (s *Server) responseDNSServers(result *lease.Result, pkt Packet) []net.IP {
	servers := make([]net.IP, 0)
	if result != nil && result.Pool != nil {
		for _, raw := range result.Pool.DNS {
			parsed := net.ParseIP(string(raw))
			if parsed == nil || parsed.To16() == nil || parsed.To4() != nil {
				s.logger.Warn("dhcpv6 dns server ignored: invalid IPv6", zap.String("value", string(raw)))
				continue
			}
			servers = append(servers, append(net.IP(nil), parsed.To16()...))
		}
	}
	if len(servers) > 0 {
		return servers
	}
	if len(pkt.DNSRecursiveServers) > 0 {
		return append([]net.IP(nil), pkt.DNSRecursiveServers...)
	}
	return nil
}

func (s *Server) responseDomainSearchList(_ *lease.Result, pkt Packet) []string {
	if len(pkt.DomainSearchList) == 0 {
		return nil
	}
	return append([]string(nil), pkt.DomainSearchList...)
}

func (s *Server) responseFQDN(_ *lease.Result, pkt Packet) string {
	return pkt.FQDN
}

func (s *Server) responseNTPServers(_ *lease.Result, pkt Packet) []net.IP {
	if len(pkt.NTPServers) == 0 {
		return nil
	}
	return append([]net.IP(nil), pkt.NTPServers...)
}

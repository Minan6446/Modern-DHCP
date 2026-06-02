package dhcpv4

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/dhcpv4/builder"
	dhcpv4metrics "modern-dhcp/internal/dhcpv4/metrics"
	dhcpv4option "modern-dhcp/internal/dhcpv4/option"
	dhcpv4parser "modern-dhcp/internal/dhcpv4/parser"
	"modern-dhcp/internal/lease"
	"modern-dhcp/internal/metrics"
	"modern-dhcp/internal/relay"
)

const flagBroadcast uint16 = 1 << 15

// Options configures a DHCPv4 UDP server.
type Options struct {
	BindAddress    string
	Port           int
	TenantID       string
	ServerIP       net.IP
	ReadTimeout    time.Duration
	Security       SecurityOptions
	Metrics        *metrics.Collector
	MessageParser  dhcpv4parser.Decoder
	RelayParser    relay.Option82Parser
	Option82Policy Option82Policy
	Partitioner    *relay.Partitioner
	WorkerCount    int
	QueueDepth     int
	ListenerFanout int
	RXBatchSize    int
	ReusePort      bool
}

// Server wraps UDP I/O and delegates business logic to Handler.
type Server struct {
	opts              Options
	relayParser       relay.Option82Parser
	option82Validator *dhcpv4Option82Validator
	partitioner       *relay.Partitioner
	handler           *Handler
	logger            *zap.Logger
	metrics           *metrics.Collector
	packetObserver    dhcpv4metrics.Observer
	responseBuilder   builder.ResponseBuilder
	messageParser     dhcpv4parser.Decoder
	macLimiter        *dhcpv4MACRateLimiter
	relayIPWhitelist  map[string]struct{}
	rogueClusterNodes map[string]struct{}
	rogueConfig       RogueDetectorConfig

	mu    sync.Mutex
	conn  *net.UDPConn
	conns []*net.UDPConn
}

type datagram struct {
	payload []byte
	addr    *net.UDPAddr
}

// NewServer constructs a DHCPv4 server.
func NewServer(opts Options, handler *Handler, logger *zap.Logger) *Server {
	if opts.Port == 0 {
		opts.Port = 67
	}
	if opts.ReadTimeout == 0 {
		opts.ReadTimeout = 5 * time.Second
	}
	if opts.WorkerCount <= 0 {
		opts.WorkerCount = max(2, runtime.NumCPU())
	}
	if opts.QueueDepth <= 0 {
		opts.QueueDepth = opts.WorkerCount * 4
	}
	if opts.ListenerFanout <= 0 {
		opts.ListenerFanout = defaultListenerFanout()
	}
	if opts.RXBatchSize <= 0 {
		opts.RXBatchSize = 4
	}
	if opts.ListenerFanout <= 1 {
		opts.ReusePort = false
	}
	if opts.ServerIP == nil {
		opts.ServerIP = net.IPv4zero
	}
	relayParser := opts.RelayParser
	if relayParser == nil {
		relayParser = relay.NewDefaultOption82Parser()
	}
	messageParser := opts.MessageParser
	if messageParser == nil {
		messageParser = dhcpv4parser.NewDecoder()
	}
	validator := dhcpv4Option82NewValidator(opts.Option82Policy)
	macLimiter := dhcpv4NewMACRateLimiter(opts.Security.MACRateLimitPPS)
	relayWhitelist := normalizeIPSet(opts.Security.RelayWhitelist)
	rogueNodes := normalizeIPSet(opts.Security.RogueDetector.ClusterNodes)
	return &Server{
		opts:              opts,
		relayParser:       relayParser,
		option82Validator: validator,
		partitioner:       opts.Partitioner,
		handler:           handler,
		logger:            logger,
		metrics:           opts.Metrics,
		packetObserver:    dhcpv4metrics.NewObserver(opts.Metrics),
		responseBuilder:   builder.NewResponseBuilder(opts.ServerIP, encodeMobilityVendorOption),
		messageParser:     messageParser,
		macLimiter:        macLimiter,
		relayIPWhitelist:  relayWhitelist,
		rogueClusterNodes: rogueNodes,
		rogueConfig:       opts.Security.RogueDetector,
	}
}

// ListenAndServe starts the UDP loop until the context is done or fatal error occurs.
func (s *Server) ListenAndServe(ctx context.Context) error {
	if s.handler == nil {
		return errors.New("dhcpv4: handler is nil")
	}

	addr := &net.UDPAddr{IP: net.ParseIP(s.opts.BindAddress), Port: s.opts.Port}
	if addr.IP == nil {
		addr.IP = net.IPv4zero
	}

	listeners, err := s.openListeners(ctx, addr)
	if err != nil {
		return fmt.Errorf("dhcpv4: listen failed: %w", err)
	}
	if len(listeners) == 0 {
		return errors.New("dhcpv4: no listeners configured")
	}

	s.mu.Lock()
	s.conn = listeners[0]
	s.conns = listeners
	s.mu.Unlock()

	s.logger.Info("dhcpv4 listener started",
		zap.String("addr", addr.String()),
		zap.Int("listeners", len(listeners)),
		zap.Int("workers", s.opts.WorkerCount),
		zap.Int("queueDepth", s.opts.QueueDepth),
	)
	s.startRogueDetector(ctx)
	jobs := make(chan datagram, s.opts.QueueDepth)
	var workers sync.WaitGroup
	for i := 0; i < s.opts.WorkerCount; i++ {
		workers.Add(1)
		go s.runWorker(ctx, &workers, jobs)
	}
	errCh := make(chan error, len(listeners))
	var listenerWG sync.WaitGroup
	for shard, listener := range listeners {
		listenerWG.Add(1)
		go s.runListener(ctx, &listenerWG, listener, shard, jobs, errCh)
	}

	watchers := make(chan struct{})
	go func() {
		listenerWG.Wait()
		close(watchers)
	}()

	var runErr error
	select {
	case <-ctx.Done():
		// graceful shutdown
	case err := <-errCh:
		runErr = err
	}
	if runErr != nil {
		for _, c := range listeners {
			_ = c.Close()
		}
	}

	<-watchers
	close(jobs)
	workers.Wait()
	return runErr
}

func (s *Server) runWorker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan datagram) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}
			s.processDatagram(ctx, job.payload, job.addr)
		}
	}
}

func (s *Server) runListener(ctx context.Context, wg *sync.WaitGroup, conn *net.UDPConn, shard int, jobs chan<- datagram, errCh chan<- error) {
	defer wg.Done()
	buf := make([]byte, 1500)
	batch := max(1, s.opts.RXBatchSize)
	for {
		if ctx.Err() != nil {
			return
		}
		if err := conn.SetReadDeadline(time.Now().Add(s.opts.ReadTimeout)); err != nil {
			s.logger.Warn("dhcpv4 read deadline failed", zap.Int("listener", shard), zap.Error(err))
		}
		for i := 0; i < batch; i++ {
			n, remote, err := conn.ReadFromUDP(buf)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					break
				}
				if errors.Is(err, net.ErrClosed) {
					return
				}
				errCh <- fmt.Errorf("dhcpv4: listener %d read error: %w", shard, err)
				return
			}
			payload := make([]byte, n)
			copy(payload, buf[:n])
			select {
			case <-ctx.Done():
				return
			case jobs <- datagram{payload: payload, addr: remote}:
			default:
				s.logger.Warn("dhcpv4 datagram queue full, dropping packet",
					zap.Int("queueDepth", s.opts.QueueDepth),
					zap.Int("listener", shard),
				)
			}
		}
	}
}

func (s *Server) openListeners(ctx context.Context, addr *net.UDPAddr) ([]*net.UDPConn, error) {
	count := s.opts.ListenerFanout
	if count <= 0 {
		count = 1
	}
	reuse := s.opts.ReusePort && count > 1 && reusePortSupported()
	if s.opts.ReusePort && count > 1 && !reusePortSupported() {
		s.logger.Warn("reuseport not supported on this platform, collapsing to single listener")
		count = 1
		reuse = false
	}
	conns := make([]*net.UDPConn, 0, count)
	for i := 0; i < count; i++ {
		conn, err := s.listenUDP(ctx, addr, reuse)
		if err != nil {
			for _, c := range conns {
				_ = c.Close()
			}
			return nil, err
		}
		conns = append(conns, conn)
	}
	return conns, nil
}

func (s *Server) listenUDP(ctx context.Context, addr *net.UDPAddr, reuse bool) (*net.UDPConn, error) {
	if reuse {
		var ctrlErr error
		lc := net.ListenConfig{
			Control: func(network, address string, c syscall.RawConn) error {
				return c.Control(func(fd uintptr) {
					if err := setReusePort(fd); err != nil {
						ctrlErr = err
					}
				})
			},
		}
		pc, err := lc.ListenPacket(ctx, "udp4", addr.String())
		if err != nil {
			return nil, err
		}
		if ctrlErr != nil {
			_ = pc.Close()
			return nil, ctrlErr
		}
		udpConn, ok := pc.(*net.UDPConn)
		if !ok {
			_ = pc.Close()
			return nil, fmt.Errorf("dhcpv4: unexpected packet conn type %T", pc)
		}
		return udpConn, nil
	}
	return net.ListenUDP("udp4", cloneUDPAddr(addr))
}

// Shutdown stops the UDP listener.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	conns := append([]*net.UDPConn(nil), s.conns...)
	s.mu.Unlock()
	if len(conns) == 0 {
		return nil
	}
	var wg sync.WaitGroup
	errCh := make(chan error, len(conns))
	for _, c := range conns {
		if c == nil {
			continue
		}
		wg.Add(1)
		go func(conn *net.UDPConn) {
			defer wg.Done()
			if err := conn.Close(); err != nil {
				errCh <- err
			}
		}(c)
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		close(errCh)
		for err := range errCh {
			if err != nil {
				return err
			}
		}
		s.logger.Info("dhcpv4 listeners stopped", zap.Int("count", len(conns)))
		return nil
	}
}

func (s *Server) processDatagram(ctx context.Context, data []byte, addr *net.UDPAddr) {
	messageParser := s.messageParser
	if messageParser == nil {
		messageParser = dhcpv4parser.NewDecoder()
	}
	msg, err := messageParser.Parse(data)
	if err != nil {
		dhcpv4Logger(ctx, s.logger).Debug("dhcpv4 parse failed", zap.Error(err))
		return
	}

	typ := msg.Option(OptionDHCPMessageType)
	if !s.dhcpv4AllowByMAC(msg, addr) {
		return
	}
	if len(typ) != 1 {
		if msg.Op == opBootRequest {
			s.observePacketMetric("REQUEST")
			s.handleBootRequest(ctx, msg, addr)
			return
		}
		dhcpv4Logger(ctx, s.logger).Debug("dhcpv4 missing message type", zap.Uint32("xid", msg.XID))
		return
	}

	switch typ[0] {
	case MessageTypeDiscover:
		s.observePacketMetric("DISCOVER")
		s.handleDiscover(ctx, msg, addr)
	case MessageTypeRequest:
		s.observePacketMetric("REQUEST")
		s.handleRequest(ctx, msg, addr)
	case MessageTypeDecline:
		s.handleDecline(ctx, msg, addr)
	case MessageTypeRelease:
		s.handleRelease(ctx, msg, addr)
	default:
		dhcpv4Logger(ctx, s.logger).Debug("dhcpv4 unsupported type", zap.Uint8("type", typ[0]))
	}
}

func (s *Server) handleDiscover(ctx context.Context, msg *Message, addr *net.UDPAddr) {
	log := dhcpv4Logger(ctx, s.logger)
	packet := s.packetFromMessage(msg)
	if !s.dhcpv4RelayValidateOrReject(ctx, msg, packet, addr) {
		return
	}
	if !s.dhcpv4Option82ValidateOrReject(ctx, msg, packet, addr) {
		return
	}
	if !s.shouldHandleRelay(packet) {
		return
	}
	result, err := s.handler.HandleDiscover(ctx, s.opts.TenantID, packet)
	if err != nil {
		log.Warn("dhcpv4 discover handling failed", zap.Error(err))
		return
	}

	resp, err := s.buildResponse(msg, result, MessageTypeOffer)
	if err != nil {
		log.Warn("dhcpv4 offer build failed", zap.Error(err))
		return
	}
	s.sendResponse(ctx, msg, resp, addr)
}

func (s *Server) handleRequest(ctx context.Context, msg *Message, addr *net.UDPAddr) {
	log := dhcpv4Logger(ctx, s.logger)
	packet := s.packetFromMessage(msg)
	if !s.dhcpv4RelayValidateOrReject(ctx, msg, packet, addr) {
		return
	}
	if !s.dhcpv4Option82ValidateOrReject(ctx, msg, packet, addr) {
		return
	}
	if !s.shouldHandleRelay(packet) {
		return
	}
	result, err := s.handler.HandleRequest(ctx, s.opts.TenantID, packet)
	if err != nil {
		log.Warn("dhcpv4 request handling failed", zap.Error(err))
		s.sendNak(ctx, msg, addr, err.Error())
		return
	}

	resp, err := s.buildResponse(msg, result, MessageTypeAck)
	if err != nil {
		log.Warn("dhcpv4 ack build failed", zap.Error(err))
		s.sendNak(ctx, msg, addr, err.Error())
		return
	}
	s.sendResponse(ctx, msg, resp, addr)
}

func (s *Server) handleBootRequest(ctx context.Context, msg *Message, addr *net.UDPAddr) {
	log := dhcpv4Logger(ctx, s.logger)
	packet := s.packetFromMessage(msg)
	if !s.dhcpv4RelayValidateOrReject(ctx, msg, packet, addr) {
		return
	}
	if !s.dhcpv4Option82ValidateOrReject(ctx, msg, packet, addr) {
		return
	}
	if !s.shouldHandleRelay(packet) {
		return
	}
	result, err := s.handler.HandleBootRequest(ctx, s.opts.TenantID, packet)
	if err != nil {
		log.Warn("bootp handling failed", zap.Error(err))
		return
	}

	resp, err := s.buildResponse(msg, result, MessageTypeAck)
	if err != nil {
		log.Warn("bootp ack build failed", zap.Error(err))
		return
	}
	s.sendResponse(ctx, msg, resp, addr)
}

func (s *Server) handleDecline(ctx context.Context, msg *Message, addr *net.UDPAddr) {
	log := dhcpv4Logger(ctx, s.logger)
	packet := s.packetFromMessage(msg)
	if !s.dhcpv4RelayValidateOrReject(ctx, msg, packet, addr) {
		return
	}
	if !s.dhcpv4Option82ValidateOrReject(ctx, msg, packet, addr) {
		return
	}
	if !s.shouldHandleRelay(packet) {
		return
	}
	if err := s.handler.HandleDecline(ctx, s.opts.TenantID, packet); err != nil {
		log.Warn("dhcpv4 decline handling failed", zap.Error(err))
	}
}

func (s *Server) handleRelease(ctx context.Context, msg *Message, addr *net.UDPAddr) {
	log := dhcpv4Logger(ctx, s.logger)
	packet := s.packetFromMessage(msg)
	if !s.dhcpv4RelayValidateOrReject(ctx, msg, packet, addr) {
		return
	}
	if !s.dhcpv4Option82ValidateOrReject(ctx, msg, packet, addr) {
		return
	}
	if !s.shouldHandleRelay(packet) {
		return
	}
	if err := s.handler.HandleRelease(ctx, s.opts.TenantID, packet); err != nil {
		log.Warn("dhcpv4 release handling failed", zap.Error(err))
	}
}

func (s *Server) shouldHandleRelay(pkt Packet) bool {
	if s.partitioner == nil {
		return true
	}
	meta := relay.BuildMetadata(s.opts.TenantID, pkt.GIAddr, pkt.RelayAgentInfo, 0, nil)
	if s.partitioner.ShouldHandle(meta) {
		return true
	}
	s.logger.Debug("relay partition drop", zap.String("relayId", meta.RelayID))
	return false
}

func (s *Server) buildResponse(req *Message, result *lease.Result, msgType byte) (*Message, error) {
	if s.responseBuilder == nil {
		return nil, errors.New("dhcpv4: response builder is nil")
	}
	return s.responseBuilder.BuildResponse(req, result, msgType)
}

func (s *Server) sendResponse(ctx context.Context, req *Message, msg *Message, addr *net.UDPAddr) {
	payload, err := msg.MarshalBinary()
	if err != nil {
		dhcpv4Logger(ctx, s.logger).Warn("dhcpv4 marshal failed", zap.Error(err))
		return
	}

	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()

	if conn == nil {
		dhcpv4Logger(ctx, s.logger).Warn("dhcpv4 connection not available")
		return
	}
	dest := s.destinationFor(req, addr)
	if dest == nil {
		dhcpv4Logger(ctx, s.logger).Warn("dhcpv4 destination missing")
		return
	}

	if _, err := conn.WriteToUDP(payload, dest); err != nil {
		dhcpv4Logger(ctx, s.logger).Warn("dhcpv4 send failed", zap.Error(err))
		return
	}
	if msgType := msg.Option(OptionDHCPMessageType); len(msgType) == 1 {
		s.observePacketMetric(dhcpv4PacketTypeName(msgType[0]))
	}
}

func (s *Server) sendNak(ctx context.Context, req *Message, addr *net.UDPAddr, reason string) {
	b := s.responseBuilder
	if b == nil {
		b = builder.NewResponseBuilder(s.opts.ServerIP, encodeMobilityVendorOption)
	}
	resp := b.BuildNAK(req, reason)
	s.sendResponse(ctx, req, resp, addr)
}

func (s *Server) observePacketMetric(packetType string) {
	if s.packetObserver == nil {
		return
	}
	s.packetObserver.ObservePacket(packetType)
}

func dhcpv4PacketTypeName(messageType byte) string {
	b := builder.NewResponseBuilder(net.IPv4zero, nil)
	return b.PacketTypeName(messageType)
}

func (s *Server) packetFromMessage(msg *Message) Packet {
	rawOption82 := msg.Option(OptionRelayAgentInfo)
	pkt := Packet{
		XID:             msg.XID,
		CHAddr:          append(net.HardwareAddr{}, msg.CHAddr...),
		RequestPhase:    dhcpv4ClassifyRequestPhase(msg),
		CIAddr:          append(net.IP{}, msg.CIAddr...),
		GIAddr:          append(net.IP{}, msg.GIAddr...),
		Options:         msg.Options,
		Broadcast:       msg.Flags&flagBroadcast != 0,
		Option82Present: len(rawOption82) > 0,
	}
	if parsed, err := dhcpv4Option82Parse(rawOption82); err != nil {
		pkt.Option82Error = err.Error()
	} else {
		pkt.Option82CircuitID = parsed.CircuitID
		pkt.Option82RemoteID = parsed.RemoteID
		pkt.Option82SubscriberID = parsed.SubscriberID
	}
	if s.relayParser != nil {
		if attrs, err := s.relayParser.Decode(rawOption82); err != nil {
			s.logger.Debug("option82 decode failed", zap.Error(err))
			if len(attrs) > 0 {
				pkt.RelayAgentInfo = attrs
			}
		} else if len(attrs) > 0 {
			pkt.RelayAgentInfo = attrs
		}
	}
	if pkt.Option82Present {
		if pkt.RelayAgentInfo == nil {
			pkt.RelayAgentInfo = make(map[string]string, 5)
		}
		if pkt.Option82CircuitID != "" {
			pkt.RelayAgentInfo["circuit-id"] = pkt.Option82CircuitID
			pkt.RelayAgentInfo["agent.circuit-id"] = pkt.Option82CircuitID
		}
		if pkt.Option82RemoteID != "" {
			pkt.RelayAgentInfo["remote-id"] = pkt.Option82RemoteID
			pkt.RelayAgentInfo["agent.remote-id"] = pkt.Option82RemoteID
		}
		if pkt.Option82SubscriberID != "" {
			pkt.RelayAgentInfo["subscriber-id"] = pkt.Option82SubscriberID
		}
	}

	if cid := msg.Option(OptionClientIdentifier); len(cid) > 0 {
		pkt.ClientID = formatClientID(cid)
	}
	if vc := msg.Option(OptionVendorClass); len(vc) > 0 {
		pkt.VendorClass = dhcpv4option.ParseVendorClass(vc)
	}
	if uc := msg.Option(OptionUserClass); len(uc) > 0 {
		pkt.UserClass = dhcpv4option.DecodeUserClass(uc)
	}
	if rip := msg.Option(OptionRequestedIPAddress); len(rip) == 4 {
		pkt.RequestedIP = append(net.IP{}, rip...)
	}
	if _, ok := pkt.Options[OptionDHCPMessageType]; !ok {
		pkt.IsBOOTP = true
	}
	pkt.IPClass = classifyIPv4(pkt)

	return pkt
}

func dhcpv4ClassifyRequestPhase(msg *Message) string {
	if msg == nil {
		return dhcpv4RequestPhaseRequest
	}
	if len(msg.Option(OptionDHCPMessageType)) != 1 || msg.Option(OptionDHCPMessageType)[0] != MessageTypeRequest {
		return dhcpv4RequestPhaseRequest
	}
	ciaddr := firstIPv4(msg.CIAddr)
	if ciaddr == nil || ciaddr.Equal(net.IPv4zero) {
		return dhcpv4RequestPhaseRequest
	}
	hasServerID := len(msg.Option(OptionServerIdentifier)) == 4
	if hasServerID {
		return dhcpv4RequestPhaseRequest
	}
	if msg.Flags&flagBroadcast != 0 {
		return dhcpv4RequestPhaseRebind
	}
	return dhcpv4RequestPhaseRenew
}

func newReplyFromRequest(req *Message) *Message {
	return builder.NewReplyFromRequest(req)
}

func (s *Server) destinationFor(req *Message, fallback *net.UDPAddr) *net.UDPAddr {
	if gi := firstIPv4(req.GIAddr); gi != nil {
		return &net.UDPAddr{IP: gi, Port: 67}
	}
	if req.Flags&flagBroadcast != 0 || fallback == nil || fallback.IP == nil || fallback.IP.Equal(net.IPv4zero) {
		return &net.UDPAddr{IP: net.IPv4bcast, Port: 68}
	}
	if ci := firstIPv4(req.CIAddr); ci != nil {
		return &net.UDPAddr{IP: ci, Port: 68}
	}
	return fallback
}

func classifyIPv4(pkt Packet) string {
	addr := pickIPv4(pkt.RequestedIP, pkt.CIAddr, pkt.GIAddr)
	if addr == nil {
		return ""
	}
	first := addr[0]
	switch {
	case first < 128:
		return "A"
	case first < 192:
		return "B"
	case first < 224:
		return "C"
	default:
		return ""
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func defaultListenerFanout() int {
	cpus := runtime.NumCPU()
	if cpus < 1 {
		return 1
	}
	if cpus > 16 {
		return 16
	}
	return cpus
}

func cloneUDPAddr(addr *net.UDPAddr) *net.UDPAddr {
	if addr == nil {
		return nil
	}
	dup := &net.UDPAddr{Port: addr.Port, Zone: addr.Zone}
	if addr.IP != nil {
		dup.IP = append(net.IP(nil), addr.IP...)
	}
	return dup
}

func pickIPv4(candidates ...net.IP) net.IP {
	for _, cand := range candidates {
		if cand == nil {
			continue
		}
		ip := net.IP(cand)
		if len(ip) == net.IPv4len {
			if !ip.Equal(net.IPv4zero) {
				return ip
			}
			continue
		}
		if v4 := ip.To4(); v4 != nil && !v4.Equal(net.IPv4zero) {
			return v4
		}
	}
	return nil
}

func firstIPv4(ip net.IP) net.IP {
	if ip == nil {
		return nil
	}
	v4 := net.IP(ip).To4()
	if v4 == nil {
		return nil
	}
	if v4.Equal(net.IPv4zero) {
		return nil
	}
	return v4
}

func decodeUserClass(data []byte) string {
	return dhcpv4option.DecodeUserClass(data)
}

func sanitizeString(data []byte) string {
	return dhcpv4option.SanitizeASCII(data)
}

func encodeUint32(v uint32) []byte {
	return dhcpv4option.EncodeUint32(v)
}

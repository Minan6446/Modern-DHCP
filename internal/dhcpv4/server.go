package dhcpv4

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/lease"
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
	RelayParser    relay.Option82Parser
	Partitioner    *relay.Partitioner
	WorkerCount    int
	QueueDepth     int
	ListenerFanout int
	RXBatchSize    int
	ReusePort      bool
}

// Server wraps UDP I/O and delegates business logic to Handler.
type Server struct {
	opts        Options
	relayParser relay.Option82Parser
	partitioner *relay.Partitioner
	handler     *Handler
	logger      *zap.Logger

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
	parser := opts.RelayParser
	if parser == nil {
		parser = relay.NewDefaultOption82Parser()
	}
	return &Server{opts: opts, relayParser: parser, partitioner: opts.Partitioner, handler: handler, logger: logger}
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
	msg, err := ParseMessage(data)
	if err != nil {
		s.logger.Debug("dhcpv4 parse failed", zap.Error(err))
		return
	}

	typ := msg.Option(OptionDHCPMessageType)
	if len(typ) != 1 {
		if msg.Op == opBootRequest {
			s.handleBootRequest(ctx, msg, addr)
			return
		}
		s.logger.Debug("dhcpv4 missing message type", zap.Uint32("xid", msg.XID))
		return
	}

	switch typ[0] {
	case MessageTypeDiscover:
		s.handleDiscover(ctx, msg, addr)
	case MessageTypeRequest:
		s.handleRequest(ctx, msg, addr)
	case MessageTypeDecline:
		s.handleDecline(ctx, msg)
	default:
		s.logger.Debug("dhcpv4 unsupported type", zap.Uint8("type", typ[0]))
	}
}

func (s *Server) handleDiscover(ctx context.Context, msg *Message, addr *net.UDPAddr) {
	packet := s.packetFromMessage(msg)
	if !s.shouldHandleRelay(packet) {
		return
	}
	result, err := s.handler.HandleDiscover(ctx, s.opts.TenantID, packet)
	if err != nil {
		s.logger.Warn("dhcpv4 discover handling failed", zap.Error(err))
		return
	}

	resp, err := s.buildResponse(msg, result, MessageTypeOffer)
	if err != nil {
		s.logger.Warn("dhcpv4 offer build failed", zap.Error(err))
		return
	}
	s.sendResponse(msg, resp, addr)
}

func (s *Server) handleRequest(ctx context.Context, msg *Message, addr *net.UDPAddr) {
	packet := s.packetFromMessage(msg)
	if !s.shouldHandleRelay(packet) {
		return
	}
	result, err := s.handler.HandleRequest(ctx, s.opts.TenantID, packet)
	if err != nil {
		s.logger.Warn("dhcpv4 request handling failed", zap.Error(err))
		s.sendNak(msg, addr, err.Error())
		return
	}

	resp, err := s.buildResponse(msg, result, MessageTypeAck)
	if err != nil {
		s.logger.Warn("dhcpv4 ack build failed", zap.Error(err))
		s.sendNak(msg, addr, err.Error())
		return
	}
	s.sendResponse(msg, resp, addr)
}

func (s *Server) handleBootRequest(ctx context.Context, msg *Message, addr *net.UDPAddr) {
	packet := s.packetFromMessage(msg)
	if !s.shouldHandleRelay(packet) {
		return
	}
	result, err := s.handler.HandleBootRequest(ctx, s.opts.TenantID, packet)
	if err != nil {
		s.logger.Warn("bootp handling failed", zap.Error(err))
		return
	}

	resp, err := s.buildResponse(msg, result, MessageTypeAck)
	if err != nil {
		s.logger.Warn("bootp ack build failed", zap.Error(err))
		return
	}
	s.sendResponse(msg, resp, addr)
}

func (s *Server) handleDecline(ctx context.Context, msg *Message) {
	packet := s.packetFromMessage(msg)
	if !s.shouldHandleRelay(packet) {
		return
	}
	if err := s.handler.HandleDecline(ctx, s.opts.TenantID, packet); err != nil {
		s.logger.Warn("dhcpv4 decline handling failed", zap.Error(err))
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
	if result == nil || result.Lease == nil {
		return nil, errors.New("dhcpv4: missing lease result")
	}

	leaseIP := net.ParseIP(result.Lease.IPAddress)
	if leaseIP == nil {
		return nil, errors.New("dhcpv4: invalid lease ip")
	}

	resp := newReplyFromRequest(req)
	resp.YIAddr = leaseIP.To4()
	resp.SIAddr = padIP(s.opts.ServerIP)
	resp.SetOption(OptionDHCPMessageType, []byte{msgType})
	resp.SetOption(OptionServerIdentifier, padIP(s.opts.ServerIP))
	leaseTime := result.Profile.DefaultDuration
	if leaseTime <= 0 {
		leaseTime = time.Hour
	}
	resp.SetOption(OptionIPAddressLeaseTime, encodeUint32(uint32(leaseTime.Seconds())))
	if result.RenewalTime > 0 {
		resp.SetOption(OptionRenewalTime, encodeUint32(uint32(result.RenewalTime.Seconds())))
	}
	if result.RebindingTime > 0 {
		resp.SetOption(OptionRebindingTime, encodeUint32(uint32(result.RebindingTime.Seconds())))
	}
	if payload := encodeMobilityVendorOption(result); len(payload) > 0 {
		resp.SetOption(OptionVendorVIVendorInfo, payload)
	}
	return resp, nil
}

func (s *Server) sendResponse(req *Message, msg *Message, addr *net.UDPAddr) {
	payload, err := msg.MarshalBinary()
	if err != nil {
		s.logger.Warn("dhcpv4 marshal failed", zap.Error(err))
		return
	}

	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()

	if conn == nil {
		s.logger.Warn("dhcpv4 connection not available")
		return
	}
	dest := s.destinationFor(req, addr)
	if dest == nil {
		s.logger.Warn("dhcpv4 destination missing")
		return
	}

	if _, err := conn.WriteToUDP(payload, dest); err != nil {
		s.logger.Warn("dhcpv4 send failed", zap.Error(err))
	}
}

func (s *Server) sendNak(req *Message, addr *net.UDPAddr, reason string) {
	resp := newReplyFromRequest(req)
	resp.YIAddr = net.IPv4zero
	resp.SetOption(OptionDHCPMessageType, []byte{MessageTypeNak})
	resp.SIAddr = padIP(s.opts.ServerIP)
	resp.SetOption(OptionServerIdentifier, padIP(s.opts.ServerIP))
	if reason != "" {
		if len(reason) > 255 {
			reason = reason[:255]
		}
		resp.SetOption(OptionMessage, []byte(reason))
	}
	s.sendResponse(req, resp, addr)
}

func (s *Server) packetFromMessage(msg *Message) Packet {
	pkt := Packet{
		XID:       msg.XID,
		CHAddr:    append(net.HardwareAddr{}, msg.CHAddr...),
		CIAddr:    append(net.IP{}, msg.CIAddr...),
		GIAddr:    append(net.IP{}, msg.GIAddr...),
		Options:   msg.Options,
		Broadcast: msg.Flags&flagBroadcast != 0,
	}
	if s.relayParser != nil {
		if attrs, err := s.relayParser.Decode(msg.Option(OptionRelayAgentInfo)); err != nil {
			s.logger.Debug("option82 decode failed", zap.Error(err))
			if len(attrs) > 0 {
				pkt.RelayAgentInfo = attrs
			}
		} else if len(attrs) > 0 {
			pkt.RelayAgentInfo = attrs
		}
	}

	if cid := msg.Option(OptionClientIdentifier); len(cid) > 0 {
		pkt.ClientID = formatClientID(cid)
	}
	if vc := msg.Option(OptionVendorClass); len(vc) > 0 {
		pkt.VendorClass = sanitizeString(vc)
	}
	if uc := msg.Option(OptionUserClass); len(uc) > 0 {
		pkt.UserClass = decodeUserClass(uc)
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

func newReplyFromRequest(req *Message) *Message {
	resp := &Message{
		Op:      opBootReply,
		HType:   req.HType,
		HLen:    req.HLen,
		XID:     req.XID,
		Secs:    req.Secs,
		Flags:   req.Flags,
		SIAddr:  make([]byte, len(req.SIAddr)),
		GIAddr:  append(net.IP{}, req.GIAddr...),
		CHAddr:  append(net.HardwareAddr{}, req.CHAddr...),
		Options: make(map[byte][]byte),
	}
	return resp
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
	if len(data) == 0 {
		return ""
	}

	var classes []string
	for i := 0; i < len(data); {
		l := int(data[i])
		i++
		if l == 0 || i+l > len(data) {
			break
		}
		classes = append(classes, sanitizeString(data[i:i+l]))
		i += l
	}
	return strings.Join(classes, ",")
}

func sanitizeString(data []byte) string {
	var b strings.Builder
	for _, c := range data {
		if c >= 32 && c <= 126 {
			b.WriteByte(c)
		}
	}
	return b.String()
}

func encodeUint32(v uint32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, v)
	return buf
}

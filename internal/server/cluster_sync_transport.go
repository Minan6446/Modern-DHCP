package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"go.uber.org/zap"
)

type clusterSyncWireMessage struct {
	TxID       string `json:"txId"`
	LeaseID    string `json:"leaseId,omitempty"`
	Type       string `json:"type"`
	SourceNode string `json:"sourceNode"`
	TargetNode string `json:"targetNode"`
	Phase      string `json:"phase"`
	SentAt     string `json:"sentAt"`
}

type clusterSyncWireAck struct {
	TxID    string `json:"txId"`
	Phase   string `json:"phase,omitempty"`
	Ack     bool   `json:"ack"`
	Error   string `json:"error,omitempty"`
	AckAt   string `json:"ackAt"`
	Latency int    `json:"latencyMs,omitempty"`
}

func (s *HTTPServer) sendClusterSyncBndupd(ctx context.Context, msg clusterSyncWireMessage, timeout time.Duration) (time.Duration, error) {
	if s == nil {
		return 0, fmt.Errorf("cluster sync transport unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	peerAddr := s.clusterSyncPeerAddress()
	if peerAddr == "" {
		return 0, fmt.Errorf("cluster peer address is not configured")
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}

	dialer := &net.Dialer{Timeout: timeout}
	started := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", peerAddr)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	prepare := msg
	prepare.Phase = "prepare"
	prepare.SentAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := enc.Encode(prepare); err != nil {
		return 0, err
	}

	var ack clusterSyncWireAck
	if err := dec.Decode(&ack); err != nil {
		return 0, err
	}
	if !ack.Ack {
		if strings.TrimSpace(ack.Error) != "" {
			return 0, fmt.Errorf(ack.Error)
		}
		return 0, fmt.Errorf("peer rejected PREPARE")
	}
	if strings.TrimSpace(ack.Phase) != "" && !strings.EqualFold(strings.TrimSpace(ack.Phase), "prepare_ack") {
		return 0, fmt.Errorf("peer prepare ack phase mismatch")
	}

	commit := msg
	commit.Phase = "commit"
	commit.SentAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := enc.Encode(commit); err != nil {
		return 0, err
	}

	ack = clusterSyncWireAck{}
	if err := dec.Decode(&ack); err != nil {
		return 0, err
	}
	if !ack.Ack {
		if strings.TrimSpace(ack.Error) != "" {
			return 0, fmt.Errorf(ack.Error)
		}
		return 0, fmt.Errorf("peer rejected COMMIT")
	}
	if strings.TrimSpace(ack.Phase) != "" && !strings.EqualFold(strings.TrimSpace(ack.Phase), "commit_ack") {
		return 0, fmt.Errorf("peer commit ack phase mismatch")
	}
	if strings.TrimSpace(ack.TxID) != "" && ack.TxID != msg.TxID {
		return 0, fmt.Errorf("peer ack tx mismatch")
	}

	latency := time.Since(started)
	if ack.Latency > 0 {
		latency = time.Duration(ack.Latency) * time.Millisecond
	}
	return latency, nil
}

func (s *HTTPServer) clusterSyncPeerAddress() string {
	cfg := s.currentHAConfig()
	host := strings.TrimSpace(cfg.Partner.Address)
	if host == "" {
		return ""
	}
	port := cfg.Partner.Port
	if port <= 0 {
		port = 647
	}
	return net.JoinHostPort(host, fmt.Sprintf("%d", port))
}

func (s *HTTPServer) startClusterSyncResponder() {
	if s == nil {
		return
	}
	if s.currentSyncTransportMode() != "tcp" {
		return
	}

	s.syncResponderMu.Lock()
	if s.syncResponderListener != nil {
		s.syncResponderMu.Unlock()
		return
	}
	s.syncResponderMu.Unlock()

	cfg := s.currentHAConfig()
	port := cfg.Partner.Port
	if port <= 0 {
		return
	}

	listenAddr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		s.setClusterSyncResponderError(err.Error())
		if s.logger != nil {
			s.logger.Warn("cluster sync responder disabled", zap.String("listen", listenAddr), zap.Error(err))
		}
		return
	}

	s.syncResponderMu.Lock()
	s.syncResponderListener = listener
	s.syncResponderDone = make(chan struct{})
	s.syncResponderLastError = ""
	done := s.syncResponderDone
	s.syncResponderMu.Unlock()

	if s.logger != nil {
		s.logger.Info("cluster sync responder started", zap.String("listen", listenAddr))
	}

	go func() {
		defer close(done)
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				if !strings.Contains(strings.ToLower(acceptErr.Error()), "closed") {
					s.setClusterSyncResponderError(acceptErr.Error())
				}
				return
			}
			go s.handleClusterSyncConn(conn)
		}
	}()
}

func (s *HTTPServer) refreshClusterSyncResponder() {
	if s == nil {
		return
	}
	if s.currentSyncTransportMode() == "tcp" {
		s.startClusterSyncResponder()
		return
	}
	s.stopClusterSyncResponder()
}

func (s *HTTPServer) stopClusterSyncResponder() {
	if s == nil {
		return
	}

	s.syncResponderMu.Lock()
	listener := s.syncResponderListener
	done := s.syncResponderDone
	s.syncResponderListener = nil
	s.syncResponderDone = nil
	s.syncResponderLastError = ""
	s.syncResponderMu.Unlock()

	if listener != nil {
		_ = listener.Close()
	}
	if done != nil {
		<-done
	}
}

func (s *HTTPServer) handleClusterSyncConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	var prepare clusterSyncWireMessage
	if err := dec.Decode(&prepare); err != nil {
		s.setClusterSyncResponderError(err.Error())
		return
	}
	if !strings.EqualFold(strings.TrimSpace(prepare.Phase), "prepare") {
		_ = enc.Encode(clusterSyncWireAck{TxID: clusterSyncMessageKey(prepare), Phase: "prepare_ack", Ack: false, Error: "invalid prepare phase", AckAt: time.Now().UTC().Format(time.RFC3339Nano)})
		return
	}
	prepareKey := clusterSyncMessageKey(prepare)
	if err := enc.Encode(clusterSyncWireAck{TxID: prepareKey, Phase: "prepare_ack", Ack: true, AckAt: time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
		s.setClusterSyncResponderError(err.Error())
		return
	}

	var commit clusterSyncWireMessage
	if err := dec.Decode(&commit); err != nil {
		s.setClusterSyncResponderError(err.Error())
		return
	}
	if !strings.EqualFold(strings.TrimSpace(commit.Phase), "commit") {
		_ = enc.Encode(clusterSyncWireAck{TxID: clusterSyncMessageKey(commit), Phase: "commit_ack", Ack: false, Error: "invalid commit phase", AckAt: time.Now().UTC().Format(time.RFC3339Nano)})
		return
	}
	commitKey := clusterSyncMessageKey(commit)
	if commitKey == "" || commitKey != prepareKey {
		_ = enc.Encode(clusterSyncWireAck{TxID: commitKey, Phase: "commit_ack", Ack: false, Error: "commit tx mismatch", AckAt: time.Now().UTC().Format(time.RFC3339Nano)})
		return
	}
	if err := enc.Encode(clusterSyncWireAck{TxID: commitKey, Phase: "commit_ack", Ack: true, AckAt: time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
		s.setClusterSyncResponderError(err.Error())
	}
}

func clusterSyncMessageKey(msg clusterSyncWireMessage) string {
	if key := strings.TrimSpace(msg.TxID); key != "" {
		return key
	}
	return strings.TrimSpace(msg.LeaseID)
}

func (s *HTTPServer) setClusterSyncResponderError(message string) {
	if s == nil {
		return
	}
	s.syncResponderMu.Lock()
	s.syncResponderLastError = strings.TrimSpace(message)
	s.syncResponderMu.Unlock()
}

func (s *HTTPServer) clusterSyncResponderStatus() (bool, string) {
	if s == nil {
		return false, ""
	}
	s.syncResponderMu.Lock()
	defer s.syncResponderMu.Unlock()
	return s.syncResponderListener != nil, s.syncResponderLastError
}

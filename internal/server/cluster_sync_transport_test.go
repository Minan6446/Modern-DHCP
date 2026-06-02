package server

import (
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestHandleClusterSyncConnPrepareCommitSuccess(t *testing.T) {
	s := &HTTPServer{}
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.handleClusterSyncConn(serverConn)
	}()

	enc := json.NewEncoder(clientConn)
	dec := json.NewDecoder(clientConn)

	prepare := clusterSyncWireMessage{TxID: "tx-1", Phase: "prepare", Type: "lease"}
	if err := enc.Encode(prepare); err != nil {
		t.Fatalf("encode prepare: %v", err)
	}
	var prepareAck clusterSyncWireAck
	if err := dec.Decode(&prepareAck); err != nil {
		t.Fatalf("decode prepare ack: %v", err)
	}
	if !prepareAck.Ack || prepareAck.Phase != "prepare_ack" {
		t.Fatalf("unexpected prepare ack: %+v", prepareAck)
	}

	commit := clusterSyncWireMessage{TxID: "tx-1", Phase: "commit", Type: "lease"}
	if err := enc.Encode(commit); err != nil {
		t.Fatalf("encode commit: %v", err)
	}
	var commitAck clusterSyncWireAck
	if err := dec.Decode(&commitAck); err != nil {
		t.Fatalf("decode commit ack: %v", err)
	}
	if !commitAck.Ack || commitAck.Phase != "commit_ack" {
		t.Fatalf("unexpected commit ack: %+v", commitAck)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("server handler did not return")
	}
}

func TestHandleClusterSyncConnCommitMismatch(t *testing.T) {
	s := &HTTPServer{}
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.handleClusterSyncConn(serverConn)
	}()

	enc := json.NewEncoder(clientConn)
	dec := json.NewDecoder(clientConn)

	if err := enc.Encode(clusterSyncWireMessage{TxID: "tx-1", Phase: "prepare"}); err != nil {
		t.Fatalf("encode prepare: %v", err)
	}
	var prepareAck clusterSyncWireAck
	if err := dec.Decode(&prepareAck); err != nil {
		t.Fatalf("decode prepare ack: %v", err)
	}
	if !prepareAck.Ack {
		t.Fatalf("prepare should be accepted: %+v", prepareAck)
	}

	if err := enc.Encode(clusterSyncWireMessage{TxID: "tx-2", Phase: "commit"}); err != nil {
		t.Fatalf("encode commit: %v", err)
	}
	var commitAck clusterSyncWireAck
	if err := dec.Decode(&commitAck); err != nil {
		t.Fatalf("decode commit ack: %v", err)
	}
	if commitAck.Ack {
		t.Fatalf("commit mismatch should be rejected: %+v", commitAck)
	}
	if commitAck.Phase != "commit_ack" {
		t.Fatalf("unexpected phase: %+v", commitAck)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("server handler did not return")
	}
}

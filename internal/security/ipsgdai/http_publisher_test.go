package ipsgdai

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

func TestHTTPPublisherPublishesWithSignature(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	bodyCh := make(chan []byte, 1)
	headerCh := make(chan string, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		bodyCh <- payload
		headerCh <- r.Header.Get("X-IPSG-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	publisher, err := NewHTTPPublisher(HTTPPublisherOptions{
		URL:    srv.URL,
		Secret: "topsecret",
		Retry:  1,
		Client: srv.Client(),
		Logger: zaptest.NewLogger(t),
	})
	if err != nil {
		t.Fatalf("publisher init failed: %v", err)
	}

	snapshot := LeaseSnapshot{TenantID: "t1", MACAddress: "aa:bb:cc:dd:ee:ff"}
	if err := publisher.Publish(ctx, snapshot); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	payload := <-bodyCh
	sig := <-headerCh

	mac := hmac.New(sha256.New, []byte("topsecret"))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	if sig != expected {
		t.Fatalf("unexpected signature: got %q want %q", sig, expected)
	}
}

func TestHTTPPublisherRetriesOnFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	publisher, err := NewHTTPPublisher(HTTPPublisherOptions{
		URL:    srv.URL,
		Secret: "",
		Retry:  2,
		Client: srv.Client(),
		Logger: zaptest.NewLogger(t),
	})
	if err != nil {
		t.Fatalf("publisher init failed: %v", err)
	}

	if err := publisher.Publish(ctx, LeaseSnapshot{TenantID: "t"}); err != nil {
		t.Fatalf("publish should have succeeded after retry: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 attempts, got %d", calls)
	}
}

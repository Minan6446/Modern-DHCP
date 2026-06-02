package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *HTTPServer) ensureClusterAPITLSAssets(primaryURL string, ipAddresses []string) (APITLSOptions, bool, error) {
	tlsOpts := s.options.APITLS
	tlsOpts.Enabled = true
	if strings.TrimSpace(tlsOpts.CertFile) != "" && strings.TrimSpace(tlsOpts.KeyFile) != "" {
		if fileExists(strings.TrimSpace(tlsOpts.CertFile)) && fileExists(strings.TrimSpace(tlsOpts.KeyFile)) {
			s.options.APITLS = tlsOpts
			return tlsOpts, false, nil
		}
	}

	dir := strings.TrimSpace(tlsOpts.SelfSignedDir)
	if dir == "" {
		dir = filepath.Join("data", "cluster", "pki")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return tlsOpts, false, fmt.Errorf("cluster tls: create cert dir: %w", err)
	}
	if strings.TrimSpace(tlsOpts.CertFile) == "" {
		tlsOpts.CertFile = filepath.Join(dir, "api-server-cert.pem")
	}
	if strings.TrimSpace(tlsOpts.KeyFile) == "" {
		tlsOpts.KeyFile = filepath.Join(dir, "api-server-key.pem")
	}

	hosts := collectTLSHosts(primaryURL, ipAddresses)
	certPEM, keyPEM, err := generateSelfSignedServerCert(hosts, ipAddresses)
	if err != nil {
		return tlsOpts, false, err
	}
	if err := os.WriteFile(tlsOpts.CertFile, certPEM, 0o600); err != nil {
		return tlsOpts, false, fmt.Errorf("cluster tls: write cert: %w", err)
	}
	if err := os.WriteFile(tlsOpts.KeyFile, keyPEM, 0o600); err != nil {
		return tlsOpts, false, fmt.Errorf("cluster tls: write key: %w", err)
	}
	s.options.APITLS = tlsOpts
	return tlsOpts, true, nil
}

func generateSelfSignedServerCert(hosts []string, ipAddresses []string) ([]byte, []byte, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), crand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("cluster tls: generate key: %w", err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := crand.Int(crand.Reader, serialLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("cluster tls: generate serial: %w", err)
	}
	notBefore := time.Now().UTC().Add(-5 * time.Minute)
	notAfter := notBefore.Add(365 * 24 * time.Hour)
	tpl := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   firstNonEmpty(firstString(hosts), firstString(ipAddresses), "modern-dhcp-cluster"),
			Organization: []string{"Modern-DHCP"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	for _, host := range hosts {
		trimmed := strings.TrimSpace(host)
		if trimmed == "" {
			continue
		}
		tpl.DNSNames = append(tpl.DNSNames, trimmed)
	}
	for _, rawIP := range ipAddresses {
		trimmed := strings.TrimSpace(rawIP)
		if trimmed == "" {
			continue
		}
		if parsed := net.ParseIP(trimmed); parsed != nil {
			tpl.IPAddresses = append(tpl.IPAddresses, parsed)
		}
	}
	if len(tpl.DNSNames) == 0 && len(tpl.IPAddresses) == 0 {
		tpl.DNSNames = []string{"localhost"}
	}
	der, err := x509.CreateCertificate(crand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("cluster tls: create cert: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("cluster tls: marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	return certPEM, keyPEM, nil
}

func collectTLSHosts(primaryURL string, ipAddresses []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(ipAddresses)+2)
	appendHost := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		if parsed := net.ParseIP(trimmed); parsed != nil {
			return
		}
		if _, ok := seen[trimmed]; ok {
			return
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	if parsed, err := url.Parse(strings.TrimSpace(primaryURL)); err == nil && parsed != nil {
		appendHost(parsed.Hostname())
	}
	appendHost("localhost")
	appendHost("127.0.0.1")
	for _, raw := range ipAddresses {
		appendHost(raw)
	}
	return result
}

func fileExists(path string) bool {
	info, err := os.Stat(strings.TrimSpace(path))
	return err == nil && !info.IsDir()
}

func firstString(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

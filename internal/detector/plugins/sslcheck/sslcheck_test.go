package sslcheck

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/detector"
)

func TestPlugin_Type(t *testing.T) {
	p := New()
	if p.Type() != "ssl_check" {
		t.Errorf("Type() = %q, want %q", p.Type(), "ssl_check")
	}
}

func TestPlugin_ValidateConfig(t *testing.T) {
	p := New()
	if err := p.ValidateConfig(json.RawMessage(`{"host":"example.com"}`)); err != nil {
		t.Errorf("valid config should pass: %v", err)
	}
	if err := p.ValidateConfig(json.RawMessage(`{}`)); err == nil {
		t.Error("empty config should fail")
	}
	if err := p.ValidateConfig(json.RawMessage(`{"host":"example.com","port":99999}`)); err == nil {
		t.Error("invalid port should fail")
	}
}

func TestPlugin_Execute(t *testing.T) {
	p := New()
	signals, err := p.Execute(context.Background(), detector.ExecuteRequest{
		ProjectID: 1,
		Config:    json.RawMessage(`{"host":"example.com"}`),
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
	// Should resolve successfully for example.com
	if signals[0].Severity == "error" {
		t.Logf("Warning: ssl_check got error for example.com: %s (may be network issue)", signals[0].Message)
	}
}

func TestPlugin_ExecuteReportsUntrustedCertificate(t *testing.T) {
	cert, err := untrustedLocalhostCertificate(t)
	if err != nil {
		t.Fatalf("create test certificate: %v", err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatalf("listen tls: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			if tlsConn, ok := conn.(*tls.Conn); ok {
				_ = tlsConn.Handshake()
			}
			_ = conn.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port

	p := New()
	signals, err := p.Execute(context.Background(), detector.ExecuteRequest{
		ProjectID: 1,
		Config:    json.RawMessage(`{"host":"127.0.0.1","port":` + strconv.Itoa(port) + `}`),
		Now:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
	if signals[0].Severity != "error" || signals[0].Status != "firing" {
		t.Fatalf("expected firing error signal, got severity=%q status=%q message=%q", signals[0].Severity, signals[0].Status, signals[0].Message)
	}
	if _, ok := signals[0].Fields["cert_verify_error"]; !ok {
		t.Fatalf("expected cert_verify_error field, got fields=%v", signals[0].Fields)
	}
}

func TestPlugin_HealthCheck(t *testing.T) {
	p := New()
	if err := p.HealthCheck(context.Background()); err != nil {
		t.Logf("HealthCheck warning (network dependent): %v", err)
	}
}

func untrustedLocalhostCertificate(t *testing.T) (tls.Certificate, error) {
	t.Helper()
	now := time.Now().Add(-time.Hour)
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test local ca"},
		NotBefore:             now,
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, err
	}

	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    now,
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caTemplate, &leafKey.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(leafKey)})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, err
	}
	// Keep the CA generated and unused by system roots; the server intentionally
	// sends only the leaf so the detector must report an unknown authority.
	_ = caDER
	return cert, nil
}

func TestPlugin_Lifecycle(t *testing.T) {
	p := New()
	if err := p.OnActivate(context.Background(), nil); err != nil {
		t.Errorf("OnActivate failed: %v", err)
	}
	if err := p.OnDeactivate(context.Background()); err != nil {
		t.Errorf("OnDeactivate failed: %v", err)
	}
}

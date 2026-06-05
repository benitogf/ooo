package ooo

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeTestCA generates a self-signed CA plus a leaf certificate with an
// IP SAN, writes them to dir, and returns (caPath, certPath, keyPath).
// This mirrors what the private certificate authority issues: a leaf
// stamped with the machine's IP so https://<ip> validates.
func writeTestCA(t *testing.T, dir string, ip net.IP) (string, string, string) {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-authority"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: ip.String()},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{ip},
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}

	caPath := filepath.Join(dir, "ca.crt")
	certPath := filepath.Join(dir, "leaf.crt")
	keyPath := filepath.Join(dir, "leaf.key")

	writePEM(t, caPath, "CERTIFICATE", caDER)
	writePEM(t, certPath, "CERTIFICATE", leafDER)
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		t.Fatal(err)
	}
	writePEM(t, keyPath, "EC PRIVATE KEY", leafKeyDER)

	return caPath, certPath, keyPath
}

func writePEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: blockType, Bytes: der}); err != nil {
		t.Fatal(err)
	}
}

// TestServerTLS proves the full HTTPS loop: a server configured with a
// keypair serves TLS, a client built via NewClient (trusting the CA)
// completes the handshake, and a client that does not trust the CA is
// rejected — confirming the listener is genuinely encrypted, not plain.
func TestServerTLS(t *testing.T) {
	dir := t.TempDir()
	caPath, certPath, keyPath := writeTestCA(t, dir, net.ParseIP("127.0.0.1"))

	server := Server{
		CertFile: certPath,
		KeyFile:  keyPath,
		Silence:  true,
	}
	server.Start("127.0.0.1:0")
	defer server.Close(os.Interrupt)
	if !server.Active() {
		t.Fatal("server did not become active")
	}

	url := "https://" + server.Address + "/"

	// Trusting client: handshake must succeed (any HTTP status is fine —
	// reaching a response proves TLS termination worked).
	trusting, err := NewClient(2*time.Second, caPath)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := trusting.Get(url)
	if err != nil {
		t.Fatalf("trusting client failed over HTTPS: %v", err)
	}
	resp.Body.Close()

	// Non-trusting client: must be rejected at TLS verification.
	untrusting := &http.Client{Timeout: 2 * time.Second}
	if _, err := untrusting.Get(url); err == nil {
		t.Fatal("expected non-trusting client to be rejected, got success")
	} else if !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("expected a certificate verification error, got: %v", err)
	}
}

// TestServerTLSBadKeypairFailsStart proves a bad keypair aborts startup
// rather than silently falling back to a plaintext listener.
func TestServerTLSBadKeypairFailsStart(t *testing.T) {
	server := Server{
		CertFile: "/nonexistent/cert.pem",
		KeyFile:  "/nonexistent/key.pem",
		Silence:  true,
	}
	err := server.StartWithError("127.0.0.1:0")
	if err == nil {
		server.Close(os.Interrupt)
		t.Fatal("expected StartWithError to fail on missing keypair")
	}
	if !strings.Contains(err.Error(), "TLS keypair") {
		t.Fatalf("expected TLS keypair error, got: %v", err)
	}
}

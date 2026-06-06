package client_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/client"
	"github.com/stretchr/testify/require"
)

// genServerTLS mints a self-signed CA + a leaf with an IP SAN, writes the
// leaf keypair to disk for the server, and returns a CertPool trusting the
// CA — mirroring how the private authority issues an internal cert.
func genServerTLS(t *testing.T, ip net.IP) (certPath, keyPath string, caPool *x509.CertPool) {
	t.Helper()
	dir := t.TempDir()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
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
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: ip.String()},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{ip},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	require.NoError(t, err)

	certPath = filepath.Join(dir, "leaf.crt")
	keyPath = filepath.Join(dir, "leaf.key")
	writePEMBlock(t, certPath, "CERTIFICATE", leafDER)
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	require.NoError(t, err)
	writePEMBlock(t, keyPath, "EC PRIVATE KEY", leafKeyDER)

	caPool = x509.NewCertPool()
	caPool.AddCert(caCert)
	return certPath, keyPath, caPool
}

func writePEMBlock(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, pem.Encode(f, &pem.Block{Type: blockType, Bytes: der}))
}

// TestSubscribeListWSSWithTLSConfig proves SubscribeConfig.TLSConfig lets a
// wss subscription trust an internal/self-signed CA: with the CA pool the
// handshake succeeds and the initial snapshot is delivered; without it the
// dial is rejected at TLS verification.
func TestSubscribeListWSSWithTLSConfig(t *testing.T) {
	certPath, keyPath, caPool := genServerTLS(t, net.ParseIP("127.0.0.1"))

	server := ooo.Server{Silence: true, CertFile: certPath, KeyFile: keyPath}
	server.Start("127.0.0.1:0")
	defer server.Close(os.Interrupt)
	require.True(t, server.Active())

	// With the trusting TLSConfig: the wss handshake completes and the
	// initial (empty) snapshot fires OnMessage. A trust failure would hang
	// here until the test times out.
	var wg sync.WaitGroup
	wg.Add(1)
	go client.SubscribeList(client.SubscribeConfig{
		Ctx:       t.Context(),
		Server:    client.Server{Protocol: "wss", Host: server.Address},
		Silence:   true,
		TLSConfig: &tls.Config{RootCAs: caPool},
	}, "devices/*", client.SubscribeListEvents[Device]{
		OnMessage: func([]client.Meta[Device]) { wg.Done() },
	})
	wg.Wait()

	// Without any TLSConfig: the system pool does not trust the self-signed
	// CA, so the dial must be rejected with a certificate error.
	errCh := make(chan error, 4)
	go client.SubscribeList(client.SubscribeConfig{
		Ctx:     t.Context(),
		Server:  client.Server{Protocol: "wss", Host: server.Address},
		Silence: true,
	}, "devices/*", client.SubscribeListEvents[Device]{
		OnMessage: func([]client.Meta[Device]) {},
		OnError: func(e error) {
			select {
			case errCh <- e:
			default:
			}
		},
	})
	select {
	case e := <-errCh:
		require.Error(t, e)
		require.Contains(t, e.Error(), "certificate")
	case <-time.After(5 * time.Second):
		t.Fatal("expected a certificate verification error without TLSConfig, got none")
	}
}

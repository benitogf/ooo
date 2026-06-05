package ooo

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"
)

// LoadCertPool builds an x509 certificate pool seeded with the host's
// system roots and extended with the PEM certificate files at caPaths.
// Use it to trust a private certificate authority in addition to the
// public roots. If the system pool is unavailable (some minimal
// containers) an empty pool is used so only caPaths are trusted.
func LoadCertPool(caPaths ...string) (*x509.CertPool, error) {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	for _, path := range caPaths {
		pem, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("ooo: failed to read CA file %q: %w", path, readErr)
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("ooo: no valid certificate found in CA file %q", path)
		}
	}
	return pool, nil
}

// NewClient returns an http.Client tuned like the server's default
// outbound client, but whose TLS configuration trusts the certificate
// authorities at caPaths (in addition to the system roots). Assign the
// result to Server.Client before Start so inter-service HTTPS calls
// validate certificates signed by a private authority.
//
// A timeout of zero applies the default 10s request timeout.
func NewClient(timeout time.Duration, caPaths ...string) (*http.Client, error) {
	pool, err := LoadCertPool(caPaths...)
	if err != nil {
		return nil, err
	}
	transport := defaultTransport()
	transport.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    pool,
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}, nil
}

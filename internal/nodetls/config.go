// Package nodetls loads the private trust roots and atomic identity files for Worker transport.
package nodetls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"path/filepath"
)

func Roots(path string) (*x509.CertPool, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("node CA file needs an absolute path")
	}
	data, err := os.ReadFile(path)
	pool := x509.NewCertPool()
	if err != nil || !pool.AppendCertsFromPEM(data) {
		return nil, errors.New("invalid node CA file")
	}
	return pool, nil
}

func Identity(path string) (*tls.Certificate, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("node identity file needs an absolute path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("node identity file unavailable")
	}
	pair, err := tls.X509KeyPair(data, data)
	if err != nil {
		return nil, errors.New("invalid node certificate or private key")
	}
	return &pair, nil
}

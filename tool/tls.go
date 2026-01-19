package tool

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"math/rand"
	"time"
)

// GenerateTLSCert generates a self-signed TLS certificate and private key.
func GenerateTLSCert() (pem []byte, keyPem []byte, err error) {
	PrivateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.New(rand.NewSource(time.Now().UnixNano())))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate ECDSA private key: %v", err)
	}
	cert := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "localsend-localCert",
			Organization: []string{"localsend-localCert"},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(time.Hour * 24 * 365),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certBytes, err := x509.CreateCertificate(rand.New(rand.NewSource(time.Now().UnixNano())), &cert, &cert, &PrivateKey.PublicKey, PrivateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create certificate: %v", err)
	}
	privateKeyBytes, err := x509.MarshalECPrivateKey(PrivateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal ECDSA private key: %v", err)
	}
	return certBytes, privateKeyBytes, nil
}

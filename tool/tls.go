package tool

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"math/rand"
	"time"
)

var (
	GenerateTlsSha256Fingerprint string
)

// GetOrCreateFingerprintFromConfig returns the fingerprint based on TLS certificate from config.
// If certificate exists in config, uses its hash. Otherwise generates cert first and returns its hash.
// Also updates the config's CertPEM and KeyPEM fields.
func GetOrCreateFingerprintFromConfig(cfg *AppConfig) string {
	// Try to load existing certificate from config
	if cfg.CertPEM != "" && cfg.KeyPEM != "" {
		certDER, _, err := loadTLSCertFromPEM(cfg.CertPEM, cfg.KeyPEM)
		if err == nil {
			// Certificate exists and valid, calculate fingerprint
			hash := sha256.Sum256(certDER)
			fingerprint := hex.EncodeToString(hash[:16])
			GenerateTlsSha256Fingerprint = fingerprint
			DefaultLogger.Debugf("Fingerprint from existing certificate in config: %s", fingerprint)
			return fingerprint
		}
		// Certificate expired or invalid, will regenerate
		DefaultLogger.Warnf("Certificate in config is invalid or expired: %v, regenerating...", err)
	}

	// No certificate or invalid, generate new one
	DefaultLogger.Debugf("No existing certificate in config, generating new one...")
	certDER, keyDER, err := generateTLSCert()
	if err != nil {
		// Failed to generate, return random fingerprint as fallback
		DefaultLogger.Warnf("Failed to generate TLS certificate: %v, using random fingerprint", err)
		return generateRandomFingerprint()
	}

	// Store certificate PEM in config
	cfg.CertPEM = string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	}))
	cfg.KeyPEM = string(pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	}))

	DefaultLogger.Infof("TLS certificate generated and stored in config")
	return GenerateTlsSha256Fingerprint
}

// generateRandomFingerprint generates a random 32-character fingerprint (fallback)
func generateRandomFingerprint() string {
	b := make([]byte, 16)
	cryptorand.Read(b)
	return hex.EncodeToString(b)
}

// GetOrCreateTLSCertFromConfig loads existing TLS certificate from config or generates a new one.
// Certificate content is stored in config's CertPEM and KeyPEM fields.
func GetOrCreateTLSCertFromConfig(cfg *AppConfig) (certDER []byte, keyDER []byte, err error) {
	// Try to load existing certificate from config
	if cfg.CertPEM != "" && cfg.KeyPEM != "" {
		certDER, keyDER, err = loadTLSCertFromPEM(cfg.CertPEM, cfg.KeyPEM)
		if err == nil {
			// Calculate fingerprint from loaded cert
			hash := sha256.Sum256(certDER)
			GenerateTlsSha256Fingerprint = hex.EncodeToString(hash[:16])
			DefaultLogger.Infof("Loaded existing TLS certificate from config")
			return certDER, keyDER, nil
		}
		// Certificate expired or invalid, will regenerate
		DefaultLogger.Warnf("Certificate in config is invalid or expired: %v, regenerating...", err)
	}

	DefaultLogger.Infof("No existing TLS certificate in config, generating new one...")

	// Generate new certificate
	certDER, keyDER, err = generateTLSCert()
	if err != nil {
		return nil, nil, err
	}

	// Store certificate PEM in config
	cfg.CertPEM = string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	}))
	cfg.KeyPEM = string(pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	}))

	DefaultLogger.Infof("TLS certificate generated and stored in config")
	return certDER, keyDER, nil
}

// loadTLSCertFromPEM loads TLS certificate and key from PEM strings.
// If certificate is expired, returns error.
func loadTLSCertFromPEM(certPEMStr, keyPEMStr string) (certDER []byte, keyDER []byte, err error) {
	// Decode PEM to DER
	certBlock, _ := pem.Decode([]byte(certPEMStr))
	if certBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode certificate PEM")
	}

	keyBlock, _ := pem.Decode([]byte(keyPEMStr))
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode key PEM")
	}

	// Validate certificate is not expired
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse certificate: %v", err)
	}

	if time.Now().After(cert.NotAfter) {
		return nil, nil, fmt.Errorf("certificate has expired")
	}

	return certBlock.Bytes, keyBlock.Bytes, nil
}

// generateTLSCert generates a new self-signed TLS certificate and private key.
func generateTLSCert() (certDER []byte, keyDER []byte, err error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.New(rand.NewSource(time.Now().UnixNano())))
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
		NotAfter:    time.Now().Add(time.Hour * 24 * 365), // 1 year validity
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certBytes, err := x509.CreateCertificate(rand.New(rand.NewSource(time.Now().UnixNano())), &cert, &cert, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create certificate: %v", err)
	}

	privateKeyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal ECDSA private key: %v", err)
	}

	hash := sha256.Sum256(certBytes)
	GenerateTlsSha256Fingerprint = hex.EncodeToString(hash[:16])

	return certBytes, privateKeyBytes, nil
}

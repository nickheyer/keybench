package benchmark

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

type RSABenchmark struct{}

func (r *RSABenchmark) Name() string {
	return "RSA"
}

func (r *RSABenchmark) SupportedKeySizes() []int {
	// Return empty to indicate no size restrictions
	return []int{}
}

func (r *RSABenchmark) GenerateKey(size int) (any, error) {
	return rsa.GenerateKey(rand.Reader, size)
}

func (r *RSABenchmark) GenerateKeyStreaming(size int, writer KeyWriter) error {
	key, err := rsa.GenerateKey(rand.Reader, size)
	if err != nil {
		return fmt.Errorf("failed to generate RSA key: %w", err)
	}
	
	// Encode private key
	privKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	
	if err := pem.Encode(writer, privKeyPEM); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}
	
	// Write separator
	if _, err := writer.Write([]byte("\n")); err != nil {
		return err
	}
	
	// Encode public key
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to marshal public key: %w", err)
	}
	
	pubKeyPEM := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	}
	
	if err := pem.Encode(writer, pubKeyPEM); err != nil {
		return fmt.Errorf("failed to write public key: %w", err)
	}
	
	return nil
}

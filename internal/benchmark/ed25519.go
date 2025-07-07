package benchmark

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

type Ed25519Benchmark struct{}

func (e *Ed25519Benchmark) Name() string {
	return "Ed25519"
}

func (e *Ed25519Benchmark) SupportedKeySizes() []int {
	return []int{256}
}

func (e *Ed25519Benchmark) GenerateKey(size int) (any, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return []any{pub, priv}, nil
}

func (e *Ed25519Benchmark) GenerateKeyStreaming(size int, writer KeyWriter) error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate Ed25519 key: %w", err)
	}
	
	// Encode private key (contains both private and public key material)
	privKeyBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return fmt.Errorf("failed to marshal Ed25519 private key: %w", err)
	}
	
	privKeyPEM := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privKeyBytes,
	}
	
	if err := pem.Encode(writer, privKeyPEM); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}
	
	// Write separator
	if _, err := writer.Write([]byte("\n")); err != nil {
		return err
	}
	
	// Encode public key
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(pub)
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

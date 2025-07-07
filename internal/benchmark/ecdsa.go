package benchmark

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

type ECDSABenchmark struct{}

func (e *ECDSABenchmark) Name() string {
	return "ECDSA"
}

func (e *ECDSABenchmark) SupportedKeySizes() []int {
	return []int{224, 256, 384, 521}
}

func (e *ECDSABenchmark) GenerateKey(size int) (any, error) {
	var curve elliptic.Curve

	switch size {
	case 224:
		curve = elliptic.P224()
	case 256:
		curve = elliptic.P256()
	case 384:
		curve = elliptic.P384()
	case 521:
		curve = elliptic.P521()
	default:
		return nil, fmt.Errorf("unsupported ECDSA key size: %d", size)
	}

	key, err := ecdsa.GenerateKey(curve, rand.Reader)
	return key, err
}

func (e *ECDSABenchmark) GenerateKeyStreaming(size int, writer KeyWriter) error {
	key, err := e.GenerateKey(size)
	if err != nil {
		return err
	}
	
	ecdsaKey := key.(*ecdsa.PrivateKey)
	
	// Encode private key
	privKeyBytes, err := x509.MarshalECPrivateKey(ecdsaKey)
	if err != nil {
		return fmt.Errorf("failed to marshal ECDSA private key: %w", err)
	}
	
	privKeyPEM := &pem.Block{
		Type:  "EC PRIVATE KEY",
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
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&ecdsaKey.PublicKey)
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

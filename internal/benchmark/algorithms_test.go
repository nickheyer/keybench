package benchmark

import (
	"testing"
)

func TestRSABenchmark(t *testing.T) {
	rsa := &RSABenchmark{}

	if rsa.Name() != "RSA" {
		t.Errorf("Expected name RSA, got %s", rsa.Name())
	}

	sizes := rsa.SupportedKeySizes()
	expectedSizes := []int{1024, 2048, 3072, 4096, 8192, 16384}

	if len(sizes) != len(expectedSizes) {
		t.Errorf("Expected %d sizes, got %d", len(expectedSizes), len(sizes))
	}

	// Test key generation for small size
	_, err := rsa.GenerateKey(1024)
	if err != nil {
		t.Errorf("Failed to generate RSA key: %v", err)
	}
}

func TestECDSABenchmark(t *testing.T) {
	ecdsa := &ECDSABenchmark{}

	if ecdsa.Name() != "ECDSA" {
		t.Errorf("Expected name ECDSA, got %s", ecdsa.Name())
	}

	sizes := ecdsa.SupportedKeySizes()
	expectedSizes := []int{224, 256, 384, 521}

	if len(sizes) != len(expectedSizes) {
		t.Errorf("Expected %d sizes, got %d", len(expectedSizes), len(sizes))
	}

	// Test key generation
	_, err := ecdsa.GenerateKey(256)
	if err != nil {
		t.Errorf("Failed to generate ECDSA key: %v", err)
	}

	// Test invalid key size
	_, err = ecdsa.GenerateKey(512)
	if err == nil {
		t.Error("Expected error for invalid ECDSA key size")
	}
}

func TestEd25519Benchmark(t *testing.T) {
	ed := &Ed25519Benchmark{}

	if ed.Name() != "Ed25519" {
		t.Errorf("Expected name Ed25519, got %s", ed.Name())
	}

	sizes := ed.SupportedKeySizes()
	if len(sizes) != 1 || sizes[0] != 256 {
		t.Errorf("Expected [256], got %v", sizes)
	}

	// Test key generation
	_, err := ed.GenerateKey(256)
	if err != nil {
		t.Errorf("Failed to generate Ed25519 key: %v", err)
	}
}

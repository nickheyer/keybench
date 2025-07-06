package benchmark

import (
	"crypto/ed25519"
	"crypto/rand"
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

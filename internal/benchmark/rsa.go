package benchmark

import (
	"crypto/rand"
	"crypto/rsa"
)

type RSABenchmark struct{}

func (r *RSABenchmark) Name() string {
	return "RSA"
}

func (r *RSABenchmark) SupportedKeySizes() []int {
	return []int{1024, 2048, 3072, 4096, 8192, 16384}
}

func (r *RSABenchmark) GenerateKey(size int) (any, error) {
	return rsa.GenerateKey(rand.Reader, size)
}

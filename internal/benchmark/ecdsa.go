package benchmark

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
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

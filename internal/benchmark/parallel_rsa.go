package benchmark

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"sync/atomic"
)

// ParallelRSAGenerator generates RSA keys using multiple cores for a single key
type ParallelRSAGenerator struct {
	workers int
}

// NewParallelRSAGenerator creates a new parallel RSA generator with default workers
func NewParallelRSAGenerator() *ParallelRSAGenerator {
	return NewParallelRSAGeneratorWithWorkers(1) // Default to 1 worker
}

// NewParallelRSAGeneratorWithWorkers creates a new parallel RSA generator with specified workers
func NewParallelRSAGeneratorWithWorkers(workers int) *ParallelRSAGenerator {
	if workers < 1 {
		workers = 1
	}
	return &ParallelRSAGenerator{
		workers: workers,
	}
}

// GenerateKey generates a single RSA key using multiple cores
func (p *ParallelRSAGenerator) GenerateKey(bits int, ctx context.Context) (*rsa.PrivateKey, error) {
	primeSize := bits / 2

	// Generate both primes using a single parallel generator
	primeChan := make(chan *big.Int, 2)
	errChan := make(chan error, 1)

	// Start parallel prime generation that yields 2 primes
	go p.generateTwoPrimesParallel(primeSize, ctx, primeChan, errChan)

	// Collect the two primes
	var primes []*big.Int
	for i := 0; i < 2; i++ {
		select {
		case prime := <-primeChan:
			primes = append(primes, prime)
		case err := <-errChan:
			return nil, fmt.Errorf("failed to generate primes: %w", err)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	p1, q1 := primes[0], primes[1]

	// Ensure p != q (extremely unlikely but check anyway)
	if p1.Cmp(q1) == 0 {
		// Generate one more prime
		prime, err := p.generateSinglePrime(primeSize, ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to regenerate prime: %w", err)
		}
		q1 = prime
	}

	// Order primes
	if p1.Cmp(q1) < 0 {
		p1, q1 = q1, p1
	}

	// Calculate n = p * q
	n := new(big.Int).Mul(p1, q1)

	// Calculate φ(n) = (p-1) * (q-1)
	p1minus1 := new(big.Int).Sub(p1, big.NewInt(1))
	q1minus1 := new(big.Int).Sub(q1, big.NewInt(1))
	phi := new(big.Int).Mul(p1minus1, q1minus1)

	// Choose e (public exponent)
	e := big.NewInt(65537)

	// Calculate d = e^(-1) mod φ(n)
	d := new(big.Int).ModInverse(e, phi)
	if d == nil {
		return nil, fmt.Errorf("failed to calculate private exponent")
	}

	// Calculate precomputed values for CRT
	dp := new(big.Int).Mod(d, p1minus1)
	dq := new(big.Int).Mod(d, q1minus1)
	qinv := new(big.Int).ModInverse(q1, p1)

	// Create RSA private key
	key := &rsa.PrivateKey{
		PublicKey: rsa.PublicKey{
			N: n,
			E: int(e.Int64()),
		},
		D:      d,
		Primes: []*big.Int{p1, q1},
		Precomputed: rsa.PrecomputedValues{
			Dp:        dp,
			Dq:        dq,
			Qinv:      qinv,
			CRTValues: []rsa.CRTValue{},
		},
	}

	// Validate the key
	if err := key.Validate(); err != nil {
		return nil, fmt.Errorf("key validation failed: %w", err)
	}

	return key, nil
}

// generateTwoPrimesParallel generates two primes in parallel and sends them to the channel
func (p *ParallelRSAGenerator) generateTwoPrimesParallel(bits int, ctx context.Context, primeChan chan<- *big.Int, errChan chan<- error) {
	var found atomic.Int32 // Count of primes found
	var stopWorkers atomic.Bool

	// Use all workers to find 2 primes
	for i := 0; i < p.workers; i++ {
		go func(workerID int) {
			// Create a reader with context support
			reader := &contextReader{
				ctx:    ctx,
				reader: rand.Reader,
			}

			for !stopWorkers.Load() {
				// Check context cancellation
				select {
				case <-ctx.Done():
					if found.Load() == 0 {
						select {
						case errChan <- ctx.Err():
						default:
						}
					}
					return
				default:
				}

				// Generate a candidate prime
				candidate, err := rand.Prime(reader, bits)
				if err != nil {
					if found.Load() == 0 {
						select {
						case errChan <- err:
						default:
						}
					}
					return
				}

				// Check if we need more primes
				if found.Add(1) <= 2 {
					select {
					case primeChan <- candidate:
						// If we've found 2 primes, signal workers to stop
						if found.Load() == 2 {
							stopWorkers.Store(true)
						}
					case <-ctx.Done():
						return
					}
				} else {
					// We have enough primes
					return
				}
			}
		}(i)
	}
}

// generateSinglePrime generates a single prime (used for regeneration if p == q)
func (p *ParallelRSAGenerator) generateSinglePrime(bits int, ctx context.Context) (*big.Int, error) {
	reader := &contextReader{
		ctx:    ctx,
		reader: rand.Reader,
	}
	return rand.Prime(reader, bits)
}

// ParallelRSABenchmark uses parallel generation for benchmarking
type ParallelRSABenchmark struct {
	generator *ParallelRSAGenerator
}

func NewParallelRSABenchmark() *ParallelRSABenchmark {
	return &ParallelRSABenchmark{
		generator: NewParallelRSAGenerator(),
	}
}

func (b *ParallelRSABenchmark) Name() string {
	return "RSA-Parallel"
}

func (b *ParallelRSABenchmark) SupportedKeySizes() []int {
	return []int{} // No restrictions
}

func (b *ParallelRSABenchmark) GenerateKey(size int) (any, error) {
	return b.generator.GenerateKey(size, context.Background())
}

func (b *ParallelRSABenchmark) GenerateKeyWithContext(size int, ctx context.Context) (any, error) {
	return b.generator.GenerateKey(size, ctx)
}

func (b *ParallelRSABenchmark) GenerateKeyStreaming(size int, writer KeyWriter) error {
	key, err := b.generator.GenerateKey(size, context.Background())
	if err != nil {
		return err
	}

	// Encode private key
	privKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}

	if err := pem.Encode(writer, privKeyPEM); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}
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

func (b *ParallelRSABenchmark) GenerateKeyWithContextStreaming(size int, ctx context.Context, writer KeyWriter) error {
	key, err := b.generator.GenerateKey(size, ctx)
	if err != nil {
		return err
	}

	// Encode private key
	privKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}

	if err := pem.Encode(writer, privKeyPEM); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}
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

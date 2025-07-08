package benchmark

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// ChunkSize for writing large keys to disk
	ChunkSize = 1024 * 1024 // 1MB chunks

	// FileThreshold - keys larger than this are written to disk
	FileThreshold = 10 * 1024 * 1024 // 10MB

	// MaxMemoryKeys - maximum number of keys to keep in memory
	MaxMemoryKeys = 1000
)

// KeyGenRequest represents a request to generate a key
type KeyGenRequest struct {
	ID       string
	Type     string
	Size     int
	UseFile  bool // Force file storage regardless of size
	Response chan<- KeyGenResponse
	Context  context.Context // Context for cancellation
}

// KeyGenResponse represents the result of key generation
type KeyGenResponse struct {
	ID       string
	Key      interface{}
	FilePath string // Empty if stored in memory
	Size     int64  // Actual size in bytes
	Error    error
	Duration time.Duration
}

// KeyGenWorker manages asynchronous key generation
type KeyGenWorker struct {
	ctx            context.Context
	cancel         context.CancelFunc
	requests       chan KeyGenRequest
	workers        int
	wg             sync.WaitGroup
	tempDir        string
	fileCleanup    *FileCleanupManager
	activeRequests int32
	totalGenerated int64
	bytesGenerated int64
}

// FileCleanupManager handles periodic cleanup of generated files
type FileCleanupManager struct {
	mu       sync.Mutex
	files    map[string]*FileInfo
	maxAge   time.Duration
	interval time.Duration
	stop     chan struct{}
}

// FileInfo tracks file metadata
type FileInfo struct {
	CreatedAt   time.Time
	CompletedAt *time.Time
	InProgress  bool
	RequestID   string
}

// NewKeyGenWorker creates a new key generation worker pool
func NewKeyGenWorker(tempDir string) *KeyGenWorker {
	return NewKeyGenWorkerWithCount(tempDir, 1) // Default to 1 worker
}

// NewKeyGenWorkerWithCount creates a new key generation worker pool with specified workers
func NewKeyGenWorkerWithCount(tempDir string, workers int) *KeyGenWorker {
	ctx, cancel := context.WithCancel(context.Background())
	// Validate workers count
	if workers < 1 {
		workers = 1
	}

	kgw := &KeyGenWorker{
		ctx:         ctx,
		cancel:      cancel,
		requests:    make(chan KeyGenRequest, workers*2),
		workers:     workers,
		tempDir:     tempDir,
		fileCleanup: NewFileCleanupManager(30*time.Minute, 5*time.Minute),
	}

	// Ensure temp directory exists
	os.MkdirAll(tempDir, 0755)

	return kgw
}

// NewFileCleanupManager creates a new file cleanup manager
func NewFileCleanupManager(maxAge, interval time.Duration) *FileCleanupManager {
	fcm := &FileCleanupManager{
		files:    make(map[string]*FileInfo),
		maxAge:   maxAge,
		interval: interval,
		stop:     make(chan struct{}),
	}
	go fcm.run()
	return fcm
}

// Start begins the worker pool
func (kgw *KeyGenWorker) Start() {
	// Start generation workers
	for i := 0; i < kgw.workers; i++ {
		kgw.wg.Add(1)
		go kgw.generationWorker()
	}
}

// Stop gracefully shuts down the worker pool
func (kgw *KeyGenWorker) Stop() {
	kgw.cancel()

	// Drain any pending requests
	go func() {
		for req := range kgw.requests {
			// Send cancellation response
			resp := KeyGenResponse{
				ID:    req.ID,
				Error: fmt.Errorf("worker shutdown"),
			}
			select {
			case req.Response <- resp:
			default:
			}
		}
	}()

	close(kgw.requests)
	kgw.wg.Wait()
	kgw.fileCleanup.Stop()
}

// Submit submits a key generation request
func (kgw *KeyGenWorker) Submit(req KeyGenRequest) error {
	select {
	case kgw.requests <- req:
		atomic.AddInt32(&kgw.activeRequests, 1)
		return nil
	case <-kgw.ctx.Done():
		return fmt.Errorf("worker pool is shutting down")
	default:
		return fmt.Errorf("worker pool is full")
	}
}

// GetStats returns current statistics
func (kgw *KeyGenWorker) GetStats() (activeRequests int32, totalGenerated int64, bytesGenerated int64) {
	return atomic.LoadInt32(&kgw.activeRequests),
		atomic.LoadInt64(&kgw.totalGenerated),
		atomic.LoadInt64(&kgw.bytesGenerated)
}

// generationWorker processes key generation requests
func (kgw *KeyGenWorker) generationWorker() {
	defer kgw.wg.Done()

	for {
		select {
		case req, ok := <-kgw.requests:
			if !ok {
				return
			}

			// Check for cancellation before processing
			if req.Context != nil {
				select {
				case <-req.Context.Done():
					resp := KeyGenResponse{
						ID:    req.ID,
						Error: fmt.Errorf("request cancelled"),
					}
					select {
					case req.Response <- resp:
					case <-kgw.ctx.Done():
						return
					}
					atomic.AddInt32(&kgw.activeRequests, -1)
					continue
				default:
				}
			}

			start := time.Now()
			resp := kgw.processRequest(req)
			resp.Duration = time.Since(start)

			// Send response
			select {
			case req.Response <- resp:
			case <-kgw.ctx.Done():
				return
			}

			// Update stats
			atomic.AddInt32(&kgw.activeRequests, -1)
			if resp.Error == nil {
				atomic.AddInt64(&kgw.totalGenerated, 1)
				atomic.AddInt64(&kgw.bytesGenerated, resp.Size)
			}

		case <-kgw.ctx.Done():
			return
		}
	}
}

// processRequest handles a single key generation request
func (kgw *KeyGenWorker) processRequest(req KeyGenRequest) KeyGenResponse {
	resp := KeyGenResponse{ID: req.ID}

	switch req.Type {
	case "RSA":
		resp.Key, resp.Size, resp.FilePath, resp.Error = kgw.generateRSAKey(req.Size, req.UseFile, req.Context)
	case "RSA-Parallel":
		resp.Key, resp.Size, resp.FilePath, resp.Error = kgw.generateParallelRSAKey(req.Size, req.UseFile, req.Context)
	case "ECDSA":
		// TODO: Implement ECDSA with chunking
		resp.Error = fmt.Errorf("ECDSA chunking not yet implemented")
	case "Ed25519":
		// TODO: Implement Ed25519 with chunking
		resp.Error = fmt.Errorf("Ed25519 chunking not yet implemented")
	default:
		resp.Error = fmt.Errorf("unknown key type: %s", req.Type)
	}

	// Register file for cleanup if created
	if resp.FilePath != "" && resp.Error == nil {
		kgw.fileCleanup.MarkCompleted(resp.FilePath)
	}

	return resp
}

// generateParallelRSAKey generates an RSA key using parallel processing
func (kgw *KeyGenWorker) generateParallelRSAKey(bits int, forceFile bool, ctx context.Context) (key interface{}, size int64, filePath string, err error) {
	// Use the parallel generator with the same number of workers as the pool
	generator := NewParallelRSAGeneratorWithWorkers(kgw.workers)

	// Check context before generation
	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, 0, "", ctx.Err()
		default:
		}
	}

	// Generate the key using parallel processing
	privateKey, err := generator.GenerateKey(bits, ctx)
	if err != nil {
		return nil, 0, "", fmt.Errorf("failed to generate parallel RSA key: %w", err)
	}

	// Encode to PEM to get size
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	pemBytes := pem.EncodeToMemory(pemBlock)
	size = int64(len(pemBytes))

	// Decide storage method
	if forceFile || size > FileThreshold || bits >= 8192 {
		// Write to file
		filename := fmt.Sprintf("rsa_parallel_%d_%d.pem", bits, time.Now().UnixNano())
		filePath = filepath.Join(kgw.tempDir, filename)

		// Register file as in-progress
		requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())
		kgw.fileCleanup.RegisterInProgress(filePath, requestID)

		err = kgw.writeChunked(filePath, pemBytes)
		if err != nil {
			kgw.fileCleanup.Remove(filePath)
			return nil, 0, "", fmt.Errorf("failed to write key to file: %w", err)
		}

		// Don't keep large keys in memory
		return nil, size, filePath, nil
	}

	// Return in-memory key
	return privateKey, size, "", nil
}

// generateRSAKey generates an RSA key with optional file storage
func (kgw *KeyGenWorker) generateRSAKey(bits int, forceFile bool, ctx context.Context) (key interface{}, size int64, filePath string, err error) {
	// For very large keys, use progressive generation
	if bits > 32768 {
		return kgw.generateLargeRSAKey(bits, ctx)
	}

	// Check context before generation
	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, 0, "", ctx.Err()
		default:
		}
	}

	// For keys that might take a while, use interruptible generation
	if bits >= 4096 {
		privateKey, err := generateInterruptibleRSAKey(bits, ctx)
		if err != nil {
			return nil, 0, "", fmt.Errorf("failed to generate RSA key: %w", err)
		}
		// Continue with the rest of the function using privateKey
		pemBlock := &pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
		}
		pemBytes := pem.EncodeToMemory(pemBlock)
		size = int64(len(pemBytes))

		// Decide storage method
		if forceFile || size > FileThreshold || bits >= 8192 {
			// Write to file
			filename := fmt.Sprintf("rsa_%d_%d.pem", bits, time.Now().UnixNano())
			filePath = filepath.Join(kgw.tempDir, filename)

			// Register file as in-progress
			requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())
			kgw.fileCleanup.RegisterInProgress(filePath, requestID)

			err = kgw.writeChunked(filePath, pemBytes)
			if err != nil {
				kgw.fileCleanup.Remove(filePath)
				return nil, 0, "", fmt.Errorf("failed to write key to file: %w", err)
			}

			// Don't keep large keys in memory
			return nil, size, filePath, nil
		}

		// Return in-memory key
		return privateKey, size, "", nil
	}

	// Standard generation for smaller keys
	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, 0, "", fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Encode to PEM
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	pemBytes := pem.EncodeToMemory(pemBlock)
	size = int64(len(pemBytes))

	// Decide storage method
	if forceFile || size > FileThreshold || bits >= 8192 {
		// Write to file
		filename := fmt.Sprintf("rsa_%d_%d.pem", bits, time.Now().UnixNano())
		filePath = filepath.Join(kgw.tempDir, filename)

		// Register file as in-progress
		requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())
		kgw.fileCleanup.RegisterInProgress(filePath, requestID)

		err = kgw.writeChunked(filePath, pemBytes)
		if err != nil {
			kgw.fileCleanup.Remove(filePath)
			return nil, 0, "", fmt.Errorf("failed to write key to file: %w", err)
		}

		// Don't keep large keys in memory
		return nil, size, filePath, nil
	}

	// Return in-memory key
	return privateKey, size, "", nil
}

// generateLargeRSAKey generates very large RSA keys using custom implementation
func (kgw *KeyGenWorker) generateLargeRSAKey(bits int, ctx context.Context) (key interface{}, size int64, filePath string, err error) {
	// For extremely large keys, we need to generate and write directly to file
	// to avoid memory exhaustion

	filename := fmt.Sprintf("rsa_%d_%d.pem", bits, time.Now().UnixNano())
	filePath = filepath.Join(kgw.tempDir, filename)

	// Register file as in-progress
	requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())
	kgw.fileCleanup.RegisterInProgress(filePath, requestID)

	// Create the file
	file, err := os.Create(filePath)
	if err != nil {
		kgw.fileCleanup.Remove(filePath)
		return nil, 0, "", fmt.Errorf("failed to create file: %w", err)
	}
	defer func() {
		file.Close()
		if err != nil {
			os.Remove(filePath) // Clean up on error
			kgw.fileCleanup.Remove(filePath)
		}
	}()

	// Generate large RSA key with custom parameters
	// Use streaming generation to avoid memory issues
	size, err = kgw.generateAndStreamLargeRSAKey(bits, file, ctx)
	if err != nil {
		return nil, 0, "", fmt.Errorf("failed to generate large RSA key: %w", err)
	}

	return nil, size, filePath, nil
}

// generateAndStreamLargeRSAKey generates and streams a large RSA key to a writer
func (kgw *KeyGenWorker) generateAndStreamLargeRSAKey(bits int, w io.Writer, ctx context.Context) (int64, error) {
	// For keys larger than what crypto/rsa supports natively,
	// we'll use a modified approach that generates very large primes

	if bits <= 65536 {
		// For keys up to 65536 bits, we can still use the standard library
		// but with careful memory management
		return kgw.generateStandardLargeRSAKey(bits, w, ctx)
	}

	// For truly massive keys, implement custom prime generation
	return kgw.generateCustomRSAKey(bits, w, ctx)
}

// generateStandardLargeRSAKey uses the standard library for large but supported key sizes
func (kgw *KeyGenWorker) generateStandardLargeRSAKey(bits int, w io.Writer, ctx context.Context) (int64, error) {
	// Generate the key with cancellation support
	key, err := generateInterruptibleRSAKey(bits, ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Marshal and write the private key
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(key)

	// Create PEM block
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}

	// Write private key using PEM encoder directly to avoid memory allocation
	written := int64(0)

	// Create a counting writer to track bytes written
	cw := &countingWriter{w: w, written: &written}

	// Encode directly to the writer
	if err := pem.Encode(cw, pemBlock); err != nil {
		return written, fmt.Errorf("failed to encode private key: %w", err)
	}

	// Also write the public key
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return written, fmt.Errorf("failed to marshal public key: %w", err)
	}

	publicPemBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}

	// Write separator
	if _, err := cw.Write([]byte("\n")); err != nil {
		return written, err
	}

	// Encode public key directly to writer
	if err := pem.Encode(cw, publicPemBlock); err != nil {
		return written, fmt.Errorf("failed to encode public key: %w", err)
	}

	return written, nil
}

// generateCustomRSAKey implements custom RSA key generation for extremely large keys
func (kgw *KeyGenWorker) generateCustomRSAKey(bits int, w io.Writer, ctx context.Context) (int64, error) {
	// Generate two large primes p and q
	primeSize := bits / 2

	// Check cancellation
	if ctx != nil {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}
	}

	p, err := generateLargePrimeWithContext(primeSize, ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to generate prime p: %w", err)
	}

	// Check cancellation again
	if ctx != nil {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}
	}

	q, err := generateLargePrimeWithContext(primeSize, ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to generate prime q: %w", err)
	}

	// Ensure p != q
	for p.Cmp(q) == 0 {
		// Check cancellation in loop
		if ctx != nil {
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			default:
			}
		}

		q, err = generateLargePrimeWithContext(primeSize, ctx)
		if err != nil {
			return 0, fmt.Errorf("failed to regenerate prime q: %w", err)
		}
	}

	// Calculate n = p * q
	n := new(big.Int).Mul(p, q)

	// Calculate φ(n) = (p-1) * (q-1)
	p1 := new(big.Int).Sub(p, big.NewInt(1))
	q1 := new(big.Int).Sub(q, big.NewInt(1))
	phi := new(big.Int).Mul(p1, q1)

	// Choose e (public exponent) - standard is 65537
	e := big.NewInt(65537)

	// Ensure gcd(e, φ(n)) = 1
	gcd := new(big.Int).GCD(nil, nil, e, phi)
	if gcd.Cmp(big.NewInt(1)) != 0 {
		// If not coprime, use a different e
		e = big.NewInt(3)
		for {
			gcd = new(big.Int).GCD(nil, nil, e, phi)
			if gcd.Cmp(big.NewInt(1)) == 0 {
				break
			}
			e.Add(e, big.NewInt(2))
		}
	}

	// Calculate d = e^(-1) mod φ(n)
	d := new(big.Int).ModInverse(e, phi)
	if d == nil {
		return 0, fmt.Errorf("failed to calculate private exponent")
	}

	// Create custom RSA key structure
	key := &customRSAKey{
		N: n,
		E: e,
		D: d,
		P: p,
		Q: q,
	}

	// Encode and write the key
	return encodeCustomRSAKey(key, w)
}

// customRSAKey represents a custom RSA key for large sizes
type customRSAKey struct {
	N *big.Int // modulus
	E *big.Int // public exponent
	D *big.Int // private exponent
	P *big.Int // prime 1
	Q *big.Int // prime 2
}

// generateInterruptibleRSAKey generates an RSA key with context cancellation support
func generateInterruptibleRSAKey(bits int, ctx context.Context) (*rsa.PrivateKey, error) {
	// Use a custom random reader that checks for cancellation
	reader := &contextReader{
		ctx:    ctx,
		reader: rand.Reader,
	}

	return rsa.GenerateKey(reader, bits)
}

// contextReader wraps a reader with context cancellation checks
type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (cr *contextReader) Read(p []byte) (n int, err error) {
	// Check for cancellation
	if cr.ctx != nil {
		select {
		case <-cr.ctx.Done():
			return 0, cr.ctx.Err()
		default:
		}
	}

	// Read in smaller chunks to allow more frequent cancellation checks
	chunkSize := 1024 // 1KB chunks
	if len(p) <= chunkSize {
		return cr.reader.Read(p)
	}

	// Read in chunks
	totalRead := 0
	for totalRead < len(p) {
		// Check cancellation before each chunk
		if cr.ctx != nil {
			select {
			case <-cr.ctx.Done():
				return totalRead, cr.ctx.Err()
			default:
			}
		}

		remaining := len(p) - totalRead
		if remaining > chunkSize {
			remaining = chunkSize
		}

		n, err := cr.reader.Read(p[totalRead : totalRead+remaining])
		totalRead += n

		if err != nil {
			return totalRead, err
		}
	}

	return totalRead, nil
}

// generateLargePrimeWithContext generates a large prime with context support
func generateLargePrimeWithContext(bits int, ctx context.Context) (*big.Int, error) {
	reader := &contextReader{
		ctx:    ctx,
		reader: rand.Reader,
	}

	return rand.Prime(reader, bits)
}

// encodeCustomRSAKey encodes a custom RSA key to PEM format
func encodeCustomRSAKey(key *customRSAKey, w io.Writer) (int64, error) {
	written := int64(0)

	// Write custom header indicating this is a large RSA key
	header := fmt.Sprintf("-----BEGIN LARGE RSA PRIVATE KEY (%d bits)-----\n", key.N.BitLen())
	n, err := w.Write([]byte(header))
	if err != nil {
		return written, err
	}
	written += int64(n)

	// Write key components in a simple format
	// Format: component_name:base64_encoded_value
	components := map[string]*big.Int{
		"N": key.N,
		"E": key.E,
		"D": key.D,
		"P": key.P,
		"Q": key.Q,
	}

	for name, value := range components {
		line := fmt.Sprintf("%s:%s\n", name, value.Text(62)) // Base 62 encoding
		n, err := w.Write([]byte(line))
		if err != nil {
			return written, err
		}
		written += int64(n)
	}

	// Write footer
	footer := "-----END LARGE RSA PRIVATE KEY-----\n"
	n, err = w.Write([]byte(footer))
	if err != nil {
		return written, err
	}
	written += int64(n)

	return written, nil
}

// countingWriter wraps an io.Writer and counts bytes written
type countingWriter struct {
	w       io.Writer
	written *int64
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	*cw.written += int64(n)
	return n, err
}

// writeChunked writes data to file in chunks
func (kgw *KeyGenWorker) writeChunked(filePath string, data []byte) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write in chunks
	for i := 0; i < len(data); i += ChunkSize {
		end := i + ChunkSize
		if end > len(data) {
			end = len(data)
		}

		_, err := file.Write(data[i:end])
		if err != nil {
			os.Remove(filePath) // Clean up on error
			return err
		}
	}

	return file.Sync()
}

// FileCleanupManager methods

func (fcm *FileCleanupManager) run() {
	ticker := time.NewTicker(fcm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fcm.cleanup()
		case <-fcm.stop:
			return
		}
	}
}

func (fcm *FileCleanupManager) RegisterInProgress(filePath, requestID string) {
	fcm.mu.Lock()
	defer fcm.mu.Unlock()
	fcm.files[filePath] = &FileInfo{
		CreatedAt:  time.Now(),
		InProgress: true,
		RequestID:  requestID,
	}
}

func (fcm *FileCleanupManager) MarkCompleted(filePath string) {
	fcm.mu.Lock()
	defer fcm.mu.Unlock()
	if info, exists := fcm.files[filePath]; exists {
		now := time.Now()
		info.CompletedAt = &now
		info.InProgress = false
	}
}

func (fcm *FileCleanupManager) Remove(filePath string) {
	fcm.mu.Lock()
	defer fcm.mu.Unlock()
	delete(fcm.files, filePath)
}

func (fcm *FileCleanupManager) cleanup() {
	fcm.mu.Lock()
	defer fcm.mu.Unlock()

	now := time.Now()
	for path, info := range fcm.files {
		// Skip files that are still in progress
		if info.InProgress {
			continue
		}

		// Use completion time if available, otherwise creation time
		referenceTime := info.CreatedAt
		if info.CompletedAt != nil {
			referenceTime = *info.CompletedAt
		}

		if now.Sub(referenceTime) > fcm.maxAge {
			os.Remove(path)
			delete(fcm.files, path)
		}
	}
}

// TerminateRequest cancels all files associated with a request and cleans them up
func (fcm *FileCleanupManager) TerminateRequest(requestID string) {
	fcm.mu.Lock()
	defer fcm.mu.Unlock()

	for path, info := range fcm.files {
		if info.RequestID == requestID {
			os.Remove(path)
			delete(fcm.files, path)
		}
	}
}

func (fcm *FileCleanupManager) Stop() {
	close(fcm.stop)
}

// GetActiveFiles returns the list of currently tracked files
func (fcm *FileCleanupManager) GetActiveFiles() []string {
	fcm.mu.Lock()
	defer fcm.mu.Unlock()

	files := make([]string, 0, len(fcm.files))
	for path := range fcm.files {
		files = append(files, path)
	}
	return files
}

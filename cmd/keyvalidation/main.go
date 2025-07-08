package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <keyfile.pem>\n", os.Args[0])
		os.Exit(1)
	}

	keyFile := os.Args[1]

	// Read
	pemData, err := os.ReadFile(keyFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading key file: %v\n", err)
		os.Exit(1)
	}

	// Decode PEM
	block, _ := pem.Decode(pemData)
	if block == nil {
		fmt.Fprintf(os.Stderr, "Failed to parse PEM block\n")
		os.Exit(1)
	}

	// Parse
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing key: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Key file: %s\n", keyFile)
	fmt.Printf("Key size: %d bits\n", key.N.BitLen())
	fmt.Printf("Public exponent: %d\n", key.E)

	// Validate basic math
	fmt.Println("\nValidating mathematical properties...")

	// Check n = p * q
	n_calculated := new(big.Int).Mul(key.Primes[0], key.Primes[1])
	if n_calculated.Cmp(key.N) == 0 {
		fmt.Println("✓ n = p × q")
	} else {
		fmt.Println("✗ n ≠ p × q")
	}

	// Try a small encrypt/decrypt
	fmt.Println("\nTesting encryption/decryption...")
	message := []byte("test")

	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, &key.PublicKey, message)
	if err != nil {
		fmt.Printf("✗ Encrypt error: %v\n", err)
		os.Exit(1)
	}

	plaintext, err := key.Decrypt(nil, ciphertext, nil)
	if err != nil {
		fmt.Printf("✗ Decrypt error: %v\n", err)
		os.Exit(1)
	}

	if string(plaintext) == string(message) {
		fmt.Printf("✓ Successfully encrypted and decrypted: \"%s\"\n", plaintext)
	} else {
		fmt.Printf("✗ Decryption mismatch\n")
	}

	fmt.Println("\nKey validation complete!")
}

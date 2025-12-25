package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

var (
	ErrInvalidKey         = errors.New("encryption key must be 32 bytes for AES-256")
	ErrCiphertextTooShort = errors.New("ciphertext too short")
	ErrDecryptionFailed   = errors.New("decryption failed")
)

// Encryptor handles AES-256-GCM encryption/decryption.
// GCM (Galois/Counter Mode) provides both confidentiality and authenticity.
type Encryptor struct {
	key []byte
}

func NewEncryptor(key []byte) (*Encryptor, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKey
	}
	return &Encryptor{key: key}, nil
}

// NewEncryptorFromString creates an Encryptor from a string key.
// The string is padded or truncated to 32 bytes.
// For production, use a proper 32-byte key.
func NewEncryptorFromString(keyStr string) (*Encryptor, error) {
	key := make([]byte, 32)
	copy(key, []byte(keyStr))
	return NewEncryptor(key)
}

// Encrypt encrypts plaintext using AES-256-GCM.
// Returns base64-encoded ciphertext (safe for database storage).
func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	// Create AES cipher
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt and append nonce
	// Seal appends the ciphertext and tag to the nonce
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// Return as base64 for safe storage
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts base64-encoded ciphertext produced by Encrypt.
// Returns the original plaintext.
func (e *Encryptor) Decrypt(ciphertextB64 string) (string, error) {
	if ciphertextB64 == "" {
		return "", nil
	}

	// Decode base64
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	// Create AES cipher
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Check minimum length (nonce + tag)
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize+gcm.Overhead() {
		return "", ErrCiphertextTooShort
	}

	// Split nonce and ciphertext
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrDecryptionFailed
	}

	return string(plaintext), nil
}

// GenerateKey generates a cryptographically secure 32-byte key.
// Use this to generate a key for production.
func GenerateKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}
	return key, nil
}

// GenerateKeyBase64 generates a key and returns it as base64.
// Useful for generating keys to put in .env files.
func GenerateKeyBase64() (string, error) {
	key, err := GenerateKey()
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

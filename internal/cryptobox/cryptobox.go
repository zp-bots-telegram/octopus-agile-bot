// Package cryptobox is a very small wrapper around AES-256-GCM for symmetric
// encryption of short strings (per-user Octopus API keys, refresh tokens, …) at rest.
// The ciphertext format is: nonce || aead-seal(plaintext). The caller is responsible
// for supplying a 32-byte key derived from the ENCRYPTION_KEY env var.
package cryptobox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

var ErrKeySize = errors.New("encryption key must be exactly 32 bytes")

type Cipher struct {
	aead cipher.AEAD
}

func New(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, ErrKeySize
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher.NewGCM: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(nonce)+len(plaintext)+c.aead.Overhead())
	out = append(out, nonce...)
	return c.aead.Seal(out, nonce, plaintext, nil), nil
}

func (c *Cipher) Decrypt(ciphertext []byte) ([]byte, error) {
	n := c.aead.NonceSize()
	if len(ciphertext) < n {
		return nil, fmt.Errorf("ciphertext too short (%d < %d)", len(ciphertext), n)
	}
	nonce, sealed := ciphertext[:n], ciphertext[n:]
	return c.aead.Open(nil, nonce, sealed, nil)
}

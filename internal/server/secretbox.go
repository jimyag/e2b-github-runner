package server

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const encryptedSecretPrefix = "v1:"

func encryptSecret(plaintext, keyMaterial string) (string, error) {
	plaintext = strings.TrimSpace(plaintext)
	if plaintext == "" {
		return "", fmt.Errorf("secret is required")
	}
	block, err := secretCipher(keyMaterial)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	out := append(nonce, sealed...)
	return encryptedSecretPrefix + base64.RawURLEncoding.EncodeToString(out), nil
}

func decryptSecret(ciphertext, keyMaterial string) (string, error) {
	ciphertext = strings.TrimSpace(ciphertext)
	if !strings.HasPrefix(ciphertext, encryptedSecretPrefix) {
		return "", fmt.Errorf("unsupported encrypted secret format")
	}
	block, err := secretCipher(keyMaterial)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(ciphertext, encryptedSecretPrefix))
	if err != nil {
		return "", err
	}
	if len(raw) <= gcm.NonceSize() {
		return "", fmt.Errorf("encrypted secret is truncated")
	}
	nonce, sealed := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func secretCipher(keyMaterial string) (cipher.Block, error) {
	keyMaterial = strings.TrimSpace(keyMaterial)
	if keyMaterial == "" {
		return nil, fmt.Errorf("auth.encryption_key is required to encrypt secrets")
	}
	sum := sha256.Sum256([]byte(keyMaterial))
	return aes.NewCipher(sum[:])
}

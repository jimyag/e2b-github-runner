package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	obfuscatedSecretPrefix = "RUNNERD_ENC(v1:"
	obfuscatedSecretSuffix = ")"
	maskedSecret           = "******"
)

var obfuscationKey = sha256.Sum256([]byte("qiniu-ci-runner/config-obfuscation/v1"))

// Secret is a sensitive configuration value. Its default text, JSON, YAML, and
// structured-log representations are masked; Value must be called explicitly
// when the application needs the plaintext.
type Secret string

func (s Secret) Value() string { return string(s) }

func (Secret) String() string { return maskedSecret }

func (Secret) Format(state fmt.State, verb rune) {
	if verb == 'q' {
		_, _ = fmt.Fprintf(state, "%q", maskedSecret)
		return
	}
	_, _ = fmt.Fprint(state, maskedSecret)
}

func (Secret) LogValue() slog.Value { return slog.StringValue(maskedSecret) }

func (Secret) MarshalJSON() ([]byte, error) { return json.Marshal(maskedSecret) }

func (s *Secret) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	plaintext, err := revealObfuscatedSecret(value)
	if err != nil {
		return err
	}
	*s = Secret(plaintext)
	return nil
}

func (Secret) MarshalYAML() (any, error) { return maskedSecret, nil }

func (s *Secret) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("secret must be a scalar value")
	}
	plaintext, err := revealObfuscatedSecret(node.Value)
	if err != nil {
		return err
	}
	*s = Secret(plaintext)
	return nil
}

// ObfuscateSecret returns a portable configuration value that does not expose
// plaintext on casual inspection. The key is embedded in runnerd, so this is
// intentionally obfuscation rather than a security boundary.
func ObfuscateSecret(plaintext []byte) (string, error) {
	gcm, err := obfuscationGCM()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return obfuscatedSecretPrefix + base64.RawURLEncoding.EncodeToString(sealed) + obfuscatedSecretSuffix, nil
}

func revealObfuscatedSecret(value string) (string, error) {
	if !strings.HasPrefix(value, "RUNNERD_ENC(") {
		return value, nil
	}
	if !strings.HasPrefix(value, obfuscatedSecretPrefix) || !strings.HasSuffix(value, obfuscatedSecretSuffix) {
		return "", fmt.Errorf("unsupported obfuscated secret format")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSuffix(strings.TrimPrefix(value, obfuscatedSecretPrefix), obfuscatedSecretSuffix))
	if err != nil {
		return "", fmt.Errorf("decode obfuscated secret: %w", err)
	}
	gcm, err := obfuscationGCM()
	if err != nil {
		return "", err
	}
	if len(raw) <= gcm.NonceSize() {
		return "", fmt.Errorf("obfuscated secret is truncated")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt obfuscated secret: %w", err)
	}
	return string(plaintext), nil
}

func obfuscationGCM() (cipher.AEAD, error) {
	block, err := aes.NewCipher(obfuscationKey[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

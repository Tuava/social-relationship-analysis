package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

const prefix = "enc:v1:"

func keyBytes(raw string) ([]byte, error) {
	key, err := hex.DecodeString(strings.TrimSpace(raw))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("SRA_CONFIG_ENCRYPTION_KEY must be 64 hex characters")
	}
	return key, nil
}

func Encrypt(raw, masterKey string) (string, error) {
	if raw == "" {
		return "", nil
	}
	key, err := keyBytes(masterKey)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
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
	ciphertext := gcm.Seal(nil, nonce, []byte(raw), nil)
	return prefix + base64.RawStdEncoding.EncodeToString(append(nonce, ciphertext...)), nil
}

func Decrypt(value, masterKey string) (string, error) {
	if !strings.HasPrefix(value, prefix) {
		return value, nil
	}
	key, err := keyBytes(masterKey)
	if err != nil {
		return "", err
	}
	encoded := strings.TrimPrefix(value, prefix)
	data, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("encrypted configuration value is truncated")
	}
	plaintext, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt configuration value: %w", err)
	}
	return string(plaintext), nil
}

func IsEncrypted(value string) bool { return strings.HasPrefix(value, prefix) }

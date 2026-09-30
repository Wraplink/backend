package token

import (
	"crypto/rand"
	"encoding/base64"
)

func Generate(bytes int) (string, error) {
	value := make([]byte, bytes)

	if _, err := rand.Read(value); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(value), nil
}

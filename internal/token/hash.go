package token

import (
	"crypto/sha256"
	"encoding/hex"
)

func Hash(value string) string {
	hash := sha256.Sum256(
		[]byte(value),
	)

	return hex.EncodeToString(
		hash[:],
	)
}

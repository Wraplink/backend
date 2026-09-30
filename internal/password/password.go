package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	memory      uint32 = 64 * 1024
	iterations  uint32 = 3
	parallelism uint8  = 4
	saltLength         = 16
	keyLength          = 32
)

func Hash(password string) (string, error) {
	if password == "" {
		return "", errors.New("password cannot be empty")
	}

	salt := make([]byte, saltLength)

	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf(
			"generate password salt: %w",
			err,
		)
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		iterations,
		memory,
		parallelism,
		keyLength,
	)

	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memory,
		iterations,
		parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func Verify(password string, encoded string) bool {
	parts := strings.Split(encoded, "$")

	//:
	// ["", "argon2id", "v=19", "m=65536,t=3,p=4", salt, hash]
	if len(parts) != 6 {
		return false
	}

	if parts[1] != "argon2id" {
		return false
	}

	if parts[2] != "v=19" {
		return false
	}

	params := make(map[string]string)

	for _, parameter := range strings.Split(parts[3], ",") {
		values := strings.SplitN(parameter, "=", 2)

		if len(values) != 2 {
			return false
		}

		params[values[0]] = values[1]
	}

	memoryValue, err := strconv.ParseUint(
		params["m"],
		10,
		32,
	)
	if err != nil {
		return false
	}

	iterationsValue, err := strconv.ParseUint(
		params["t"],
		10,
		32,
	)
	if err != nil {
		return false
	}

	parallelismValue, err := strconv.ParseUint(
		params["p"],
		10,
		8,
	)
	if err != nil {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}

	if len(salt) < 16 || len(salt) > 64 {
		return false
	}

	if len(expectedHash) == 0 {
		return false
	}

	actualHash := argon2.IDKey(
		[]byte(password),
		salt,
		uint32(iterationsValue),
		uint32(memoryValue),
		uint8(parallelismValue),
		uint32(len(expectedHash)),
	)

	return subtle.ConstantTimeCompare(
		actualHash,
		expectedHash,
	) == 1
}

var dummyHash string

func init() {
	hash, err := Hash(
		"WrapLink-Dummy-Password-Do-Not-Use",
	)

	if err != nil {
		panic(err)
	}

	dummyHash = hash
}

func VerifyDummy(passwordValue string) {
	_ = Verify(
		passwordValue,
		"$argon2id$v=19$m=65536,t=3,p=4$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	)
}

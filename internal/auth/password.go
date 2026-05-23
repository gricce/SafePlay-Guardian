package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters. These are the OWASP-recommended defaults as of 2024
// for interactive logins: 64 MiB memory, time=1, threads=4. Strong enough to
// resist offline cracking, fast enough on a parent's laptop.
const (
	argonTime    uint32 = 1
	argonMemory  uint32 = 64 * 1024
	argonThreads uint8  = 4
	argonKeyLen  uint32 = 32
	saltLen             = 16
)

var errMalformedHash = errors.New("malformed password hash")

// HashPassword returns an encoded argon2id hash suitable for direct storage.
// Format: $argon2id$v=19$m=<memory>,t=<time>,p=<threads>$<salt>$<key> (PHC).
func HashPassword(plain string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(plain), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword runs the same argon2id KDF with the salt and parameters
// embedded in `encoded`, then constant-time compares to the stored key.
func VerifyPassword(plain, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var memory, t uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &t, &threads); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(plain), salt, t, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

var _ = errMalformedHash // reserved for richer error surface later

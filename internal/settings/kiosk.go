package settings

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const pinIterations = 210_000

// ValidatePIN accepts 4–8 digits.
func ValidatePIN(pin string) error {
	if len(pin) < 4 || len(pin) > 8 {
		return errors.New("the PIN must be 4 to 8 digits")
	}
	for _, c := range pin {
		if c < '0' || c > '9' {
			return errors.New("the PIN must contain digits only")
		}
	}
	return nil
}

// HashPIN returns a salted PBKDF2-SHA256 record.
func HashPIN(pin string) (string, error) {
	if err := ValidatePIN(pin); err != nil {
		return "", err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	h, err := pbkdf2.Key(sha256.New, pin, salt, pinIterations, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2$%d$%s$%s", pinIterations, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(h)), nil
}

// CheckPIN verifies pin against a record from HashPIN in constant time.
func CheckPIN(record, pin string) bool {
	parts := strings.Split(record, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2" {
		return false
	}
	iters, err := strconv.Atoi(parts[1])
	if err != nil || iters < 1000 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[2])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pin, salt, iters, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

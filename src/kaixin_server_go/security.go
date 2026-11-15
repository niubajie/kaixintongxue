package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

// 与 Python 版保持一致：PBKDF2-SHA256 / 120000 次 / base64 存储，旧库密码可直接沿用。
const pbkdf2Iterations = 120000

func hashPassword(password string) (hash string, salt string) {
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	digest := pbkdf2.Key([]byte(password), raw, pbkdf2Iterations, 32, sha256.New)
	return base64.StdEncoding.EncodeToString(digest), base64.StdEncoding.EncodeToString(raw)
}

func verifyPassword(password, hash, salt string) bool {
	if hash == "" || salt == "" {
		return false
	}
	expected, err := base64.StdEncoding.DecodeString(hash)
	if err != nil {
		return false
	}
	rawSalt, err := base64.StdEncoding.DecodeString(salt)
	if err != nil {
		return false
	}
	actual := pbkdf2.Key([]byte(password), rawSalt, pbkdf2Iterations, len(expected), sha256.New)
	return hmac.Equal(expected, actual)
}

func newToken() string {
	raw := make([]byte, 24)
	_, _ = rand.Read(raw)
	return hex.EncodeToString(raw)
}

func validPhone(phone string) bool {
	if len(phone) != 11 || !strings.HasPrefix(phone, "1") {
		return false
	}
	for _, char := range phone {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func parseInt64(value string) (int64, bool) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

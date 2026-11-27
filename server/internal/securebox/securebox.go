// Package securebox 提供字段级加密与去重 HMAC 的共用原语。
//
// 说明：S1/S2 使用环境变量主密钥（DEK 直用）；S7 接入 KMS 后
// 只替换 key 的来源，AAD 与存储格式保持不变。
package securebox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
)

var ErrKeySize = errors.New("securebox: key must be 32 bytes")

// EncryptGCM 返回 (密文, nonce)。aad 绑行，防密文跨行/跨表搬运。
func EncryptGCM(key, plaintext []byte, aad string) (cipherText, nonce []byte, err error) {
	if len(key) != 32 {
		return nil, nil, ErrKeySize
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	return gcm.Seal(nil, nonce, plaintext, []byte(aad)), nonce, nil
}

// HMACHex 对多个片段做 HMAC-SHA256，返回 hex。用于去重与等值查询。
func HMACHex(key []byte, parts ...string) string {
	m := hmac.New(sha256.New, key)
	for _, p := range parts {
		m.Write([]byte(p))
		m.Write([]byte{0x1f}) // 分隔符，避免拼接歧义
	}
	return hex.EncodeToString(m.Sum(nil))
}

// SHA256Hex 普通摘要（非密钥场景）。
func SHA256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

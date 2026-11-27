package resource

import (
	"math/bits"
	"strings"
	"unicode"

	"gocircle/server/internal/securebox"
)

// normalize 归一化：小写、去标点空白、保留文字与数字。
func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// DedupKey 强去重：HMAC(分类|地区|规范化标题)。
func DedupKey(secret []byte, category, region, title string) string {
	return securebox.HMACHex(secret, normalize(category), normalize(region), normalize(title))
}

// simhash 计算 64 位 SimHash。分词：归一化文本的字符二元组（对中文友好）。
func simhash(text string) uint64 {
	tokens := shingles(normalize(text))
	if len(tokens) == 0 {
		return 0
	}
	var vec [64]int
	for _, t := range tokens {
		h := fnv64a(t)
		for i := 0; i < 64; i++ {
			if h>>uint(i)&1 == 1 {
				vec[i]++
			} else {
				vec[i]--
			}
		}
	}
	var out uint64
	for i := 0; i < 64; i++ {
		if vec[i] > 0 {
			out |= 1 << uint(i)
		}
	}
	return out
}

func shingles(s string) []string {
	r := []rune(s)
	if len(r) < 2 {
		if len(r) == 1 {
			return []string{string(r)}
		}
		return nil
	}
	out := make([]string, 0, len(r)-1)
	for i := 0; i+1 < len(r); i++ {
		out = append(out, string(r[i:i+2]))
	}
	return out
}

func fnv64a(s string) uint64 {
	const (
		offset = 14695981039346656037
		prime  = 1099511628211
	)
	h := uint64(offset)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime
	}
	return h
}

// Hamming 汉明距离。阈值 3 约等于相似度 > 0.92。
func Hamming(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}

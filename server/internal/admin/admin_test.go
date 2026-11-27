package admin

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("S3cret!pass")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if VerifyPassword(hash, "wrong") {
		t.Fatal("错误密码不应通过")
	}
	if !VerifyPassword(hash, "S3cret!pass") {
		t.Fatal("正确密码应通过")
	}
	if VerifyPassword("not-a-hash", "x") {
		t.Fatal("非法哈希不应通过")
	}
}

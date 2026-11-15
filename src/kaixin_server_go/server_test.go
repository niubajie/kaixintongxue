package main

import "testing"

func TestBumpVersion(t *testing.T) {
	cases := map[string]string{
		"1.2.3": "1.2.4",
		"0.1.0": "0.1.1",
		"2.9.9": "2.9.10",
		"bad":   "0.1.0",
		"":      "0.1.0",
	}
	for input, expected := range cases {
		if actual := bumpVersion(input); actual != expected {
			t.Fatalf("bumpVersion(%q) = %q, 期望 %q", input, actual, expected)
		}
	}
}

func TestPasswordRoundTrip(t *testing.T) {
	hash, salt := hashPassword("kaixin123")
	if !verifyPassword("kaixin123", hash, salt) {
		t.Fatal("正确密码校验失败")
	}
	if verifyPassword("wrong-password", hash, salt) {
		t.Fatal("错误密码不应通过校验")
	}
	if verifyPassword("kaixin123", "", "") {
		t.Fatal("空哈希不应通过校验")
	}
}

func TestMaskPhone(t *testing.T) {
	if actual := maskPhone("13800001111"); actual != "138****1111" {
		t.Fatalf("maskPhone 结果 = %q", actual)
	}
	if actual := maskPhone(""); actual != "" {
		t.Fatalf("空手机号应原样返回，实际 %q", actual)
	}
}

func TestVersionPattern(t *testing.T) {
	if !versionPattern.MatchString("1.0.0") {
		t.Fatal("合法版本号被拒绝")
	}
	for _, invalid := range []string{"1.0", "v1.0.0", "1.0.0-beta"} {
		if versionPattern.MatchString(invalid) {
			t.Fatalf("非法版本号 %q 被接受", invalid)
		}
	}
}

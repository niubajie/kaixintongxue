package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestReusableArchive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pkg.zip")
	content := []byte("pretend this is an installer archive")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(content)
	goodSHA := hex.EncodeToString(sum[:])

	app := &App{}

	// 没有校验值：文件存在即可复用
	if !app.reusableArchive(path, "") {
		t.Error("存在且非空时应可复用")
	}

	// 校验值匹配：可复用
	if !app.reusableArchive(path, goodSHA) {
		t.Error("校验值匹配时应可复用")
	}

	// 校验值不匹配：必须重新下载
	if app.reusableArchive(path, "deadbeef") {
		t.Error("校验值不匹配时不可以复用")
	}

	// 文件不存在：不可复用
	if app.reusableArchive(filepath.Join(dir, "missing.zip"), "") {
		t.Error("文件不存在时不可以复用")
	}

	// 空文件：不可复用，避免把半截文件当成品
	empty := filepath.Join(dir, "empty.zip")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if app.reusableArchive(empty, "") {
		t.Error("空文件不可以复用")
	}
}

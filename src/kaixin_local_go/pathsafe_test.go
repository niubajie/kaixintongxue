package main

import (
	"path/filepath"
	"testing"
)

func TestValidateRelativePathRejectsDangerous(t *testing.T) {
	cases := []string{
		"../etc/passwd",
		"a/../../b",
		"CON",
		"sub/NUL.txt",
		"aux",
		"COM1",
		"LPT9.log",
		"bad:name.txt",
		"star*.txt",
		"question?.txt",
		"trailing.",
		"trailing ",
		"//server/share",
	}
	for _, input := range cases {
		if err := validateRelativePath(input); err == nil {
			t.Errorf("应当拒绝但没有拒绝：%q", input)
		}
	}
}

func TestValidateRelativePathAllowsNormal(t *testing.T) {
	cases := []string{
		"工作资料/需求说明.md",
		"a/b/c.txt",
		"图片素材/封面.png",
		"根目录说明.txt",
		"dir.with.dots/file.txt",
	}
	for _, input := range cases {
		if err := validateRelativePath(input); err != nil {
			t.Errorf("应当允许但被拒绝：%q → %v", input, err)
		}
	}
}

func TestContainedIn(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "root", "library")

	inside := []string{
		filepath.Join(root, "a.txt"),
		filepath.Join(root, "sub", "b.txt"),
		root,
	}
	for _, target := range inside {
		ok, err := containedIn(root, target)
		if err != nil {
			t.Fatalf("containedIn 出错：%v", err)
		}
		if !ok {
			t.Errorf("应当算在目录内：%s", target)
		}
	}

	outside := []string{
		filepath.Join(string(filepath.Separator), "root", "other.txt"),
		filepath.Join(root, "..", "escape.txt"),
	}
	for _, target := range outside {
		ok, err := containedIn(root, target)
		if err != nil {
			t.Fatalf("containedIn 出错：%v", err)
		}
		if ok {
			t.Errorf("应当算在目录外：%s", target)
		}
	}
}

func TestSafeJoin(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "root")

	if _, err := safeJoin(root, "a/b.txt"); err != nil {
		t.Errorf("正常路径被拒绝：%v", err)
	}
	for _, bad := range []string{"../x", "CON", "a/../../b"} {
		if _, err := safeJoin(root, bad); err == nil {
			t.Errorf("危险路径没有被拒绝：%q", bad)
		}
	}
}

package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// 路径安全：借鉴 Rakazo desktop-sandbox-paths 的做法。
// 只做「目标是否落在允许的根目录内」还不够，Windows 上还有设备名、
// 保留字符、结尾点与空格这些能把路径玩弄出目录的手法，一并挡掉。

var windowsReservedName = regexp.MustCompile(`(?i)^(CON|PRN|AUX|NUL|COM[1-9¹²³]|LPT[1-9¹²³])$`)

// validateRelativePath 校验相对路径的每一段是否可以在 Windows 上安全使用。
func validateRelativePath(rel string) error {
	portable := strings.ReplaceAll(rel, "\\", "/")
	if strings.HasPrefix(portable, "//") {
		return fmt.Errorf("路径不能是 UNC 形式")
	}

	for _, segment := range strings.Split(strings.TrimLeft(portable, "/"), "/") {
		if segment == "" || segment == "." {
			continue
		}
		if segment == ".." {
			return fmt.Errorf("路径不能包含 ..")
		}
		base := segment
		if dot := strings.IndexByte(segment, '.'); dot >= 0 {
			base = segment[:dot]
		}
		if windowsReservedName.MatchString(base) {
			return fmt.Errorf("路径包含 Windows 设备名：%s", segment)
		}
		if strings.HasSuffix(segment, " ") || strings.HasSuffix(segment, ".") {
			return fmt.Errorf("路径段不能以空格或点结尾：%s", segment)
		}
		for _, r := range segment {
			if r <= 31 || strings.ContainsRune(`<>:"|?*`, r) {
				return fmt.Errorf("路径包含非法字符：%s", segment)
			}
		}
	}
	return nil
}

// containedIn 判断 target 是否落在 root 之内（含等于根目录）。
// 只做词法判断，不解析符号链接——调用方需要真实路径时自行 EvalSymlinks。
func containedIn(root, target string) (bool, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return false, err
	}
	if rel == "." {
		return true, nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return false, nil
	}
	return true, nil
}

// safeJoin 把用户给的相对路径拼到根目录下，并做完整校验。
func safeJoin(root, rel string) (string, error) {
	if err := validateRelativePath(rel); err != nil {
		return "", err
	}
	joined := filepath.Join(root, filepath.FromSlash(strings.ReplaceAll(rel, "\\", "/")))
	ok, err := containedIn(root, joined)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("路径越出允许的目录")
	}
	return joined, nil
}

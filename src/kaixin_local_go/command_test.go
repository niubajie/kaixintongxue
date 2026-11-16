package main

import "testing"

func TestSplitCommandPlain(t *testing.T) {
	fields, err := splitCommand("ollama.exe pull qwen2.5:7b")
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if len(fields) != 3 {
		t.Fatalf("期望 3 段，实际 %d：%v", len(fields), fields)
	}
	if fields[0] != "ollama.exe" || fields[2] != "qwen2.5:7b" {
		t.Fatalf("切分结果异常：%v", fields)
	}
}

func TestSplitCommandQuotedPath(t *testing.T) {
	fields, err := splitCommand(`"C:\Program Files\App\app.exe" --version`)
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if len(fields) != 2 {
		t.Fatalf("期望 2 段，实际 %d：%v", len(fields), fields)
	}
	if fields[0] != `C:\Program Files\App\app.exe` {
		t.Fatalf("带空格路径应保持完整，实际 %q", fields[0])
	}
	if fields[1] != "--version" {
		t.Fatalf("第二个参数错误：%q", fields[1])
	}
}

func TestSplitCommandCollapsesWhitespace(t *testing.T) {
	fields, err := splitCommand("a   b\t c")
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if len(fields) != 3 {
		t.Fatalf("连续空白应折叠，实际 %v", fields)
	}
}

func TestSplitCommandRejectsUnclosedQuote(t *testing.T) {
	if _, err := splitCommand(`"C:\Program Files\App`); err == nil {
		t.Fatal("引号未闭合时应报错")
	}
}

func TestSplitCommandRejectsEmpty(t *testing.T) {
	if _, err := splitCommand("   "); err == nil {
		t.Fatal("空命令应报错")
	}
}

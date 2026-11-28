package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRunFlags(t *testing.T) {
	f, err := parseRunFlags([]string{"main.gx", "-root", ".", "-prefer", "go", "arg1", "arg2"})
	if err != nil {
		t.Fatal(err)
	}
	if f.file != "main.gx" || f.root != "." || f.prefer != "go" ||
		len(f.progArgs) != 2 || f.progArgs[0] != "arg1" || f.progArgs[1] != "arg2" {
		t.Errorf("解析结果异常: %+v", f)
	}
	f, err = parseRunFlags([]string{"main.gx", "--", "-x", "y"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.progArgs) != 2 || f.progArgs[0] != "-x" || f.progArgs[1] != "y" {
		t.Errorf("-- 之后的参数解析异常: %+v", f.progArgs)
	}
	if _, err := parseRunFlags(nil); err == nil {
		t.Error("缺少 file.gx 应报错")
	}
}

func TestFindGoModDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module demo\n\ngo 1.27\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got := findGoModDir(filepath.Join(sub, "main.gx"))
	if got != dir {
		t.Errorf("findGoModDir = %q, want %q", got, dir)
	}
}

// TestCmdRunEndToEnd 端到端验证：.gx → 临时 .go → go run，且不污染源码目录。
func TestCmdRunEndToEnd(t *testing.T) {
	dir := t.TempDir()
	gx := filepath.Join(dir, "main.gx")
	src := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"gox-run-ok\")\n}\n"
	if err := os.WriteFile(gx, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/run\ngo 1.27\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if err := cmdRunWithIO([]string{gx}, &out, &errOut); err != nil {
		t.Fatalf("cmdRun 失败: %v\nstdout=%s\nstderr=%s", err, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "gox-run-ok") {
		t.Errorf("输出中未找到预期文本: stdout=%q stderr=%q", out.String(), errOut.String())
	}
	// 单文件模式的产物必须落在临时目录，源码目录里不能有生成的 main.go。
	if _, err := os.Stat(filepath.Join(dir, "main.go")); !os.IsNotExist(err) {
		t.Error("源码目录出现了转译产物 main.go")
	}
}

func TestCmdRunPassesArgs(t *testing.T) {
	dir := t.TempDir()
	gx := filepath.Join(dir, "main.gx")
	src := "package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n)\n\nfunc main() {\n\tfmt.Println(os.Args[1])\n}\n"
	if err := os.WriteFile(gx, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/runargs\ngo 1.27\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if err := cmdRunWithIO([]string{gx, "hello-arg"}, &out, &errOut); err != nil {
		t.Fatalf("cmdRun 失败: %v\nstderr=%s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "hello-arg") {
		t.Errorf("程序参数未传递: stdout=%q stderr=%q", out.String(), errOut.String())
	}
}

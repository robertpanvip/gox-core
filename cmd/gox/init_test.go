package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModuleNameForDir(t *testing.T) {
	cases := []struct{ dir, want string }{
		{".", "goxapp"},
		{"myapp", "myapp"},
		{"/tmp/my app", "my-app"},
		{"/tmp/..", "goxapp"},
	}
	for _, c := range cases {
		if got := moduleNameForDir(c.dir); got != c.want {
			t.Errorf("moduleNameForDir(%q) = %q, want %q", c.dir, got, c.want)
		}
	}
}

func TestGoModContent(t *testing.T) {
	got := goModContent("example.com/demo", "v1.2.3", "")
	for _, want := range []string{
		"module example.com/demo",
		"go 1.27",
		"require github.com/robertpanvip/gox-core v1.2.3",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("goModContent 缺少 %q:\n%s", want, got)
		}
	}
	withReplace := goModContent("demo", "v1.2.3", "../gox-core")
	if !strings.Contains(withReplace, "replace github.com/robertpanvip/gox-core => ../gox-core") {
		t.Errorf("replace 指令缺失:\n%s", withReplace)
	}
}

func TestCmdInitGeneratesSkeleton(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "myapp")
	if err := cmdInit([]string{dir, "-module", "example.com/myapp", "-runtime", "v9.9.9"}); err != nil {
		t.Fatal(err)
	}
	mod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"module example.com/myapp", "require github.com/robertpanvip/gox-core v9.9.9"} {
		if !strings.Contains(string(mod), want) {
			t.Errorf("go.mod 缺少 %q:\n%s", want, mod)
		}
	}
	gx, err := os.ReadFile(filepath.Join(dir, "main.gx"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gx), "package main") || !strings.Contains(string(gx), "func main()") {
		t.Errorf("main.gx 内容异常:\n%s", gx)
	}
}

func TestCmdInitRefusesOverwriteUnlessForce(t *testing.T) {
	dir := t.TempDir()
	modPath := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(modPath, []byte("module keepme\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := cmdInit([]string{dir}); err == nil {
		t.Fatal("已有 go.mod 时应拒绝覆盖")
	}
	b, _ := os.ReadFile(modPath)
	if string(b) != "module keepme\n" {
		t.Fatalf("已有文件被意外修改: %q", b)
	}
	if err := cmdInit([]string{dir, "-force", "-module", "example.com/new"}); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(modPath)
	if !strings.Contains(string(b), "module example.com/new") {
		t.Fatalf("-force 未覆盖 go.mod: %q", b)
	}
}

func TestParseInitFlags(t *testing.T) {
	f, err := parseInitFlags([]string{"-module", "x/y", "-runtime", "v0.2.0", "-replace", "../core", "-force", "app"})
	if err != nil {
		t.Fatal(err)
	}
	if f.dir != "app" || f.module != "x/y" || f.runtime != "v0.2.0" || f.replace != "../core" || !f.force {
		t.Errorf("解析结果异常: %+v", f)
	}
	if _, err := parseInitFlags([]string{"a", "b"}); err == nil {
		t.Error("多个目录应报错")
	}
}

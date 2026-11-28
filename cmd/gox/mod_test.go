package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdModInit(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "demo")
	if err := cmdModInit([]string{dir, "-module", "example.com/demo", "-version", "v1.0.0"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "require github.com/robertpanvip/gox-core v1.0.0") {
		t.Errorf("go.mod 内容异常:\n%s", b)
	}
}

func TestCmdModUpgrade(t *testing.T) {
	dir := t.TempDir()
	modPath := filepath.Join(dir, "go.mod")
	initial := "module example.com/demo\n\ngo 1.27\n\nrequire github.com/robertpanvip/gox-core v0.1.0\n"
	if err := os.WriteFile(modPath, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}
	if err := cmdModUpgrade([]string{"v2.3.4", "-dir", dir, "-no-tidy"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(modPath)
	if !strings.Contains(string(b), "require github.com/robertpanvip/gox-core v2.3.4") {
		t.Errorf("版本未更新:\n%s", b)
	}
	if strings.Contains(string(b), "v0.1.0") {
		t.Errorf("旧版本残留:\n%s", b)
	}
}

func TestCmdModUpgradeAppendsWhenMissing(t *testing.T) {
	dir := t.TempDir()
	modPath := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(modPath, []byte("module example.com/demo\n\ngo 1.27\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := cmdModUpgrade([]string{"v3.0.0", "-dir", dir, "-no-tidy"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(modPath)
	if !strings.Contains(string(b), "require github.com/robertpanvip/gox-core v3.0.0") {
		t.Errorf("缺少 require 行未追加:\n%s", b)
	}
}

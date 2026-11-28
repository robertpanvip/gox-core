package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// cmdMod 只管 GoX 自有运行时的版本，第三方依赖请直接用 go get。
func cmdMod(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("mod: 需要子命令 init 或 upgrade")
	}
	switch args[0] {
	case "init":
		return cmdModInit(args[1:])
	case "upgrade":
		return cmdModUpgrade(args[1:])
	default:
		return fmt.Errorf("mod: 未知子命令 %q（可用: init, upgrade）", args[0])
	}
}

type modFlags struct {
	dir     string
	module  string
	version string
	force   bool
	noTidy  bool
}

// positional 取 "dir"（mod init）或 "version"（mod upgrade），决定裸参数含义。
func parseModFlags(args []string, positional string) (*modFlags, error) {
	f := &modFlags{dir: "."}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-module":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("-module 缺少参数")
			}
			i++
			f.module = args[i]
		case "-dir":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("-dir 缺少参数")
			}
			i++
			f.dir = args[i]
		case "-version":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("-version 缺少参数")
			}
			i++
			f.version = args[i]
		case "-force":
			f.force = true
		case "-no-tidy":
			f.noTidy = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return nil, fmt.Errorf("mod: 未知参数 %q", args[i])
			}
			switch positional {
			case "dir":
				if f.dir != "." {
					return nil, fmt.Errorf("mod init: 只能指定一个目录")
				}
				f.dir = args[i]
			case "version":
				if f.version != "" {
					return nil, fmt.Errorf("mod upgrade: 只能指定一个版本")
				}
				f.version = args[i]
			}
		}
	}
	return f, nil
}

// cmdModInit 只在当前目录写 go.mod（不生成 main.gx），用于已有 .gx 源码的项目。
func cmdModInit(args []string) error {
	f, err := parseModFlags(args, "dir")
	if err != nil {
		return err
	}
	if f.version == "" {
		f.version = defaultRuntimeVersion
	}
	if err := os.MkdirAll(f.dir, 0755); err != nil {
		return err
	}
	modPath := filepath.Join(f.dir, "go.mod")
	if !f.force {
		if _, err := os.Stat(modPath); err == nil {
			return fmt.Errorf("mod init: %s 已存在（使用 -force 覆盖）", modPath)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	module := f.module
	if module == "" {
		module = moduleNameForDir(f.dir)
	}
	content := goModContent(module, f.version, "")
	if err := os.WriteFile(modPath, []byte(content), 0644); err != nil {
		return err
	}
	fmt.Printf("已生成 %s（runtime=%s）\n", modPath, f.version)
	return nil
}

// 用 [ \t] 而非 [[:space:]]，避免匹配跨行空白而吞掉 go.mod 里的空行。
var requireRuntimeRE = regexp.MustCompile(`(?m)^[ \t]*require[ \t]+` + regexp.QuoteMeta(runtimeModule) + `[ \t]+[^ \t\r\n]+`)

// cmdModUpgrade 更新 go.mod 中 gox-core 的 require 版本，再执行 go mod tidy。
func cmdModUpgrade(args []string) error {
	f, err := parseModFlags(args, "version")
	if err != nil {
		return err
	}
	if f.version == "" {
		f.version = defaultRuntimeVersion
	}
	modPath := filepath.Join(f.dir, "go.mod")
	b, err := os.ReadFile(modPath)
	if err != nil {
		return err
	}
	content := string(b)
	want := "require " + runtimeModule + " " + f.version
	if requireRuntimeRE.MatchString(content) {
		content = requireRuntimeRE.ReplaceAllString(content, want)
	} else {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += "\n" + want + "\n"
	}
	if err := os.WriteFile(modPath, []byte(content), 0644); err != nil {
		return err
	}
	fmt.Printf("已将 %s 的运行时版本更新为 %s\n", modPath, f.version)
	if f.noTidy {
		return nil
	}
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = f.dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

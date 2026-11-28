package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// runtimeModule 是 GoX 自有运行时的 module 路径。它与 gox-core 的 module 路径一致：
// 运行时（runtime/*）本身就在这个 module 里，走 Go 原生模块分发。
const runtimeModule = "github.com/robertpanvip/gox-core"

// defaultRuntimeVersion 是 gox init 写入 go.mod 的默认运行时版本。
// 可通过 -runtime 覆盖；发布时由 release 流程 bump 此常量。
const defaultRuntimeVersion = "v0.1.0"

type initFlags struct {
	dir     string
	module  string
	runtime string
	replace string
	force   bool
}

func parseInitFlags(args []string) (*initFlags, error) {
	f := &initFlags{runtime: defaultRuntimeVersion}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-module":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("-module 缺少参数")
			}
			i++
			f.module = args[i]
		case "-runtime":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("-runtime 缺少参数")
			}
			i++
			f.runtime = args[i]
		case "-replace":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("-replace 缺少参数")
			}
			i++
			f.replace = args[i]
		case "-force":
			f.force = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return nil, fmt.Errorf("init: 未知参数 %q", args[i])
			}
			if f.dir != "" {
				return nil, fmt.Errorf("init: 只能指定一个目录")
			}
			f.dir = args[i]
		}
	}
	if f.dir == "" {
		f.dir = "."
	}
	if f.module == "" {
		f.module = moduleNameForDir(f.dir)
	}
	if f.module == "" {
		return nil, fmt.Errorf("init: 无法从目录名推导 module 路径，请用 -module 指定")
	}
	return f, nil
}

var invalidModuleChars = regexp.MustCompile(`[^A-Za-z0-9_./-]`)

// moduleNameForDir 从目录名推导一个可用的 module 路径。
// 目录名含空格等非法字符时清理；当前目录（.）回退为 goxapp。
func moduleNameForDir(dir string) string {
	base := filepath.Base(filepath.Clean(dir))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "goxapp"
	}
	name := invalidModuleChars.ReplaceAllString(base, "-")
	name = strings.Trim(name, "-/")
	if name == "" {
		return "goxapp"
	}
	return name
}

func goModContent(module, version, replace string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "module %s\n\ngo 1.27\n\nrequire %s %s\n", module, runtimeModule, version)
	if replace != "" {
		fmt.Fprintf(&b, "\nreplace %s => %s\n", runtimeModule, replace)
	}
	return b.String()
}

const initMainGX = `package main

import "fmt"

// 这是 gox init 生成的最小示例。
// 需要 UI / 网络等能力时，import gox-core/runtime 下的包即可，
// 依赖已由 go.mod 中的 require 声明，不需要额外安装步骤。
func main() {
	fmt.Println("hello, gox!")
}
`

// cmdInit 生成 GX 项目骨架：go.mod + main.gx。
// 已存在同名文件时默认拒绝覆盖（-force 可强制），避免毁掉开发者现有依赖。
func cmdInit(args []string) error {
	f, err := parseInitFlags(args)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(f.dir, 0755); err != nil {
		return err
	}
	goModPath := filepath.Join(f.dir, "go.mod")
	mainGXPath := filepath.Join(f.dir, "main.gx")
	if !f.force {
		for _, p := range []string{goModPath, mainGXPath} {
			if _, err := os.Stat(p); err == nil {
				return fmt.Errorf("init: %s 已存在（使用 -force 覆盖）", p)
			} else if !os.IsNotExist(err) {
				return err
			}
		}
	}
	if err := os.WriteFile(goModPath, []byte(goModContent(f.module, f.runtime, f.replace)), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(mainGXPath, []byte(initMainGX), 0644); err != nil {
		return err
	}
	fmt.Printf("已初始化项目: %s\n", f.dir)
	fmt.Printf("  go.mod  module=%s runtime=%s@%s\n", f.module, runtimeModule, f.runtime)
	if f.replace != "" {
		fmt.Printf("  replace %s => %s\n", runtimeModule, f.replace)
	}
	fmt.Printf("  main.gx\n")
	fmt.Printf("下一步: cd %s && gox run main.gx\n", f.dir)
	return nil
}

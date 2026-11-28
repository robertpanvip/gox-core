package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/robertpanvip/gox-core/pkg/compiler"
	"github.com/robertpanvip/gox-core/pkg/module"
)

type runFlags struct {
	file       string
	root       string
	modulePath string
	prefer     string
	progArgs   []string
}

func parseRunFlags(args []string) (*runFlags, error) {
	f := &runFlags{modulePath: runtimeModule, prefer: "gx"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-root":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("-root 缺少参数")
			}
			i++
			f.root = args[i]
		case "-module":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("-module 缺少参数")
			}
			i++
			f.modulePath = args[i]
		case "-prefer":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("-prefer 缺少参数")
			}
			i++
			f.prefer = args[i]
		case "--":
			f.progArgs = append(f.progArgs, args[i+1:]...)
			return f, nil
		default:
			if f.file == "" {
				f.file = args[i]
			} else {
				f.progArgs = append(f.progArgs, args[i])
			}
		}
	}
	if f.file == "" {
		return nil, fmt.Errorf("run: 需要 <file.gx>")
	}
	if f.prefer != "gx" && f.prefer != "go" {
		return nil, fmt.Errorf("-prefer 只支持 gx 或 go，得到 %q", f.prefer)
	}
	return f, nil
}

// findGoModDir 从入口文件所在目录向上查找 go.mod，返回其所在目录。
// 找不到时返回入口文件所在目录——此时 go run 会以 GOMODCACHE/GOPATH 模式运行。
func findGoModDir(file string) string {
	dir := filepath.Dir(file)
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filepath.Dir(file)
}

// cmdRun 转译 .gx 后直接调用 go run，一步到位。
// 转译失败不会启动 go run；产物放临时目录，不污染源码树。
func cmdRun(args []string) error {
	return cmdRunWithIO(args, os.Stdout, os.Stderr)
}

func cmdRunWithIO(args []string, stdout, stderr io.Writer) error {
	f, err := parseRunFlags(args)
	if err != nil {
		return err
	}
	if f.root != "" {
		return runProject(f, stdout, stderr)
	}
	return runSingle(f, stdout, stderr)
}

func runSingle(f *runFlags, stdout, stderr io.Writer) error {
	src, err := readFile(f.file)
	if err != nil {
		return err
	}
	res, err := compiler.CompileWithModule(f.file, src, f.modulePath)
	if err != nil {
		return err
	}
	var diags []gxFileDiagnostic
	if res.Analysis != nil {
		for _, d := range res.Analysis.Diagnostics() {
			diags = append(diags, gxFileDiagnostic{
				File: f.file, Line: d.Pos.Line, Col: d.Pos.Col,
				Code: d.Code, Message: d.Message,
			})
		}
	}
	if res.InvalidGo {
		return fmt.Errorf("生成的 Go 代码语法无效: 编译产物已被编译器闸门抑制")
	}
	if err := res.Validate(); err != nil {
		return fmt.Errorf("生成的 Go 代码语法无效: %w", err)
	}
	if len(diags) > 0 {
		printGxDiagnostics(diags)
		return fmt.Errorf("GX 语义检查未通过")
	}

	tmp, err := os.MkdirTemp("", "gox-run-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	goFile := filepath.Join(tmp, "main.go")
	if err := os.WriteFile(goFile, []byte(res.GoSource), 0644); err != nil {
		return err
	}
	return execGoRun(findGoModDir(f.file), goFile, f.progArgs, stdout, stderr)
}

func runProject(f *runFlags, stdout, stderr io.Writer) error {
	projModule := readProjectModule(f.root)
	if projModule == "" {
		projModule = f.modulePath
	}
	p := module.PreferGX
	if f.prefer == "go" {
		p = module.PreferGo
	}
	proj, err := module.LoadProject(f.root, projModule, p)
	if err != nil {
		return err
	}
	proj.RuntimeModule = f.modulePath
	files, err := proj.Build(f.file)
	if err != nil {
		return err
	}

	absEntry, _ := filepath.Abs(f.file)
	var entryGo string
	var diags []gxFileDiagnostic
	for _, cf := range files {
		if err := os.WriteFile(cf.GoPath, []byte(cf.GoSource), 0644); err != nil {
			return err
		}
		absSrc, _ := filepath.Abs(cf.SourcePath)
		if absSrc == absEntry || filepath.Clean(cf.SourcePath) == filepath.Clean(f.file) {
			entryGo = cf.GoPath
		}
		if cf.Analysis != nil {
			for _, d := range cf.Analysis.Diagnostics() {
				diags = append(diags, gxFileDiagnostic{
					File: filepath.ToSlash(cf.SourcePath), Line: d.Pos.Line, Col: d.Pos.Col,
					Code: d.Code, Message: d.Message,
				})
			}
		}
	}
	if len(diags) > 0 {
		printGxDiagnostics(diags)
		return fmt.Errorf("GX 语义检查未通过")
	}
	if entryGo == "" {
		return fmt.Errorf("run: 在项目 %s 中找不到入口 %s", f.root, f.file)
	}
	return execGoRun(f.root, entryGo, f.progArgs, stdout, stderr)
}

func execGoRun(workDir, goFile string, args []string, stdout, stderr io.Writer) error {
	cmdArgs := append([]string{"run", goFile}, args...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Dir = workDir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// version 用于让安装脚本确认二进制可用；值由 release 流程通过 -ldflags 注入。
var version = "dev"

func cmdVersion(_ []string) error {
	fmt.Println(version)
	return nil
}

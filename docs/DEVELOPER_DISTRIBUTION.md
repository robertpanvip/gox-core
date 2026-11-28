# GoX 开发者分发设计

> 状态：已定稿，按此实施。最后更新：2026-11-28
>
> 配套实现：`cmd/gox`（`init` / `run` / `mod`）、`scripts/install.sh`、`.github/workflows/release.yml`

## 1. 结论先行

**GoX 是 Go 的转译层，不是独立语言。** `.gx` → 标准 `.go` → Go 官方工具链编译；运行时 `runtime/{scene,app,signal,list,fetch,...}` 本身就是一个普通的 Go module（`github.com/robertpanvip/gox-core`）。这个事实决定了三件事：

| 决策 | 结论 | 理由 |
|---|---|---|
| 要不要自建完整开发环境 | **不要** | 等于复制 Go 的编译器、链接器、包管理、标准库、交叉编译，成本极高，而卖点只是语法糖层 |
| 要不要退化成裸 CLI | **不要** | 工具链已存在（`gox` / `gox-lsp` / `libgox.so` / Web Playground），缺的是把它们串成一条龙 |
| 正确路线 | **依附 Go 作为后端，把 gox 做成一体化工具链** | 类比 TypeScript 之于 Node：不造运行时，在 Go 之上提供更好的语言与开发体验 |

开发者应该感受到的是：**装一个 gox、写 `.gx`、`gox run` 就跑**。底层全是 Go，但我们不重复造 Go 的轮子。

## 2. 现状盘点（代码事实）

### 2.1 已有的工具链资产

| 资产 | 位置 | 作用 | 状态 |
|---|---|---|---|
| `gox` | `cmd/gox` | `build` / `check` / `fmt` / `sourcemap` / `map-lookup` / `lsp-map` / `lsp` / `test`，`build` 带真实 `go build` 错误回投（sourcemap） | 已有 |
| `gox-lsp` | `cmd/gox-lsp` | LSP server | 已有 |
| `libgox.so` | `bindings/cabi` | C-ABI，供 JetBrains 插件经 JNA 调用 | 已有 |
| 运行时库 | `runtime/*` | scene/app/signal/list/fetch/canvas/loop 等 | 已有 |
| Web Playground | — | 浏览器内体验 | 已有 |

### 2.2 缺口

1. **没有一个命令把"转译 + 运行"一步做完**：现在要 `gox build -o main.go` 再 `go run main.go`。
2. **没有项目脚手架**：开发者不知道新项目要怎么写 `go.mod`、怎么引入运行时。
3. **没有安装入口**：只能 `go install`，没有预编译二进制和一键脚本。

## 3. 核心原则

1. **依附 Go，不封装 Go**：产物是标准 Go，`go.mod` 是依赖的唯一真源。绝不再造一套包管理器。
2. **依赖分两类，处理方式不同**（见第 4 节）。
3. **自有运行时随 Go module 分发**，走 Go 原生缓存与校验，不走自建下载器。
4. **IDE 体验复用现有标准接口**（LSP / C-ABI），不碰编辑器本身。
5. **新增命令不动现有用例**：`cmd/gox` 无历史测试，新增命令配独立测试文件。

## 4. 依赖分两类

这是整个设计最容易搞混的地方，必须先分清：

| 类别 | 例子 | 谁负责 | 怎么给开发者 |
|---|---|---|---|
| **GoX 自有运行时** | `runtime/{scene,app,signal,list,fetch}` | **我们自带** | `gox init` 自动写好 `go.mod` 的 `require github.com/robertpanvip/gox-core vX.Y.Z` |
| **第三方 Go 包** | `fmt`、`net/http`、yaml、其它开源库 | **交给 Go** | 开发者照常 `go get` / `go mod tidy`，gox 不碰 |

> **为什么不自己做包管理器？** 因为产物是标准 Go，`go.mod` 天然就是依赖的唯一真源。再造一个只会和 `go mod` 打架——版本冲突、缓存不一致、离线行为分叉，全是坑。

## 5. 分发物清单

一次 Release 应产出：

| 文件 | 平台 | 说明 |
|---|---|---|
| `gox_<ver>_linux_amd64.tar.gz` | Linux x86_64 | 含 `gox` 二进制 |
| `gox_<ver>_linux_arm64.tar.gz` | Linux arm64 | 含 `gox` 二进制 |
| `gox_<ver>_darwin_amd64.tar.gz` | macOS Intel | 含 `gox` 二进制 |
| `gox_<ver>_darwin_arm64.tar.gz` | macOS Apple Silicon | 含 `gox` 二进制 |
| `gox_<ver>_windows_amd64.zip` | Windows x86_64 | 含 `gox.exe` |
| `checksums.txt` | 全部 | SHA256 校验和 |

前置要求（写进 README 与安装脚本提示）：**Go 1.27+**。Go 是 prerequisite，不是我们要打包的东西——因为产物最终要交给 Go 工具链编译，绕不开。

## 6. 命令设计

### 6.1 `gox init [目录]`

生成项目骨架：

```
<dir>/
├── go.mod          module <name>；go 1.27；require gox-core <runtime-version>
├── main.gx         package main / func main() 最小可运行示例
└── .gitignore      忽略转译产物 *.go（可选）
```

参数：

| 参数 | 默认 | 说明 |
|---|---|---|
| `-module <path>` | 目录名（或 `example.com/<dir>`） | 项目 module 路径 |
| `-runtime <version>` | 内置常量 `defaultRuntimeVersion` | 自有运行时版本；写进 `require` |
| `-replace <dir>` | 空 | 本地开发时写 `replace github.com/robertpanvip/gox-core => <dir>` |

幂等性：目标目录已有 `go.mod` 时**默认拒绝覆盖**（除非 `-force`），避免毁掉开发者已有依赖。

### 6.2 `gox run <file.gx> [args...]`

一步到位：转译 → 生成临时 `.go` → `go run`。

- 默认单文件模式；支持 `-root <dir>` 走跨文件项目模式。
- 转译失败直接返回，不启动 `go run`。
- 产物落在临时目录（`os.MkdirTemp`），跑完清理；不污染源码目录。
- 运行时依赖由项目 `go.mod` 解析（所以 `gox run` 前应先 `gox init` 或已有等价 `go.mod`）。

> 为什么用临时目录而不是就地生成 `main.go`？避免 `.go` 与 `.gx` 同名冲突、避免忘记清理导致提交转译产物。

### 6.3 `gox mod <subcommand>`

包装"自有运行时版本"管理，第三方仍走原生 `go` 命令：

- `gox mod init` — 等价于 `gox init` 里写 `go.mod` 的部分。
- `gox mod upgrade [version]` — 更新 `go.mod` 里 gox-core 的 `require` 版本，再跑 `go mod tidy`。

不做 `gox mod add <pkg>`：第三方包请直接用 `go get`。

## 7. 自有运行时怎么"自带"

两个方案，二选一。**选方案 A。**

| 方案 | 做法 | 优点 | 缺点 |
|---|---|---|---|
| **A（推荐）** | 运行时作为独立 Go module 发布 tag；`gox init` 写入固定版本 `require github.com/robertpanvip/gox-core vX.Y.Z` | 升级只改版本号，走 Go 原生模块缓存/校验，零维护成本 | 首次需要网络下载模块 |
| B（完全自包含） | gox 二进制内嵌运行时源码，`gox init` 直接落到项目里 | 离线可用、版本绝对一致 | 升级要重发 gox；项目里多一堆生成文件；与 Go module 语义割裂 |

选择 A 的根本原因：**运行时本来就是一个 Go module，让它走 Go 原生分发是最自然的**；自建内嵌机制只会制造第二套版本管理。

## 8. 安装脚本

`scripts/install.sh`（同时提供 `install.ps1` 给 Windows）：

1. 探测 OS / ARCH（`uname -s` / `uname -m`）。
2. 从 GitHub Releases 下载对应 tarball 到临时目录。
3. 用 `checksums.txt` 校验 SHA256（下载失败/校验失败明确报错退出）。
4. 解压出 `gox` 到 `${GOX_INSTALL_DIR:-$HOME/.local/bin}`。
5. 提示把安装目录加入 `PATH`。
6. 支持 `GOX_VERSION=vX.Y.Z` 指定版本；默认取最新 Release。

不自动改 shell 配置文件——只提示，不越权。

## 9. Release 工作流

`.github/workflows/release.yml`：


- 触发：`push tags: ['v*']`。
- 矩阵交叉编译 5 个目标（`GOOS`/`GOARCH`），`CGO_ENABLED=0`（gox 本体是纯 Go）。
- 打包 + 生成 `checksums.txt`。
- 用 `softprops/action-gh-release` 创建 Release 并上传全部产物。
- 权限：`contents: write`。

版本号约定：`vMAJOR.MINOR.PATCH`。运行时版本与 gox 版本共用同一个 tag（简单、好记；未来若运行时独立演进可拆 tag 前缀）。

## 10. IDE 接入

不用自己造编辑器：

- **LSP 路线**：`gox-lsp` 已实现 → VS Code / Neovim / Sublime 装通用 LSP 客户端即可。
- **C-ABI 路线**：`libgox.so` 已提供 → JetBrains 插件继续走 JNA。

我们只需维护好这两个接入点，不碰编辑器本身。

## 11. 落地顺序与验收

| 序 | 项目 | 验收标准 |
|---|---|---|
| 1 | `gox init` / `gox run` 命令 + 测试 | `go test ./cmd/gox/...` 全绿；现有用例零改动 |
| 2 | `scripts/install.sh` / `install.ps1` | 在干净环境能下载、校验、安装；失败场景有明确报错 |
| 3 | `.github/workflows/release.yml` | 打 `v*` tag 后产出 5 个平台包 + checksums |
| 4 | README 更新 | 快速开始：`安装 → gox init → gox run` 三步跑通 |

## 12. FAQ

**Q：开发者机器没装 Go 能用吗？**
不能，也不该。产物是标准 Go，编译必须走 Go 工具链。我们把"前置要求 Go 1.27+"写清楚即可，不试图打包 Go。

**Q：gox 会不会和 `go` 命令冲突？**
不会。gox 是独立二进制，只在内部调用 `go run` / `go build`。

**Q：为什么 `gox run` 用临时目录而不是就地生成？**
避免 `.go` 与 `.gx` 同名、避免污染源码树、避免忘记清理导致转译产物被提交。

**Q：第三方依赖谁管？**
`go get` / `go mod tidy`。gox 只在 `gox mod upgrade` 时碰 go.mod 里 gox-core 那一行。

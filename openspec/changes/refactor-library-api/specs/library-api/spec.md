# 规范增量：库 API

## 新增需求

### 需求：以项目根目录打开项目

系统应提供 `codei18n.Open(root string, opts ...Option) (*Project, error)`，所有文件路径相对于 root 下的 `.codei18n` 目录，不依赖进程当前目录。

#### 场景：同一进程打开多个项目
- **当** 调用方并发地对多个 Project 执行 map update 与翻译
- **那么** 每个项目只读写自己的 `.codei18n`，结果互不串扰

### 需求：长任务可取消并报告进度

扫描、map update、翻译、convert 必须接受 `context.Context`，并通过回调发出包含阶段、done/total、当前文件与错误的进度事件。

#### 场景：翻译中途取消
- **当** 翻译进行到第 2 批时调用方取消 ctx
- **那么** 不再派发新批次，返回 `context.Canceled` 且结果 `Canceled=true`
- **并且** 第 1 批的译文已写入 mappings.json

### 需求：库代码不打印、不退出

库代码只能通过注入的 Logger 输出日志，不得调用 `os.Exit`、不得写 stdout/stderr、不得依赖 spinner 或彩色输出库。

#### 场景：静态检查
- **当** 运行 `go test ./...`
- **那么** `purity_test.go` 对 `cmd/` 与 `internal/log` 以外的源码做导入与调用检查并通过

### 需求：分层配置与来源层

配置按 内置默认 < 全局 < 项目 < 调用方覆盖 合并，并返回每个字段的值与来源层。

#### 场景：查询字段来源
- **当** 全局配置设置 localLanguage=de，调用方覆盖 sourceLanguage=fr
- **那么** localLanguage 的来源为 global，sourceLanguage 的来源为 override

### 需求：Provider 配置档

全局配置可以定义多个命名 profile；项目选用 profile 并可以覆盖单个字段。

#### 场景：项目覆盖 profile 的单个字段
- **当** 全局 profile `local` 为 ollama/qwen，项目选用 `local` 并覆盖 model=qwen2.5:7b
- **那么** 有效 provider 为 ollama（来源 global），model 为 qwen2.5:7b（来源 project）

### 需求：JSON 原子写

mappings.json、index.json、config.json、overrides.json 的写入必须先写临时文件再 rename。

#### 场景：并发读取
- **当** 写入方反复重写文件
- **那么** 读取方只会读到完整的旧内容或完整的新内容

### 需求：CLI 兼容

现有 CLI 命令的参数与输出保持兼容。

#### 场景：既有集成测试
- **当** 运行 `tests/` 下既有集成测试
- **那么** 全部通过

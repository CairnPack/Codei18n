# CodeI18n 技术文档

**Code Comment Internationalization Infrastructure**

[![CI](https://github.com/studyzy/codei18n/actions/workflows/ci.yml/badge.svg)](https://github.com/studyzy/codei18n/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/studyzy/codei18n)](https://goreportcard.com/report/github.com/studyzy/codei18n)
[![License](https://img.shields.io/github/license/studyzy/codei18n)](LICENSE)
[![Release](https://img.shields.io/github/v/release/studyzy/codei18n)](https://github.com/studyzy/codei18n/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/studyzy/codei18n)](go.mod)

---

## 1. 项目定位与问题定义

### 1.1 项目定位

**CodeI18n** 是一个面向工程团队的 **代码注释国际化基础设施**，目标是在不破坏 Git 语义、不污染源码、不影响编译与调试的前提下，实现：

* **源码仓库统一语言（推荐英文）**
* **本地开发环境以母语阅读和编写注释**
* **注释语言在 Git 提交与 IDE 展示之间自动、可逆转换**

CodeI18n 不是简单的“翻译插件”，而是一个 **围绕 AST、Git、IDE 的完整工具链**。

---

### 1.2 要解决的核心问题

| 问题              | 传统方案      | CodeI18n      |
| --------------- | --------- | ------------- |
| 注释语言冲突          | 强制英文 / 中文 | 英文入库，本地多语言    |
| diff / blame 污染 | 严重        | 不污染           |
| 多语言协作           | 成本高       | 零成本           |
| IDE 体验          | 无         | 原生集成          |
| 可扩展性            | 无         | AST + Adapter |

---

## 2. 总体设计目标

### 2.1 功能目标

* 支持 **多自然语言**

    * 默认：英文（en）、中文（zh-CN）
    * 识别候选：en、zh-Hans、zh-Hant、ja、ko、de、fr、es、ru（可通过 `detectLanguages` 配置），可翻译到任意语言
* 支持 **多编程语言**

    * Go、Rust、Java、Kotlin、TypeScript、JavaScript、C#、Dart（扩展名见 §8.2；groovy / scala 按 Java 语法解析）
* 支持 **多 IDE**

    * VS Code
    * JetBrains 家族（IntelliJ / GoLand / RustRover 等）

---

### 2.2 非功能目标（硬约束）

1. **Git 仓库中只存在一种注释语言**
2. **IDE 展示不修改源码**
3. **所有注释解析必须基于 AST / PSI**
4. **注释翻译必须可逆、可定位**
5. **Core 与 IDE 插件解耦**

---

## 3. 核心设计原则

### 3.1 单一事实源（Single Source of Truth）

* 英文注释源码 = 唯一事实源
* 本地语言注释 = 派生视图

---

### 3.2 语义优先，而非文本优先

* 注释必须绑定到：

    * 文件
    * 语义符号（函数 / 结构体 / 模块）
* 禁止基于行号、正则处理注释

---

### 3.3 Core 与 IDE 解耦

* **Core（Go 实现）**

    * 注释解析
    * ID 生成
    * 翻译与映射
* **IDE 插件**

    * 只负责渲染
    * 不包含翻译、AST 逻辑

---

## 4. 整体系统架构

### 4.1 架构总览

```text
┌────────────────────────────────────────────┐
│                   IDE Layer                │
│                                            │
│  ┌───────────────┐   ┌──────────────────┐ │
│  │ VS Code       │   │ JetBrains IDEs   │ │
│  │ Extension     │   │ Plugin           │ │
│  └──────▲────────┘   └────────▲─────────┘ │
│         │ Decorations / Inlay           │   │
└─────────┼────────────────────────────────┘
          │ JSON / CLI
┌─────────┴────────────────────────────────┐
│          CodeI18n Core (Go)               │
│  - AST Parsers                            │
│  - Comment Model                          │
│  - Comment ID Generator                   │
│  - Mapping Store                          │
│  - Translation Engine                     │
└─────────▲────────────────────────────────┘
          │ pre-commit
┌─────────┴────────────────────────────────┐
│          Git Hooks / CLI                  │
└─────────▲────────────────────────────────┘
          │
┌─────────┴────────────────────────────────┐
│          Git Remote Repository            │
│        (English-only Source Code)         │
└───────────────────────────────────────────┘
```

---

## 5. CodeI18n Core 设计

### 5.1 Core 职责

* 扫描源码并提取注释
* 为注释生成稳定 ID
* 管理多语言注释映射
* 执行注释语言转换（提交前）
* 为 IDE 提供结构化注释数据

---

### 5.2 注释统一抽象模型

```go
type Comment struct {
    ID         string      // 稳定 ID
    File       string      // 相对项目根的路径
    Language   string      // 编程语言标识：go / rust / ...
    Symbol     string      // 语义符号路径
    Range      TextRange
    SourceText string      // 原文
    Type       CommentType // line / block / doc
}
```

`mappings.json` 中以 `comments[ID][语言码] = 文本` 保存各语言文本，位置信息见 `index.json`。

### 5.3 作为库嵌入

模块路径为 `github.com/CairnPack/codei18n`，根包提供 `codei18n.Open(root, opts...)`，同一进程可同时打开多个项目。详见 [docs/library-api.md](docs/library-api.md)。

### 5.4 构建要求

* Go 1.27.1
* `CGO_ENABLED=1`：tree-sitter 通过 cgo 编译 C 源码；Windows 上需要 MinGW gcc（并保证在 `PATH` 中）。

```bash
CGO_ENABLED=1 go build ./cmd/codei18n
```

---

## 6. 注释 ID 设计（关键）

### 6.1 ID 生成原则

注释 ID 必须满足：

* 稳定
* 与代码语义绑定
* 不依赖行号

### 6.2 ID 计算方式

```text
ID = SHA1(
    file_path + "|" +
    language + "|" +
    parent_symbol + "|" +
    normalized_comment_text
)
```

📌 **parent_symbol 示例**：

* Go：`package.func`
* Rust：`impl::fn`
* Java：`Class#method`

连续行注释合并后按一条注释计算 ID。消费方应把 ID 视为不透明字符串。

---

---

## 7. 多自然语言支持设计

### 7.1 映射文件结构

```json
{
  "version": "1.0",
  "sourceLanguage": "en",
  "targetLanguage": "zh-CN",
  "comments": {
    "a8f9c3e2": {
      "en": "Calculate account balance",
      "zh-CN": "计算账户余额"
    }
  }
}
```

### 7.2 存储策略

* 默认路径：`.codei18n/`（完整布局见 §14.3）
* 映射文件 `mappings.json` 为本地使用，**默认不提交 Git**（`.codei18n/` 会被加入 `.gitignore`）
* 位置索引 `index.json` 记录每条注释的位置、类型与识别出的原语言，格式见 [docs/index-format.md](docs/index-format.md)
* 注释按识别出的原语言存放，规则见 §14.7

---

## 8. 多编程语言支持策略

### 8.1 编程语言适配器接口

```go
type LanguageAdapter interface {
    Language() string
    Parse(file string, src []byte) ([]*domain.Comment, error)
}
```

### 8.2 已支持的编程语言

Go 使用 `go/parser` / `go/ast`；其余语言统一走 Tree-sitter 适配器。

| 语言 | 扩展名 | 说明 |
| --- | --- | --- |
| Go | `.go` | `go/ast`，支持 `//`、`/* */`、Doc comment |
| Rust | `.rs` | `//`、`///`、`//!`、`/** */` |
| Java | `.java` | groovy、scala 没有独立适配器，借用 Java 语法解析（符号可能不准） |
| Kotlin | `.kt` `.kts` | 独立 Kotlin 语法 |
| TypeScript | `.ts` `.tsx` `.mts` `.cts` | `.tsx` 使用 tsx 语法 |
| JavaScript | `.js` `.jsx` `.mjs` `.cjs` | 使用 javascript 语法 |
| C# | `.cs` | `///` 为文档注释 |
| Dart | `.dart` | 内置 tree-sitter-dart（ABI 14），语法版本早于 Dart 3.10，新语法下注释仍可提取但符号可能不准 |

### 8.3 连续行注释的统一合并规则

所有语言统一：标记相同、相邻行、同列、独占一行的连续行注释合并为一条记录（记录 `parts`），整组为空时丢弃；行尾注释始终单独成条。完整规则见 [docs/index-format.md §6.1](docs/index-format.md)。

---

## 9. IDE 支持设计

### 9.1 IDE 统一交互模式

IDE 插件通过调用 CLI 获取结构化数据：

```bash
codei18n scan --file xxx.go --lang zh-CN --format json
```

---

## 10. VS Code 插件设计

### 10.1 渲染方式

* `TextEditorDecorationType`
* 覆盖 / 附加显示中文注释
* 不修改文档内容

```ts
after: {
  contentText: "计算账户余额"
}
```

---

## 11. JetBrains IDE 插件设计

### 11.1 插件类型

* IntelliJ Platform Plugin
* Kotlin 实现
* 多 IDE 共用

---

### 11.2 渲染方案（推荐）

#### Inlay Hint

```text
// Calculate account balance
// ↓ 计算账户余额
```

* 使用 `InlayModel`
* 官方推荐方式
* 不影响 PSI / LSP

---

### 11.3 备选方案

* Gutter icon + hover tooltip

---

## 12. Git 提交流程设计

### 12.1 pre-commit Hook

`codei18n hook install` 安装 `pre-commit` 与 `commit-msg` hook（`hook uninstall` 卸载）。pre-commit 的流程为：

```text
git commit
 └─ pre-commit
    ├─ codei18n map update   更新映射与索引
    ├─ codei18n translate    翻译缺失条目（失败时跳过并提示检查 codei18n auth status）
    └─ codei18n convert --to en --file <暂存文件>   把暂存文件的注释改写为英文
```

hook 使用项目配置选用的 provider 和令牌（环境变量或凭据库），不再强制 `--provider openai`。升级后已安装的旧 hook 需重新执行 `codei18n hook install` 才会更新。

### 12.2 行为原则

* 只处理 staged 文件（当前 hook 脚本只匹配暂存的 `.go` 文件）
* 翻译失败不阻断提交（hook 中跳过并提示）

## 13. 翻译引擎设计

### 13.1 抽象接口

```go
type Translator interface {
    Translate(ctx context.Context, text, from, to string) (string, error)
    TranslateBatch(ctx context.Context, texts []string, from, to string) ([]string, error)
}
```

### 13.2 实现策略

* 本地缓存优先，优先复用已有翻译结果，减少对外部服务的调用频率。
* 支持的翻译后端（通过 `translationProvider` 或配置档的 `provider` 选择）：

    * `openai` / `llm`：基于 OpenAI 兼容协议的远程 LLM（如官方 OpenAI、DeepSeek 或自建代理），令牌见 §14.4，可选 `OPENAI_BASE_URL`。
    * `ollama`：本地 Ollama 服务，通过 REST API 调用本地模型（如 `llama3`、`qwen3` 等）。
    * `mock`：仅用于测试和集成测试场景，不用于生产环境。
* 避免在 Git 提交路径上频繁同步调用大模型，可通过预翻译批量填充映射文件。
* 所有长任务支持 `context` 取消（CLI 中 Ctrl+C 可取消），取消后已完成的结果已落盘。

### 13.3 翻译服务配置示例

CodeI18n 的翻译服务配置由两部分组成：

1. 顶层字段：`translationProvider`（选择后端）
2. 细节字段：`translationConfig`（不同后端的参数）

#### 13.3.1 使用 OpenAI / DeepSeek / 兼容 LLM

```json
{
  "sourceLanguage": "en",
  "localLanguage": "zh-CN",
  "translationProvider": "openai",
  "translationConfig": {
    "baseUrl": "https://api.openai.com/v1",
    "model": "gpt-4.1-mini"
  },
  "batchSize": 10
}
```

运行前需要提供 API 令牌，推荐保存到系统凭据库（见 §14.4）：

```bash
codei18n auth set --profile default
```

也可以使用环境变量（优先级高于凭据库）：

```bash
export CODEI18N_API_KEY="your-api-key"   # 或沿用 OPENAI_API_KEY
# 可选：自建代理或第三方 OpenAI 兼容服务
export OPENAI_BASE_URL="https://api.openai.com/v1"
```

如果 `model` 设置为 `deepseek-chat` 或 `deepseek-coder`，系统会自动将 `baseUrl` 设为 `https://api.deepseek.com`，也可以手动通过 `baseUrl` 显式指定其他兼容服务。

#### 13.3.2 使用本地 Ollama

```json
{
  "sourceLanguage": "en",
  "localLanguage": "zh-CN",
  "translationProvider": "ollama",
  "translationConfig": {
    "endpoint": "http://localhost:11434",
    "model": "qwen3:4b"
  },
  "batchSize": 5
}
```

说明：

* `endpoint`：Ollama HTTP 服务地址，默认 `http://localhost:11434`。
* `model`：本地已拉取的模型名称，例如 `qwen3:4b`、`llama3` 等。
* CodeI18n 在调用 Ollama 时会显式设置 `"think": false`，关闭思维链模式，避免额外的延迟和算力消耗。

启用本地 Ollama 集成测试示例（非必须）：

```bash
CODEI18N_OLLAMA_TEST=1 \
CODEI18N_OLLAMA_MODEL=qwen3:4b \
go test ./adapters/translator -run TestOllamaTranslator_Translate_Integration -v
```

#### 13.3.3 通过 CLI 覆盖配置

在 `.codei18n/config.json` 配置好默认翻译服务后，也可以在命令行临时覆盖：

```bash
# 覆盖 provider 为 openai
codei18n translate --provider openai

# 覆盖 provider 为本地 ollama
codei18n translate --provider ollama

# 临时指定模型（会覆盖 translationConfig.model）
codei18n translate --model gpt-4.1-mini
```

#### 13.3.4 从 Google / DeepL 迁移

历史版本中可能使用过 `translationProvider: "google"` 或 `"deepl"`。迁移建议：

1. 打开 `.codei18n/config.json`，删除或重命名以下配置：
   - 将 `"translationProvider": "google"` 或 `"deepl"` 改为 `"openai"` 或 `"ollama"`；
   - 如有旧的 `google`/`deepl` 相关字段，可以移除，仅保留 `baseUrl`/`model` 等通用字段。
2. 为 `openai` 模式配置：
   - 设置 `translationProvider: "openai"`；
   - 在 `translationConfig` 中指定 `model`，必要时指定 `baseUrl`；
   - 在环境中配置 `OPENAI_API_KEY`（以及可选的 `OPENAI_BASE_URL`）。
3. 或者切换到本地 `ollama` 模式：
   - 设置 `translationProvider: "ollama"`；
   - 在 `translationConfig` 中设置 `endpoint` 和本地模型名称；
4. 运行：

```bash
codei18n translate
```

   确认命令可以正常执行且不会再出现“翻译提供商 google/deepl 已不再支持”的错误提示。

---

## 14. 配置文件设计

### 14.1 分层规则

配置按以下优先级合并，后者覆盖前者：

```text
内置默认  <  ~/.codei18n/config.json（全局）  <  <项目>/.codei18n/config.json（项目）  <  CLI 参数
```

库 API 的 `Project.Config()` 返回合并后的值，并标明每个字段来自哪一层（`default` / `global` / `project` / `override`）。项目显式选用了 `profile` 时，项目配置里旧版的 provider 字段（旧版 `init` 生成的快照）会被忽略，以免遮住所选配置档。

CLI 全局参数：`--config <path>` 指定配置文件，`--verbose` 显示详细日志。

### 14.2 全局配置档（profiles）与项目覆盖

全局配置 `~/.codei18n/config.json` 中用 `profiles` 定义多个命名配置档，项目用 `profile` 选用其中一个，并可用 `profileOverrides` 覆盖单个字段。配置档支持的字段：`provider`、`baseUrl`、`model`、`endpoint`、`batchSize`、`concurrency`、`apiKeyRef`。

全局 `~/.codei18n/config.json`：

```json
{
  "sourceLanguage": "en",
  "localLanguage": "zh-CN",
  "detectLanguages": ["en", "zh-Hans", "zh-Hant", "ja", "ko", "de", "fr", "es", "ru"],
  "profiles": {
    "openai-mini": {
      "provider": "openai",
      "baseUrl": "https://api.openai.com/v1",
      "model": "gpt-4.1-mini",
      "batchSize": 10,
      "concurrency": 5
    },
    "local": {
      "provider": "ollama",
      "endpoint": "http://localhost:11434",
      "model": "qwen3:4b",
      "batchSize": 5
    }
  }
}
```

项目 `<项目>/.codei18n/config.json`：

```json
{
  "projectId": "3f2b8c1e-0000-0000-0000-000000000000",
  "sourceLanguage": "en",
  "localLanguage": "zh-CN",
  "excludePatterns": [".git/**", "vendor/**", ".codei18n/**"],
  "detectLanguages": ["en", "zh-Hans", "ja"],
  "profile": "openai-mini",
  "profileOverrides": {
    "model": "gpt-4.1",
    "batchSize": 20
  }
}
```

说明：

* `projectId` 由 `codei18n init` 生成，用于项目级令牌的凭据库账户名（见 §14.4）。
* `detectLanguages` 是语种识别的候选集合，默认 `en, zh-Hans, zh-Hant, ja, ko, de, fr, es, ru`；识别结果不在候选集内时标为 `unknown`。
* §13.3 中的 `translationProvider` / `translationConfig` 是旧版平铺字段，仍然可用。

### 14.3 `.codei18n/` 目录布局

```text
<项目根>/.codei18n/
├─ config.json      # 项目配置
├─ mappings.json    # 注释 ID → 语言 → 文本
├─ index.json       # 位置索引（格式见 docs/index-format.md）
├─ overrides.json   # 用户手动纠正的注释原语言，扫描不会覆盖
└─ cache/           # 增量扫描与语种识别缓存，可随时删除
```

所有 JSON 文件都以“写临时文件 + rename”的方式原子写入。库 API 的 `EnsureGitignore` 与 CLI 初始化流程会把 `.codei18n/` 加入 `.gitignore`（如尚未包含）。

### 14.4 令牌存储

翻译 API 令牌不写入配置文件、日志、错误信息或进度事件，而是保存在系统凭据库中。

* 存储后端：目前仅实现 Windows 凭据管理器（generic credential，TargetName 为 `codei18n/<account>`）。其它平台请使用环境变量。
* account 命名：
    * 全局配置档令牌：`profile/<name>`
    * 项目级覆盖令牌：`project/<projectId>/<name>`
* 单个令牌上限 2560 字节。
* 读取优先级（先命中者生效）：

    1. 环境变量 `CODEI18N_API_KEY`
    2. 环境变量 `OPENAI_API_KEY`
    3. 项目级覆盖令牌（`project/<projectId>/<name>`）
    4. 所选配置档的全局令牌（`profile/<name>`，全局配置中配置档的 `apiKeyRef` 可指向其它 account）
    5. 配置文件中的旧版明文字段（仍可使用，但会给出警告，应迁移）

    以上均未命中时报错，并提示设置方式。

* 安全限制：项目配置会随仓库分发，因此不可信。
    * 项目文件中的 `apiKeyRef` 会被忽略。
    * 项目文件把 `baseUrl` / `endpoint` 改成与全局配置不同的地址时，不会使用全局令牌（`auth status` 中显示为已阻止）。这种情况下请用 `codei18n auth set --project` 为该项目保存专用令牌，或把地址写进全局配置的 profile。

命令：

```bash
# 保存全局令牌（只从标准输入或隐藏输入读取，不接受命令行参数）
codei18n auth set --profile openai-mini
echo "$KEY" | codei18n auth set --profile openai-mini

# 保存当前项目的覆盖令牌
codei18n auth set --profile openai-mini --project

# 查看是否已保存、末 4 位与生效来源层
codei18n auth status --profile openai-mini

# 删除令牌
codei18n auth delete --profile openai-mini --project

# 把配置文件中的旧版明文令牌迁入凭据库，并从文件中删除
codei18n auth migrate
```

`--profile` 省略时使用当前生效的配置档；`status` 只显示末 4 位。

### 14.5 扫描性能与增量缓存

* 并行扫描：worker pool（默认 CPU 核数），每个 worker 持有自己的解析器。
* 增量扫描：`.codei18n/cache/scan.gob` 记录每个文件的 size、mtime、内容 hash 与解析结果；未变化的文件直接复用，全部命中且映射/索引/覆盖文件都未变时直接返回。
* 语种识别结果按文本 hash 缓存（`cache/lang.gob`）。
* 翻译时方向相同、归一化原文相同的条目只请求一次；映射文件按批次节流保存，结束（含取消）时必定保存。
* `codei18n map update --no-cache` 不使用缓存，重新解析所有文件。
* `codei18n map update --prune` 删除源码中已不存在的映射条目；默认不删，只显示可清理的数量。只能在扫描整个项目时使用，指定 `--scan-dir` 子目录时会被拒绝。

### 14.6 文档注释结构化翻译

对 Javadoc、KDoc、JSDoc/TSDoc、Dartdoc、C# XML doc 等文档注释，翻译前先把代码引用、类型、参数名、标签、代码块等受保护片段替换为占位符 `⟦n⟧`，只翻译剩余散文；翻译后还原，并校验占位符一个不少、一个不多、顺序不变，再按原注释的外框与缩进重新组装。

* 校验失败重试 1 次，仍失败则标记为失败，不写入错误译文。
* 正文全是受保护片段的注释不发请求，直接沿用原文。
* 需要 `index.json`（据此选择格式）；没有 `index.json` 时保持旧行为。
* 已知限制：Markdown 缩进式代码块不受保护；`a<b>` 之类文本可能被当作 HTML 标签保护。

### 14.7 注释语种识别

* 识别使用裁剪后内置的 lingua（`third_party/lingua`，仅保留 en/zh/ja/ko/de/fr/es/ru 模型），简繁中文按字表区分。
* 识别前会剥掉注释符号、受保护片段、代码、标识符、URL、路径等；剩余文本过短时用字符集启发式。
* 无法判断时标记为 `unknown`，不强行归类；识别结果不在 `detectLanguages` 候选集内时同样为 `unknown`。
* 存放规则：与本地语言/源语言等价时用配置写法（如 `zh-CN`）；无法识别时按源语言存放；第三种语言（如日文）按规范码（如 `ja`）存放。
* 翻译目标为 {源语言, 本地语言} 中与原语言不等价且缺失的语言；第三种语言两者都补。
* 用户可通过 `overrides.json`（库 API：`SetCommentLanguage`）纠正原语言，纠正优先于识别结果。
* 限制：只含汉字的日文会被判为中文。

---

## 15. 项目目录结构

```text
codei18n/
├─ cmd/codei18n        # CLI（库之上的薄壳）
├─ codei18n.go ...     # 根包：Open / Project / Global 等库 API
├─ core/               # 配置、映射、索引、工作流、语种识别、文档注释格式、凭据库等
├─ adapters/           # 各编程语言适配器（含 commentgroup、translator）
├─ third_party/        # 内置的第三方代码（裁剪版 lingua、tree-sitter-dart）
├─ docs/               # 文档（index-format.md、library-api.md 等）
└─ openspec/           # 变更提案与规范
```

---

## 16. 开发工作流与 Makefile

本项目提供了完整的 Makefile 来简化日常开发操作。

### 16.1 常用命令

```bash
# 查看所有可用命令
make help

# 构建可执行文件
make build

# 运行所有测试（跳过集成测试）
make test

# 运行所有测试（包括集成测试，需要有效的 API key）
make test-integration

# 生成测试覆盖率报告
make coverage

# 查看 HTML 格式的覆盖率报告
make coverage-html

# 格式化代码
make fmt

# 运行代码检查
make lint

# 运行所有质量检查（fmt + vet + lint）
make check

# 开发模式：快速检查
make dev

# 完整 CI 流程
make all

# CI/CD 模式（包含覆盖率验证）
make ci

# Pre-commit 检查
make pre-commit
```

### 16.2 代码质量要求

根据项目章程要求：

- **总体测试覆盖率**: ≥ 60%
- **核心模块覆盖率**: ≥ 80%（`core/comment`, `core/mapping`, `core/translate`）
- **代码规范**: 必须通过 `gofmt` 和 `golint`/`staticcheck` 检查

使用以下命令验证：

```bash
# 检查覆盖率是否达标
make coverage-check

# 运行完整的 CI 检查
make ci
```

### 16.3 开发工作流示例

#### 日常开发

```bash
# 1. 编写代码
# 2. 快速验证
make dev

# 3. 提交前检查
make pre-commit

# 4. 构建
make build
```

#### 提交前完整检查

```bash
# 运行完整 CI 流程
make ci
```

### 16.4 Docker 支持

项目支持 Docker 容器化部署：

```bash
# 构建 Docker 镜像
make docker-build

# 运行 Docker 容器
make docker-run

# 在 Docker 中运行测试
make docker-test

# 清理 Docker 镜像
make docker-clean
```

### 16.5 CI/CD 集成

项目配置了完整的 GitHub Actions CI/CD 流程：

#### 自动化检查

每次 push 或 pull request 时自动运行：

- ✅ 代码质量检查（gofmt、go vet、staticcheck）
- ✅ 多平台构建测试（Linux、macOS、Windows）
- ✅ 单元测试和覆盖率报告
- ✅ 集成测试
- ✅ 安全扫描（gosec、govulncheck）
- ✅ 依赖审计

#### 自动发布

推送版本 tag 时自动构建和发布：

```bash
# 创建版本 tag
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0

# GitHub Actions 将自动：
# - 构建多平台二进制文件
# - 生成 Release Notes
# - 创建 GitHub Release
# - 构建并推送 Docker 镜像（如果配置）
```

更多详情请查看 [CI/CD 工作流说明](.github/README.md)。

---

## 17. Roadmap

* 已完成：Go / Rust / Java / Kotlin / TypeScript / JavaScript / C# / Dart 适配器，库化 API（`codei18n.Open`），位置索引 `index.json`，系统凭据库令牌，增量并行扫描，文档注释结构化翻译，多语种识别。
* 进行中：桌面端与 IDE 插件基于 `index.json` 的精确匹配。
* 计划：更多编程语言（如 Python）、非 Windows 平台的凭据库实现（目前请使用环境变量）。


---

## 18. 结论

**CodeI18n 本质是一个“代码注释 i18n 基础设施”**：

* Git 负责事实
* AST 负责语义
* IDE 负责体验

这是一个 **长期存在、但一直没人系统解决的问题**。

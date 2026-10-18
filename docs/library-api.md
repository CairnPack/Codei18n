# 库 API：把 CodeI18n 嵌入你的程序

CodeI18n 的 CLI 只是库之上的薄壳。桌面端或其它 Go 程序可以直接引用根包，在同一进程内同时管理多个项目。

```go
import "github.com/CairnPack/codei18n"
```

模块路径 `github.com/CairnPack/codei18n`，需要 Go 1.27.1 和 `CGO_ENABLED=1`（Windows 上需要 MinGW gcc，tree-sitter 依赖 cgo）。

库代码不打印、不调用 spinner、不 `os.Exit`（由 `purity_test.go` 静态检查）；日志通过可注入的 `Logger` 输出。

## 1. 打开项目

```go
func Open(root string, opts ...Option) (*Project, error)
```

`root` 必须是已存在的目录，项目不必已初始化。同一进程可多次调用，各 `Project` 互不影响。`Project` 的方法可以从多个 goroutine 调用。

```go
p, err := codei18n.Open("C:/work/myproj",
    codei18n.WithLogger(myLogger),
)
if err != nil { /* ... */ }
if !p.Initialized() {
    err = p.Init(codei18n.InitOptions{})
}
```

### 1.1 选项

| 选项 | 作用 |
| --- | --- |
| `WithLogger(l Logger)` | 注入日志。`Logger` 有 `Debugf` / `Infof` / `Warnf` 三个方法；库不会把令牌放进日志参数。 |
| `WithGlobalDir(dir string)` | 指定全局配置目录（默认 `~/.codei18n`），便于测试隔离。 |
| `WithConfigFile(path string)` | 指定配置文件路径（对应 CLI 的 `--config`）。使用后不再读取全局层，`GlobalConfigPath()` 为空。 |
| `WithOverrides(ov config.Overrides)` | 调用方临时覆盖（最高优先级，不写入任何文件），可覆盖 `SourceLanguage`、`LocalLanguage`、`ExcludePatterns`、`Profile`、`Provider`。 |
| `WithTranslator(t core.Translator)` | 注入翻译后端（测试或自定义后端），忽略配置中的 provider。 |
| `WithSecretStore(s SecretStore)` | 注入令牌存储，默认使用系统凭据库；测试可用 `secrets.NewMemory()`。 |
| `WithEnv(getenv func(string) string)` | 注入环境变量读取函数（默认 `os.Getenv`），用于 `CODEI18N_API_KEY` / `OPENAI_API_KEY`。 |

### 1.2 常用只读方法

```go
func (p *Project) Root() string
func (p *Project) DataDir() string              // <root>/.codei18n
func (p *Project) Initialized() bool
func (p *Project) Config() (*config.Effective, error)
func (p *Project) UpdateConfig(fn func(*config.File) error) error
```

`Config()` 每次重新读取文件，返回合并后的值，每个字段带来源层：`default` < `global` < `project` < `override`。

## 2. 长任务、取消与进度

扫描、`map update`、翻译、convert 都接受 `context.Context`：

```go
func (p *Project) Scan(ctx context.Context, opts ScanOptions) (*ScanResult, error)
func (p *Project) UpdateMappings(ctx context.Context, opts MapOptions) (*MapUpdateResult, error)
func (p *Project) Translate(ctx context.Context, opts TranslateOptions) (*TranslateResult, error)
func (p *Project) Convert(ctx context.Context, opts ConvertOptions) (*ConvertResult, error)
func (p *Project) TranslateText(ctx context.Context, text, from, to string) (string, error)
```

* 取消 `ctx` 后任务在一个批次内停下，已完成的结果已落盘。`Translate` 取消时返回 `ctx.Err()`，且结果中 `Canceled` 为 `true`。
* `Scan` 只读，不写任何文件；`UpdateMappings` 写 `mappings.json` 并原子写出 `index.json`。
* `MapOptions` 字段：`ScanDir`、`DryRun`、`Prune`（删除源码中已不存在的条目，只能扫描整个项目时使用）、`NoCache`（禁用增量扫描缓存）、`Workers`（<=0 为 CPU 核数）、`Progress`。
* `TranslateOptions` 字段：`Concurrency`（<=0 用配置值）、`OnlyIDs`（只翻译这些 ID，用于失败重试）、`Progress`。
* `TranslateText` 的 `from` / `to` 为空时是源语言 → 本地语言；它不加项目锁。

### 2.1 进度事件

```go
type ProgressFunc = events.Func   // func(events.Event)

type Event struct {
    Phase Phase  // 阶段
    Done  int
    Total int
    File  string // 当前文件（可选）
    Err   string // 已脱敏的错误描述（可选）
}
```

阶段常量（`codei18n.Phase` 的取值）：`discover`（枚举文件）、`scan`（解析）、`map`（合并到 mappings）、`index`（写位置索引）、`translate`（调用翻译后端）、`save`（落盘）、`convert`（改写源码）、`done`（结束，`Done == Total`）。

回调可能在任意 goroutine 上被调用，需要自行切回 UI 线程；回调应尽快返回。

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

res, err := p.UpdateMappings(ctx, codei18n.MapOptions{
    Prune: false,
    Progress: func(e codei18n.Event) {
        fmt.Printf("%s %d/%d %s\n", e.Phase, e.Done, e.Total, e.File)
    },
})
```

### 2.2 ErrBusy 语义

写入映射的任务（`UpdateMappings`、`Translate`、`Convert`、`SetTranslation`、`SetCommentLanguage`、`MigratePlaintextTokens`）在同一个 `Project` 上互斥。已有任务在运行时立即返回 `codei18n.ErrBusy`，不排队、不阻塞。调用方可据此提示“项目正忙”。只读方法（`Scan`、`Status`、`Comments`、`Config`）不受影响。

其它哨兵错误：`ErrNotInitialized`（项目未初始化）、`ErrAlreadyInitialized`（已初始化且未设置 `Force`）、`ErrUnknownComment`（mappings 中没有该 ID）、`ErrNoToken`（找不到可用令牌）。

## 3. 浏览与编辑注释

```go
func (p *Project) Status() (*Status, error)
func (p *Project) Comments() ([]CommentRecord, error)
func (p *Project) SetTranslation(id, lang, text string) error
func (p *Project) SetCommentLanguage(id, lang string) error
func (p *Project) Failures() ([]Failure, error)
```

* `Status()` 是“快速载入”：只读 mappings、index 与缓存，不触发扫描。字段：`Initialized`、`HasMappings`、`HasIndex`、`Comments`（总数）、`Translated`、`Pending`（待翻译数）、`Failed`、`Languages`（各原语言的注释数）、`IndexGeneratedAt`。
* `Comments()` 返回 `index.json` 位置信息与 `mappings.json` 文本合并后的记录（`ID`、`File`、`Language`、`Type`、`Range`、`Parts`、`Symbol`、`Lang`、`LangSource`、`SourceKey`、`Original`、`Texts`、`Error`），按文件与位置排序，不在索引中的条目排最后；`record.Translation(lang)` 按等价语言码取文本。
* `SetTranslation(id, lang, text)` 手动设置某语言文本并原子写回 `mappings.json`；`text` 为空表示删除该语言文本；ID 不存在返回 `ErrUnknownComment`。
* `SetCommentLanguage(id, lang)` 纠正识别出的原语言，持久化到 `overrides.json`（下次扫描不会覆盖），同时把原文移到新语言对应的 mappings 键下并更新 `index.json`；`lang` 为空表示撤销纠正。

```go
recs, _ := p.Comments()
for _, r := range recs {
    if r.Lang == "unknown" {
        _ = p.SetCommentLanguage(r.ID, "ja")
    }
}
_ = p.SetTranslation(recs[0].ID, "zh-CN", "计算账户余额")
```

## 4. 全局配置档与令牌（Global）

不依赖任何项目，管理 `~/.codei18n/config.json` 中的配置档及其令牌：

```go
func OpenGlobal(opts ...Option) (*Global, error)   // 支持 WithGlobalDir / WithSecretStore / WithLogger / WithEnv

func (g *Global) Profiles() ([]string, error)
func (g *Global) SaveProfile(name string, fields config.ProviderFields) error
func (g *Global) CopyProfile(from, to string) error     // 不复制令牌
func (g *Global) RenameProfile(from, to string) error   // 迁移全局与项目令牌
func (g *Global) DeleteProfile(name string) error       // 一并删除令牌

func (g *Global) SetProfileToken(profile string, s Secret) error
func (g *Global) DeleteProfileToken(profile string) error
func (g *Global) ProfileTokenStatus(profile string) (saved bool, last4 string, err error)
func (g *Global) PlaintextTokens() ([]PlaintextToken, error)
func (g *Global) MigratePlaintextTokens() ([]MigratedToken, error)
```

配置档名称需通过 `ValidateProfileName`；重复、不存在时分别返回 `ErrProfileExists` / `ErrProfileNotFound`。

项目级令牌：

```go
func (p *Project) SetProjectToken(profile string, s Secret) error
func (p *Project) DeleteProjectToken(profile string) error
func (p *Project) MigratePlaintextTokens() ([]MigratedToken, error)
```

### 4.1 Secret

库 API 里没有接收令牌明文字符串的参数，使用 `Secret`：

```go
s := codei18n.NewSecret([]byte(key))
err := g.SetProfileToken("openai-mini", s)
```

`Secret` 的 `String` / `Format` / `MarshalJSON` 只输出脱敏形式，不会因误打印而泄漏。

### 4.2 TokenStatus

```go
func (p *Project) TokenStatus() (*TokenStatus, error)
```

返回当前生效的令牌来源与末 4 位，不含明文。主要字段：

| 字段 | 含义 |
| --- | --- |
| `Profile` | 生效的配置档名 |
| `Required` | 当前 provider 是否需要令牌（ollama / mock 不需要） |
| `Source` | `env` / `project` / `global` / `config-file` / `none` |
| `Last4` | 生效令牌的末 4 位 |
| `EnvVar` | `Source` 为 `env` 时的环境变量名 |
| `ProjectAccount` / `ProjectSaved` / `ProjectLast4` | 项目级覆盖令牌的 account 与保存情况 |
| `GlobalAccount` / `GlobalSaved` / `GlobalLast4` | 全局令牌的 account 与保存情况 |
| `Plaintext` | 配置文件中发现的旧版明文字段（不含值），应调用迁移 |
| `StoreError` | 凭据库不可用时的错误（例如非 Windows 平台） |

令牌读取优先级：`CODEI18N_API_KEY` > `OPENAI_API_KEY` > 项目级覆盖令牌 > 配置档全局令牌 > 旧版明文字段（带警告）。account 命名为 `profile/<name>` 与 `project/<projectId>/<name>`，详见根目录 README 的“令牌存储”一节。

## 5. 其它

* `Init(InitOptions)`：写入 `.codei18n/config.json` 并生成 `projectId`；`Force` 覆盖已有配置但保留原 `projectId`。
* `InstallHooks()` / `HooksInstalled()` / `IsGitRepo()` / `EnsureGitignore()`：Git 集成。
* 位置索引格式见 [index-format.md](index-format.md)。

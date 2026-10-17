# 变更：库化重构（A1）与位置索引 index.json（A6）

## 为什么

桌面端（Codei18nDesktop）需要在同一进程内直接调用 core，同时管理多个项目；IDE 插件需要按"文件 + 范围 + 文本"精确匹配注释。现有实现路径写死为相对 cwd 的 `.codei18n/...`、配置依赖全局 viper 单例、spinner 直接写 stderr、`context.Background()` 无法取消、`log.Fatal` 会退出进程，无法作为库使用；mappings.json 只有 ID → 文本，没有位置信息。

## 变更内容

- 新增模块根包 `codei18n`：`Open(root, opts...) (*Project, error)`，同一进程可同时打开多个 Project，互不影响。
- 所有长任务（扫描、map update、翻译、convert）接受 `context.Context`，取消后在 1 个批次内停下、已完成结果已落盘；通过 `ProgressFunc` 发出结构化进度事件（阶段、done/total、当前文件、错误）。
- 日志改为可注入的 `Logger` 接口；库代码不打印、不调用 spinner、不 `os.Exit`（由 `purity_test.go` 静态检查）。spinner 与彩色输出只放在 `cmd/`。
- 配置分层：内置默认 < 全局 `~/.codei18n/config.json` < 项目 `.codei18n/config.json` < 调用方覆盖；`Effective` 返回合并后的值与每个字段的来源层。
- 翻译 Provider 配置档：全局 `profiles` 定义多个命名 profile（provider、baseUrl、model、endpoint、batchSize、concurrency、apiKeyRef）；项目用 `profile` 选用并以 `profileOverrides` 覆盖单个字段。
- 新增 `.codei18n/index.json` 位置索引（格式见 `docs/index-format.md`），由 `map update` 原子写出；mappings.json 格式不变。
- 新增 `.codei18n/overrides.json`：用户手动纠正的原语言，扫描不覆盖。
- 所有 JSON（mappings.json、index.json、config.json、overrides.json）一律"写临时文件 + rename"原子写；convert 改写源码也改为原子写。
- 模块路径改为 `github.com/CairnPack/codei18n`，Go 升级到 1.27.1，移除 viper 依赖。
- CLI 命令（init/scan/map update|get/translate/convert/hook）参数与行为保持兼容，成为库之上的薄壳；Ctrl+C 可取消长任务。
- 旧的库函数（`config.Config`、`config.LoadConfig`、`workflow.MapUpdate`、`workflow.Translate`、`translator.NewFromConfig`、`scanner.Directory`）保留为已弃用的兼容包装。

## 影响

- 受影响规范：library-api（新增）、index-format（新增）
- 受影响代码：`codei18n.go` `tasks.go` `browse.go` `auth.go`（新增），`core/config`（重写）、`core/workflow`、`core/scanner`、`core/mapping`、`core/index`（新增）、`core/langcode`（新增）、`core/gitint`（新增）、`internal/atomicfile`（新增）、`cmd/codei18n/*`
- 行为差异（均为保守选择，见 design.md）：
  - pre-commit hook 不再强制 `--provider openai`，改用项目配置选用的 provider；已安装的旧 hook 需重新 `codei18n hook install` 才会更新。
  - 项目显式选用 profile 时，项目配置中的旧版 provider 字段（旧版 init 生成的快照）被忽略。

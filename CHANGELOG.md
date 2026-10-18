# 更新日志

本文档记录 CodeI18n 项目的所有重要变更。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [未发布]

### 破坏性变更
- 模块路径改为 `github.com/CairnPack/codei18n`，Go 升级到 1.27.1，移除 viper 依赖
- 构建需要 `CGO_ENABLED=1`（tree-sitter）；Windows 上需要 MinGW gcc
- pre-commit hook 不再强制 `--provider openai`，改用项目配置选用的 provider；已安装的旧 hook 需重新执行 `codei18n hook install`
- 连续行注释按统一规则合并为一条记录（标记相同、相邻行、同列、独占一行），记录数减少；Kotlin 注释的符号修正后 ID 改变，`map update` 会按原文迁移旧译文
- 移除“含汉字即视为中文”的判断，改为多语种识别

### 新增
- 库化 API：`codei18n.Open(root, opts...)`，支持 `WithLogger` / `WithGlobalDir` / `WithConfigFile` / `WithOverrides` / `WithTranslator` / `WithSecretStore` / `WithEnv`；长任务支持 `context` 取消与 `ProgressFunc` 进度事件，同项目写任务互斥并返回 `ErrBusy`（见 `docs/library-api.md`）
- 配置分层：内置默认 < `~/.codei18n/config.json` < 项目配置 < CLI 参数；全局 `profiles` 配置档，项目用 `profile` 选用并以 `profileOverrides` 覆盖
- 位置索引 `.codei18n/index.json`（格式见 `docs/index-format.md`）与用户纠正文件 `.codei18n/overrides.json`；所有 JSON 原子写入
- 令牌存入系统凭据库（目前为 Windows 凭据管理器，`codei18n/<account>`）；读取优先级 `CODEI18N_API_KEY` > `OPENAI_API_KEY` > 项目级令牌 > 配置档全局令牌 > 旧版明文（带警告）
- CLI：`codei18n auth set|status|delete [--profile <name>] [--project]` 与 `codei18n auth migrate`（`set` 只从标准输入或隐藏输入读取）
- 新增 Kotlin（`.kt` `.kts`）、C#、Dart 支持；补齐 TypeScript / JavaScript 扩展名（`.mts` `.cts` `.mjs` `.cjs`）
- 并行与增量扫描（`.codei18n/cache/`）；`map update --prune` 清理孤儿条目，`map update --no-cache` 禁用缓存
- 翻译去重与节流保存
- 文档注释结构化翻译：受保护片段以占位符 `⟦n⟧` 保护，翻译后校验并还原
- 多语种识别：可配置候选集合 `detectLanguages`，无法判断时为 `unknown`，第三种语言按规范码存放

### 改进
- 重写 README，补充语言支持、配置分层、令牌存储、`.codei18n/` 目录布局等说明，并移除已失效的描述

## [未发布（早期：CI/CD 与构建）]

### 新增
- 添加 GitHub Actions CI/CD 工作流
  - 代码质量检查（gofmt, go vet, staticcheck）
  - 多平台构建测试（Linux, macOS, Windows）
  - 单元测试和覆盖率报告
  - 集成测试
  - 安全扫描（gosec, govulncheck）
  - 依赖审计
- 添加自动发布工作流
  - 多平台二进制文件构建
  - Docker 镜像构建和发布
  - 自动生成 Release Notes
- 添加 Makefile 用于常用操作
- 添加 Dockerfile 支持容器化部署
- 添加依赖自动更新工作流

### 改进
- 更新 .gitignore 添加更多忽略模式
- 更新 README.md 添加开发工作流说明
- 优化构建流程

## [0.1.0] - TBD

### 新增
- Go 语言 AST 适配器
- 注释扫描和提取功能
- 注释 ID 生成逻辑
- 多语言映射管理
- 基于 Cobra 的 CLI 框架
- 基础配置管理

### 核心功能
- `scan` 命令：扫描源码并提取注释
- `init` 命令：初始化项目配置
- `map` 命令：管理多语言映射
- `translate` 命令：自动翻译注释
- `convert` 命令：转换注释语言
- `hook` 命令：管理 Git Hooks

[未发布]: https://github.com/studyzy/codei18n/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/studyzy/codei18n/releases/tag/v0.1.0

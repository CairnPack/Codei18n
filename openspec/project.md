# 项目上下文

## 目的
CodeI18n 是一个面向工程团队的**代码注释国际化基础设施**，目标是在不破坏 Git 语义、不污染源码、不影响编译与调试的前提下，实现：

- 仓库中代码注释统一使用一种语言（推荐英文）
- 开发者本地在 IDE 中以母语阅读和编写注释
- 在 Git 提交与 IDE 展示之间实现自动、可逆的注释语言转换
- 基于 AST/PSI 严格绑定注释与代码语义，避免行号/正则方式带来的不稳定

## 技术栈
- 编程语言：Go 1.27.1（`go.mod:3`），模块路径 `github.com/CairnPack/codei18n`
- 构建与命令行：原生 Go + Makefile + Cobra CLI 框架（`go.mod:5-13`）
- 配置管理：`core/config` 分层配置（默认 < 全局 < 项目 < 调用方覆盖），不再使用 Viper
- 语法解析：go/parser、go/ast（Go 语言）；后续通过 tree-sitter 适配多语言（`CODEBUDDY.md:193-203`）
- 终端交互：fatih/color、briandowns/spinner（仅限 `cmd/`，库代码禁止使用）
- 测试框架：testing + testify（`go.mod:12`）
- 持续集成：GitHub Actions（`.github/README.md:5-43`）
- 容器化：可选 Docker 支持，通过 Makefile 封装（`README.md:520-536`）

> 本仓库专注于 Core 和 CLI，引导 VS Code / JetBrains 插件在独立仓库实现（`CODEBUDDY.md:261-270,334-335`）。

## 项目约定

### 代码风格
- 遵循 Effective Go 与 Go Code Review Comments（`CONTRIBUTING.md:104-105`）
- 必须通过 `gofmt`、`go vet` 和 `staticcheck`/`golint` 检查（`CODEBUDDY.md:66-75`,`CONTRIBUTING.md:111-124`）
- 文档与代码注释默认使用中文；函数/类型命名使用英文，符合 Go 社区规范（`CONTRIBUTING.md:135-139`）
- 所有注释处理必须基于 AST/PSI，禁止基于行号或正则表达式的实现（`CODEBUDDY.md:136-139,319-321`,`CONTRIBUTING.md:140-142`）

### 架构模式
- 分层架构：
  - Core（Go）：注释解析、ID 生成、多语言映射、翻译引擎（`CODEBUDDY.md:83-95,136-138`）
  - 语言适配器：针对不同语言的 AST 解析器与注释抽取逻辑（`CODEBUDDY.md:96-100,182-191`）
  - IDE 插件：VS Code / JetBrains，仅负责渲染和交互，不包含 AST/翻译逻辑（`CODEBUDDY.md:101-104,261-273`）
- CLI 优先：核心能力通过 `codei18n` 命令行暴露，IDE 与 Git Hook 通过 CLI 集成（`CODEBUDDY.md:77-81,274-278,324-326`）
- 注释模型统一抽象，使用稳定的 SHA1 ID 绑定注释与语义符号，保证重构后的稳定性（`CODEBUDDY.md:140-158,167-175`）

### 测试策略
- 使用 Go 原生测试框架 + testify 进行单元测试和集成测试（`go.mod:12`,`README.md:262-273`）
- 通过 `go test ./...` 或 `make test` 运行全部测试；关键路径提供集成测试与端到端 CLI 测试（`CODEBUDDY.md:53-64`,`README.md:445-449`,`README.md:276-278`）
- 覆盖率要求：
  - 全局覆盖率 ≥ 60%（`CODEBUDDY.md:58-63,322`）
  - 核心模块 `core/comment`、`core/mapping`、`core/translate` 覆盖率 ≥ 80%（`CODEBUDDY.md:322`,`CONTRIBUTING.md:126-133`）
- 提交前必须通过 `make pre-commit` 或 `make ci`，其中包含格式化、lint、测试和覆盖率检查（`CONTRIBUTING.md:56-63`,`README.md:463-473`）

### Git 工作流
- 分支：远程以 `main`/`master`/`develop` 为主干，特性开发从主干创建 `feature/*` 分支（`.github/README.md:9-12`,`CONTRIBUTING.md:46-49`）
- 提交信息：采用类 Conventional Commits 风格，前缀包括 `feat`/`fix`/`docs`/`style`/`refactor`/`test`/`chore`，提交说明使用简洁中文（`CONTRIBUTING.md:68-79`）
- PR 流程：所有变更通过 Pull Request 合并，要求 CI 全绿、描述清晰且聚焦单一目的（`.github/README.md:112-117`,`CONTRIBUTING.md:87-96`）
- 发布：维护者通过创建 `vX.Y.Z` tag 触发 GitHub Actions 自动构建和发布多平台二进制与（可选）Docker 镜像（`.github/README.md:47-80,118-129`,`CONTRIBUTING.md:191-201`）
- Git Hook：预期集成 pre-commit hook，在提交前扫描 staged 文件、检测非英文注释并完成翻译与映射更新（`CODEBUDDY.md:243-260`,`README.md:348-360`）

## 领域上下文
- 目标场景：中大型工程团队，希望在保持仓库注释统一语言的同时，为不同母语的开发者提供友好的本地阅读/编写体验（`README.md:13-35`）
- 注释是代码语义的一部分：所有决策围绕“注释与代码语义强绑定”展开，避免因重构/格式化导致映射丢失（`README.md:69-85`,`CODEBUDDY.md:123-136`）
- 多语言支持：当前重点在英文 ↔ 中文（简体），未来通过适配器和翻译引擎扩展到更多自然语言和编程语言（`README.md:41-52,202-218`）
- IDE 体验优先：终端工具与 IDE 插件共同构成“基础设施 + 渲染层”的组合，确保不会直接修改源码，只在视图层展示本地语言注释（`README.md:289-307,317-345`）

## 重要约束
- AST 优先：所有注释解析 **必须** 基于 AST/PSI，禁止使用正则或行号（`CODEBUDDY.md:319-321`,`CONTRIBUTING.md:140-142`）
- 单一语言源：Git 仓库中只存储一种注释语言；多语言内容仅存在于本地映射文件和 IDE 视图中（`CODEBUDDY.md:125-129,222-227`,`README.md:59-66`）
- 代码规范：必须通过 gofmt、go vet、staticcheck/golint，遵守 Effective Go 和 Go Code Review Comments（`CODEBUDDY.md:66-75,321-323`,`CONTRIBUTING.md:102-124`）
- 覆盖率门槛：总覆盖率 ≥ 60%，核心模块 ≥ 80%，作为 CI 的硬约束（`CODEBUDDY.md:58-63,322`,`CONTRIBUTING.md:126-133`）
- CLI 与 IDE 解耦：核心逻辑只暴露为 CLI，IDE 插件和 Git Hook 不得重新实现 AST/翻译逻辑（`CODEBUDDY.md:136-139,261-273,324-326`）

## 外部依赖
- 翻译后端：DeepL、OpenAI 以及自定义术语词典，通过 `Translator` 接口抽象（`CODEBUDDY.md:205-212,214-220,273-275`）
- 配置与映射存储：
  - `.codei18n/config.json` 用于配置源语言、本地语言和 IDE 行为（`CODEBUDDY.md:280-297`,`README.md:391-405`）
  - `.codei18n/` 目录用于存储本地注释映射文件，默认不提交到 Git（`CODEBUDDY.md:222-240`）
- CI/CD 与质量平台：GitHub Actions、（可选）Codecov 以及 Docker Registry，用于自动测试、覆盖率上报和镜像分发（`README.md:539-569`,`.github/README.md:47-80,118-149`）

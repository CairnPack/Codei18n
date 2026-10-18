# 设计：库化重构与位置索引

## 包结构

| 包 | 职责 |
| --- | --- |
| `codei18n`（模块根） | 公开入口：`Open`、`Project`、选项、进度/日志类型别名 |
| `core/config` | 单层配置文件 `File`（保留未知字段）与合并结果 `Effective`（带来源层） |
| `core/workflow` | 长任务实现：`UpdateMappings`、`TranslateMissing`、`Convert`、`TranslateTextWith`；运行环境 `Env` |
| `core/scanner` | 目录扫描（ctx、进度、逐文件 contentHash 与 UTF-16 坐标） |
| `core/index` | index.json / overrides.json 读写、文本归一化、`textHash`、坐标换算 |
| `core/langcode` | BCP 47 规范化与等价（与 IDEA 插件共享测试向量） |
| `core/langdetect` | 语种识别接口；本变更只提供复现旧规则的 `Legacy` 实现 |
| `core/gitint` | git hook 与 .gitignore |
| `core/logging`、`core/events` | Logger 接口、进度事件 |
| `internal/atomicfile` | 原子写 |

## 关键决策

1. **无全局状态**：配置每次由 `Project.Config()` 从文件重新读取并合并；`Project` 之间不共享可变状态。tree-sitter Parser 每次 `GetAdapter` 新建，不跨 goroutine 共享。
2. **同项目互斥**：写映射的长任务（UpdateMappings / Translate / Convert / SetTranslation / SetCommentLanguage）在同一个 Project 上用 `TryLock` 互斥，冲突时返回 `ErrBusy`，避免翻译中途的手动编辑被翻译保存覆盖。GUI 在任务进行中禁用编辑。
3. **取消语义**：翻译派发循环在每个批次前检查 ctx；进行中的请求随 ctx 取消；因取消而中断的批次不计成功也不计失败；`wg.Wait()` 后保存已完成结果，返回 `ctx.Err()` 与 `Canceled=true` 的结果。
4. **provider 合并顺序**：默认 < 全局旧版字段 < 全局中选用的 profile < 项目旧版字段 < 项目 profileOverrides < 调用方覆盖。项目显式选用 profile 时忽略项目旧版 provider 字段——旧版 `init` 会把全局配置快照进项目文件，不忽略的话 GUI 选的 profile 会被快照遮住。（brief 未覆盖，保守假设）
5. **mappings.json 语言键**：原文按识别出的语言存放；与本地语言等价时使用配置里本地语言的写法（例如 `zh-CN`），其余使用源语言写法，与旧版行为一致，老插件不受影响。index.json 的 `sourceKey` 记录实际键。
6. **index.json 坐标**：列为 UTF-16 码元（与 IntelliJ Document 一致），`scan --format json` 的 `range` 保持字节列（convert 依赖）。
7. **textHash 归一化**与注释 ID 的归一化分开：ID 沿用 `core/utils.NormalizeCommentText`，保证已有 mappings 的 ID 不变；textHash 用逐行去标记的新规则，便于插件从文档文本复现。
8. **pre-commit hook**：去掉 `--provider openai`，使用项目选用的 provider 与凭据库令牌（A7）。hook 只处理 `.go` 文件的行为不变（扩展到其他语言会在提交时改写更多源码，属于对外行为变化，不在本次范围）。
9. **Windows 原子写**：读者以不含 `FILE_SHARE_DELETE` 的方式打开目标文件时 rename 会被拒绝，按指数退避重试约 3 秒。

## 兼容性

- `codei18n init` 仍把合并后的配置（去掉敏感字段）快照写入项目文件，并新增 `projectId`。
- `--config <file>`：只使用该文件（作为项目层），不读取全局与默认位置，与旧版一致。
- 旧版 viper `AutomaticEnv` 的环境变量覆盖配置键的能力被移除（旧实现实际只对已绑定的键生效，未见文档使用）。

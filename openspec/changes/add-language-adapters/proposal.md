# 变更：补齐 Kotlin / C# / Dart 支持，统一注释合并规则（A3）

## 为什么

目标语言为 Kotlin、Java、TypeScript、JavaScript、Dart、C#。现状：`.kt` 被错误地用 Java 语法解析；C# 与 Dart 不支持；TS/JS 扩展名不全；连续行注释在 core 中逐行入库，而 IDE 中是多个 `PsiComment`，各语言规则也不统一，插件难以按"范围 + parts"匹配。

## 变更内容

- 新增 `adapters/commentgroup`：所有语言统一的"物理注释 → 注释记录"规则（docs/index-format.md §6.1/§6.3）：标记相同、相邻行、同列、独占一行的行注释合并为一条，记录 `parts`；整组为空时丢弃；类型按文本判定。
- 新增 `adapters/kotlin`（smacker kotlin 语法）、`adapters/csharp`（smacker csharp 语法，`///` 为文档注释）、`adapters/dart`（引入 tree-sitter-dart @c8e7cbbd 的 C 源码，ABI 14，自写 cgo 绑定，附 MIT 许可证与重新生成说明）。
- 既有 go / rust / java / typescript adapter 改用 commentgroup；单条注释的符号与 ID 保持不变（全仓库 918 条单条注释逐一核对）。
- TS/JS：补齐 `.mjs .cjs .mts .cts`；`.tsx` 用 tsx 语法，`.js/.jsx/.mjs/.cjs` 用 javascript 语法；`Comment.Language` 仍为 `typescript` 以保持 ID 稳定，index.json 的精确语言由扩展名推导。
- 各 adapter 的 tree-sitter Query 只编译一次并复用，每个 Adapter 持有自己的 Parser（为并行扫描做准备）。
- `map update` 迁移旧 ID 的译文：合并前逐行 ID 的译文按行连接继承；符号规则变化（如 Kotlin）导致的新 ID 从唯一的同原文孤儿条目继承。避免规则调整后重复付费翻译。

## 影响

- 受影响规范：language-adapters（新增）
- 受影响代码：`adapters/*`、`adapters/registry.go`、`core/workflow/migrate.go`、`tests/rust_integration_test.go`
- 行为差异：
  - 连续行注释合并后记录数减少（例如 tests 中 lib.rs 由 11 条变为 7 条，测试已按规则更新）。
  - Rust `/** */` 的类型由 block 改为 doc（符合 §6.3，ID 不变）。
  - Kotlin 注释的符号修正后 ID 改变，旧译文通过原文迁移继承；有歧义的不迁移。
  - Dart 语法版本早于 Dart 3.10（dot-shorthand 等新语法会产生 ERROR 节点，注释仍可提取，符号可能不准）。

# 任务列表：语言支持

- [x] 1. commentgroup：合并规则、类型规则、空组丢弃
- [x] 2. Kotlin adapter + 符号解析 + 单测
- [x] 3. C# adapter + 符号解析 + 单测（`///` XML doc）
- [x] 4. Dart 语法 cgo 绑定（ABI 14、许可证、README）+ adapter + 单测
- [x] 5. 既有 adapter 迁移到 commentgroup，Query 只编译一次，Parser 每 Adapter 一个
- [x] 6. TS/JS 扩展名与语法选择
- [x] 7. registry 注册
- [x] 8. 旧 ID 译文迁移（合并迁移 + 原文迁移）
- [x] 9. 每种语言的测试：行注释、块注释、文档注释、字符串中的伪注释、连续行注释合并、行尾注释不合并、符号稳定性
- [x] 10. 验证：单条注释 ID 不变（全仓库对比）；旧 CLI → 新 CLI 跨版本迁移无需重新翻译；`go test -race ./...`

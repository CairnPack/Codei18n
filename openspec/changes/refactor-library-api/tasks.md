# 任务列表：库化重构与位置索引

## 1. 准备
- [x] 1.1 模块路径改为 `github.com/CairnPack/codei18n`，Go 升级到 1.27.1
- [x] 1.2 编写 `docs/index-format.md` 与共享测试向量 `docs/testdata/*.json`

## 2. 基础包
- [x] 2.1 `internal/atomicfile`：临时文件 + rename，Windows 重试
- [x] 2.2 `core/logging`、`core/events`
- [x] 2.3 `core/langcode`：规范化与等价，跑共享测试向量
- [x] 2.4 `core/index`：读写、归一化、textHash、UTF-16 坐标、overrides.json

## 3. 配置
- [x] 3.1 `core/config.File`：指针字段、保留未知字段、原子更新
- [x] 3.2 `core/config.Resolve`：四层合并、来源层、profile 与 profileOverrides
- [x] 3.3 保留旧版 `Config` / `LoadConfig` / `SaveConfig` 兼容包装

## 4. 工作流
- [x] 4.1 `scanner.Scan`：ctx、进度、Logger、contentHash、UTF-16 坐标
- [x] 4.2 `workflow.UpdateMappings`：写 mappings 与 index.json，尊重 overrides
- [x] 4.3 `workflow.TranslateMissing`：ctx 取消、进度、失败记录、错误脱敏
- [x] 4.4 `workflow.Convert`：从 cmd 移入库，原子写源码
- [x] 4.5 旧函数保留为兼容包装

## 5. 公开 API 与 CLI
- [x] 5.1 `codei18n.Open/Project`：Init、Config、UpdateConfig、Scan、UpdateMappings、Translate、TranslateText、Convert、Status、Comments、SetTranslation、SetCommentLanguage、Failures、git 集成
- [x] 5.2 CLI 改为薄壳；移除 viper；Ctrl+C 取消
- [x] 5.3 `purity_test.go` 静态检查库代码不打印、不退出

## 6. 验证
- [x] 6.1 `go test -race ./...` 通过（含既有集成测试）
- [x] 6.2 修复 Windows 上 `TestInitGlobalConfigInheritance` 只设置 HOME 的预先存在问题

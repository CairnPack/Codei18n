# 变更：扫描与翻译性能优化（A2）

## 为什么

现有扫描逐文件顺序解析、每个文件重新编译 tree-sitter Query；每次 map update 都全量重扫；翻译时每批全量重写 mappings.json，相同原文重复请求。中大型项目扫描慢、翻译贵，且频繁重写文件会触发 IDE 插件反复重载。

## 变更内容

- 并行扫描：worker pool（默认 `runtime.NumCPU()`），每个 worker 持有自己的 adapter / Parser；并发目录遍历，直接使用目录项的 size/mtime。
- 增量扫描：`.codei18n/cache/scan.gob` 记录每个文件的 size、mtime、内容 hash 与解析结果；size+mtime 快筛，变化时用 hash 复核；mtime 落在上次写缓存前后 2 秒内的文件强制复核（避免同一时钟周期内的修改被漏掉）。缓存目录已在默认 exclude（`.codei18n/**`）中，并自动写入 `.gitignore`（`*`）。
- 无改动快速路径：扫描全部命中缓存，且 mappings / index / overrides 的 size+mtime、语言设置、识别器标识都与上次一致时直接返回。
- 增量写盘：mappings.json 只在内容实际变化时写；index.json 按语义摘要判断。所有 JSON 写入仍为原子写。
- 语种识别结果按归一化文本 hash 缓存（`.codei18n/cache/lang.gob`）。
- 翻译去重：方向相同、归一化原文相同的条目只请求一次，结果写回所有 ID。
- 保存节流：每 N 批（默认 10）或每 T 秒（默认 5 秒）保存一次，在独立 goroutine 中进行，结束（含取消）时必定保存。
- 孤儿清理：`map update --prune` 删除源码中已不存在的 ID；默认不删，只显示可清理的数量；扫描子目录时拒绝 prune。
- 基准：`internal/benchgen`（`go run ./scripts/genbench -out <dir>`）确定性生成约 2000 个文件的样例项目；`core/scanner` 与 `core/workflow` 下的 `Benchmark*`。

## 基准数据

环境：Windows 11，16 逻辑核（以 `-cpu 8` 模拟 8 核），Go 1.27.1，MinGW gcc 16，样例 2000 个文件、约 8 MB。

| 场景 | 耗时 | 说明 |
| --- | --- | --- |
| 重构前 main，`scanner.Directory`（顺序） | 3279 ms | 只含 go/java/kt/ts/js/rs（main 不支持 cs/dart） |
| 本变更，全量扫描，1 worker | 约 1240 ms | 8 种语言 |
| 本变更，全量扫描，8 worker | 约 207 ms | **相对 main 15.8×**，相对 1 worker 6.0× |
| 本变更，无改动重扫（扫描层） | 15.8 ms | **全量的 7.6%** |
| 本变更，首次 map update（含写 mappings/index） | 约 460–550 ms | |
| 本变更，无改动 map update | 17.9 ms | 首次的约 3.5% |

复现：

```bash
go run ./scripts/genbench -out /tmp/bench            # 或让基准自动生成
go test -run x -bench . -benchtime 10x -cpu 8 ./core/scanner/
go test -run x -bench MapUpdate -benchtime 10x -cpu 8 ./core/workflow/
```

## 影响

- 受影响规范：performance（新增）
- 受影响代码：`core/scanner`、`core/workflow`、`core/mapping`、`internal/benchgen`、`scripts/genbench`、`cmd/codei18n/map.go`
- 新增文件：`.codei18n/cache/{scan.gob,lang.gob,mapstate.json,.gitignore}`，均可随时删除。

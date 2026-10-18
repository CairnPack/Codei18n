# 任务列表：性能

- [x] 1. 并行扫描 worker pool、每 worker 独立 adapter、并发目录遍历
- [x] 2. 增量扫描缓存（size+mtime 快筛、hash 复核、同周期修改保护、紧凑编码、无变化不写盘）
- [x] 3. 无改动快速路径与失效条件测试
- [x] 4. mappings / index 增量写盘
- [x] 5. 语种识别结果缓存
- [x] 6. 翻译去重
- [x] 7. 保存节流（独立 goroutine，结束必定保存，取消时保存）
- [x] 8. `map update --prune` 与孤儿数量提示
- [x] 9. 样例项目生成器与基准；PR 中给出优化前后数据

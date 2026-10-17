# 规范增量：位置索引 index.json

## 新增需求

### 需求：map update 写出位置索引

`map update` 必须原子写出 `.codei18n/index.json`，格式遵循 `docs/index-format.md`（版本 1），包含每个文件的 contentHash、编程语言，以及每条注释的 ID、范围（UTF-16 列）、类型、原语言、原文所在键、textHash、parts 与符号。

#### 场景：中文注释
- **当** 项目本地语言为 zh-CN，源码中有 `// 你好世界`
- **那么** index.json 中该条目 `lang` 为 `zh-Hans`、`sourceKey` 为 `zh-CN`，range 的列按 UTF-16 计算

### 需求：mappings.json 向后兼容

mappings.json 的结构保持 ID → 语言 → 文本不变，不读 index.json 的旧插件照常工作。

#### 场景：旧插件读取
- **当** 旧版插件只读取 mappings.json
- **那么** 能按原有规则找到译文

### 需求：手动纠正的原语言持久化

用户纠正的原语言保存在 `.codei18n/overrides.json`，后续扫描不覆盖。

#### 场景：重新扫描
- **当** 用户把某条注释纠正为 en 后再次执行 map update
- **那么** index.json 中该条目仍为 `lang=en`、`langSource=override`

### 需求：跨语言一致的归一化与语言码

textHash 归一化与语言码等价规则由 Go 与 Java 两端共享测试向量保证一致。

#### 场景：共享测试向量
- **当** 运行 `core/index` 与 `core/langcode` 的单测
- **那么** `docs/testdata/text-normalization.json` 与 `language-codes.json` 中的全部用例通过

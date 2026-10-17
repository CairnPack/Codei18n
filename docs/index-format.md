# 位置索引 `.codei18n/index.json` 格式说明（版本 1）

`index.json` 是 `codei18n map update` 写出的**位置索引**，记录每条注释在源文件中的位置、类型、识别出的原语言等信息。它是 IDE 插件（精确匹配、无 PSI 回退）和桌面端（注释浏览器、统计）共同依赖的契约。

- `mappings.json` 的格式**不变**，老版本插件不读 `index.json` 也能照常工作。
- `index.json` 与 `mappings.json` 一样，总是以"写临时文件 + rename"的方式**原子写出**，读方不会读到半截 JSON。
- 本文档中的"必须 / 不得"是对生产方（core）和消费方（插件、桌面端）的共同约束。

## 1. 文件位置

```
<root>/
└── .codei18n/
    ├── config.json
    ├── mappings.json      # ID → 语言 → 文本（格式不变）
    ├── index.json         # 本文档描述的位置索引
    ├── overrides.json     # 用户手动纠正的原语言（见 §7）
    └── cache/             # 增量扫描缓存，可随时删除，不应入库
```

`<root>` 是包含 `.codei18n` 目录的目录，下文称"项目根目录"。

## 2. 顶层结构

```json
{
  "version": "1",
  "generatedAt": "2026-10-18T12:00:00Z",
  "generator": "codei18n/1.2.0",
  "sourceLanguage": "en",
  "localLanguage": "zh-CN",
  "files": {
    "src/main/kotlin/Foo.kt": {
      "contentHash": "3f786850e387550fdab836ed7e6dc881de23001b",
      "language": "kotlin",
      "comments": [
        {
          "id": "9a0364b9e99bb480dd25e1f0284c8555f5f2b4a1",
          "range": { "startLine": 3, "startCol": 1, "endLine": 5, "endCol": 4 },
          "type": "doc",
          "lang": "en",
          "langSource": "detected",
          "sourceKey": "en",
          "textHash": "2fd4e1c67a2d28fced849ee1bb76e7391b93eb12",
          "parts": 1,
          "symbol": "com.example.Foo#bar"
        }
      ]
    }
  }
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `version` | string | 格式版本，当前为 `"1"`。消费方遇到**不认识的主版本**必须忽略整个文件、回退到不使用索引的行为。 |
| `generatedAt` | string | 生成时间，RFC 3339 UTC。 |
| `generator` | string | 生成工具及版本，仅供诊断。 |
| `sourceLanguage` / `localLanguage` | string | 生成时项目配置中的源语言、本地语言（保持配置原样，例如 `zh-CN`）。 |
| `files` | object | 键为文件路径（§3），值为文件条目（§4）。只包含至少有一条注释的文件。 |

消费方必须忽略不认识的字段；生产方新增字段时不升主版本。

## 3. 路径

- 相对于**项目根目录**（`.codei18n` 所在目录）。
- 分隔符统一为 `/`，不以 `./` 或 `/` 开头。
- 大小写与磁盘上一致；不做 Unicode 归一化。

注意：注释 ID 的计算方式沿用 `mappings.json` 的既有规则（见 `core/utils/id.go`），其中的路径部分取决于扫描目录。消费方必须把 ID 视为**不透明字符串**，不得自行计算 ID。

## 4. 文件条目

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `contentHash` | string | 生成索引时文件**磁盘原始字节**的 SHA-1，小写十六进制，40 个字符。 |
| `language` | string | 编程语言标识：`go` `rust` `java` `kotlin` `typescript` `tsx` `javascript` `jsx` `csharp` `dart`（未来可能增加）。 |
| `comments` | array | 按 `range` 起始位置升序排列的注释条目（§5）。 |

## 5. 注释条目

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 与 `mappings.json` 中 `comments` 的键相同。 |
| `range` | object | 注释在文件中的位置（§5.1）。合并注释（`parts > 1`）为从第一条物理注释开头到最后一条结尾。 |
| `type` | string | `line`（`//` 等行注释）、`block`（`/* */`）、`doc`（文档注释：`/** */`、`///`、`//!` 等，具体见 §6.3）。 |
| `lang` | string | 识别出的注释原语言，规范化 BCP 47 语言码（§8）；判断不了时为 `"unknown"`。 |
| `langSource` | string | `detected`（识别库）、`heuristic`（短文本字符集启发式）、`override`（用户手动纠正，§7）、`fallback`（无法识别，`lang` 为 `unknown`）。 |
| `sourceKey` | string | 该注释**原文**在 `mappings.json` 中存放的语言键（例如 `en`、`zh-CN`）。它可能与 `lang` 写法不同（`lang` 是规范码 `zh-Hans`，`sourceKey` 是配置写法 `zh-CN`），`lang` 为 `unknown` 时一般等于 `sourceLanguage`。 |
| `textHash` | string | 归一化文本（§6.2）的 SHA-1，小写十六进制。 |
| `parts` | integer | 该条目由几条**物理注释**合并而来（§6.1），≥ 1。 |
| `symbol` | string | 注释所属的语义符号路径，可能为空字符串。仅供显示，不参与匹配。 |

### 5.1 `range` 的坐标

- `startLine` / `endLine`：从 **1** 开始的行号。行分隔符为 `\n`；`\r\n` 视为一个分隔符（`\r` 不计入列）。
- `startCol` / `endCol`：从 **1** 开始的列号，单位是 **UTF-16 码元**（与 IntelliJ `Document`、VS Code、LSP 默认一致）。制表符计为 1 列。
- `startCol` 指向注释第一个字符（例如 `/` 或 `/*` 的 `/`）；`endCol` 是注释最后一个字符**之后**的位置（左闭右开）。
- 示例：第 3 行 `    // 你好` 中注释从第 5 列开始，`endCol` 为 `5 + len16("// 你好") = 10`。

> 说明：`codei18n scan --format json` 输出的 `range` 仍沿用旧语义（列为字节偏移），供 `convert` 改写源码使用，与本索引不同。

## 6. 注释的切分、合并与归一化

### 6.1 连续行注释合并规则（所有编程语言统一）

扫描器先得到每一条**物理注释**（一个 `//...` 行注释或一个 `/* ... */` 块注释），再按下列规则把相邻的物理注释合并成一条记录：

两条物理注释 A（在前）、B（在后）合并，当且仅当**全部**满足：

1. A、B 都是行注释（不是 `/* */` 块注释）；
2. A、B 的**标记相同**：标记指注释开头连续的 `/` 与紧随其后的一个可选 `!`，即 `//`、`///`、`//!`、`////` 互不相同；
3. B 的起始行 = A 的结束行 + 1（中间没有空行、没有代码行）；
4. A、B 的起始列相同；
5. A、B 都**独占一行**：注释前面只有空白字符（行尾注释 `x = 1 // foo` 永远单独成条，不与上下行合并）。

合并后的记录：

- 原文 = 各物理注释原文按顺序以 `\n` 连接（每行保留自己的 `//` 标记，不含缩进）；
- `range` 从 A 的开头到最后一条的结尾；`parts` = 物理注释条数；
- `type`：标记为 `///` 或 `//!`，且该语言把它们视为文档注释时（§6.3）为 `doc`，否则为 `line`。

只由空白和标记组成的物理注释（例如单独的 `//`）可以出现在合并组的中间（表示段落分隔），但**整组归一化后为空**的记录会被丢弃，不写入索引和 mappings。

IDE 插件依赖本规则：在 PSI 中是多个相邻的 `PsiComment`，插件需要按同样的规则聚合成组，再与 `parts > 1` 的条目匹配，并把译文显示在**整组**下方。

### 6.2 文本归一化（`textHash` 用，版本 1）

输入为合并后的原文（磁盘文本中 `range` 覆盖的子串，或各物理注释按 §6.1 连接后的文本；两者归一化结果相同）。

1. 把 `\r\n` 与单独的 `\r` 替换为 `\n`，按 `\n` 切分成行。
2. 对每一行：
   1. 去掉首尾空白（空白 = Unicode `White_Space` 属性的字符；Java 端**不能**直接用 `String.strip()`/`trim()`，二者的定义不同）；
   2. 按顺序检查并**只去掉一个**开头标记：`/**`、`/*!`、`/*`、`///`、`//!`、`//`；都不匹配时，如果行以 `*` 开头且不以 `*/` 开头，去掉这一个 `*`；
   3. 如果此时行以 `*/` 结尾，去掉结尾的 `*/`。
3. 把所有行用一个空格连接，再把连续空白（同上定义）压缩为一个空格，去掉首尾空白。
4. `textHash` = 结果按 UTF-8 编码后的 SHA-1，小写十六进制。

示例：

| 原文 | 归一化结果 |
| --- | --- |
| `// Hello   world` | `Hello world` |
| `/// Line one\n/// Line two` | `Line one Line two` |
| `/**\n * Returns the sum.\n * @param a first\n */` | `Returns the sum. @param a first` |
| `/* 你好 */` | `你好` |

`core/index` 包的单测和插件的 `MappingIndexTest` 使用同一组测试向量（`docs/testdata/text-normalization.json`）。

### 6.3 各语言的注释类型

| 语言 | `doc` | `block` | `line` |
| --- | --- | --- | --- |
| Java | `/** */`（Javadoc） | `/* */` | `//` |
| Kotlin | `/** */`（KDoc） | `/* */` | `//` |
| TypeScript / JavaScript | `/** */`（JSDoc/TSDoc） | `/* */` | `//`、`///`（三斜线指令不是文档注释） |
| C# | `///`（XML 文档注释）、`/** */` | `/* */` | `//` |
| Dart | `///`、`/** */`（Dartdoc） | `/* */` | `//` |
| Rust | `///`、`//!`、`/** */`、`/*! */` | `/* */` | `//` |
| Go | — | `/* */` | `//` |

## 7. 手动纠正的原语言 `overrides.json`

桌面端允许用户纠正某条注释识别出的原语言。纠正结果持久化在 `.codei18n/overrides.json`，下次扫描**不会覆盖**：

```json
{
  "version": "1",
  "languages": {
    "9a0364b9e99bb480dd25e1f0284c8555f5f2b4a1": "ja"
  }
}
```

`map update` 写索引时，`languages` 中出现的 ID 使用纠正后的语言，`langSource` 为 `override`。

## 8. 语言码

- 全项目使用规范化 BCP 47 语言码。规范化函数 `Canonical(code)`（Go：`core/langcode`；Java：插件 `LanguageCodes`）：
  1. 去掉首尾空白，`_` 视为 `-`，大小写不敏感；空串返回空串；`und`、`unknown` 返回 `unknown`；
  2. 主语言子标签做别名替换：`zho` `chi` `cmn` → `zh`，`eng` → `en`，`jpn` → `ja`，`kor` → `ko`，`deu` `ger` → `de`，`fra` `fre` → `fr`，`spa` → `es`，`rus` → `ru`，`iw` → `he`，`in` → `id`，`ji` → `yi`；
  3. 中文：有文字子标签 `Hant` 或地区子标签 `TW` `HK` `MO` 时为 `zh-Hant`，其余（含 `zh`、`zh-CN`、`zh-SG`、`zh-Hans`）为 `zh-Hans`；
  4. 其他语言只保留小写的主语言子标签（`en-US` → `en`，`pt-BR` → `pt`）。
- `Equivalent(a, b)`：二者 `Canonical` 后相等且不为空。
- 在 `mappings.json` 中查找某种语言的文本时，先按原写法查键，找不到再在该条目的所有键中找与之 `Equivalent` 的键。
- Go 与 Java 两端共享测试向量 `docs/testdata/language-codes.json`。

## 9. 消费方匹配建议（IDE 插件）

1. 读取 `index.json`；不存在、无法解析或主版本不认识时，退回到只用 `mappings.json` 的全局文本匹配（与旧版本行为一致）。
2. 找到当前文件的条目：
   - 文件内容的 SHA-1 与 `contentHash` 一致：直接按 `range` 定位；
   - 不一致（文件已修改）：在该文件的条目集合内按 `textHash` 匹配。
3. 对每个命中的条目，用 `sourceKey`（或 `lang` 的等价键）取原文，用显示语言的等价键取译文。原文语言已经是显示语言时不显示。
4. 都未命中时，再用全局文本匹配（沿用歧义规则：同一原文对应多个不同译文时不显示）。
5. 无 PSI 时（例如 Rider 的 C#、未装 Dart 插件的 IDE）：按 `range` 取文档文本，归一化后 `textHash` 一致才显示，否则不显示。

## 10. 版本演进

- 新增可选字段：不升版本，消费方忽略未知字段。
- 改变已有字段的含义、坐标单位、归一化或合并规则：`version` 升为 `"2"`，并在本文档记录差异。

# 变更：文档注释结构化翻译（A4）

## 为什么

KDoc、Javadoc、JSDoc/TSDoc、Dartdoc、C# XML doc 中混有代码引用、类型、参数名、标签与代码块。整段交给 LLM 翻译时，这些片段常被翻译、改写或丢失，IDE 端也无法再按原规则高亮。

## 变更内容

- 新增 `core/docfmt`：把文档注释切分为外框（`/**`、行首 `*`、`///`、`*/`）、可翻译散文段与受保护片段；受保护片段替换为占位符 `⟦n⟧`；翻译后还原，并校验占位符一个不少、一个不多、顺序不变；按原注释的外框与缩进重新组装（译文行数可以变化）。
  - Javadoc：`{@link …}` `{@code …}` `{@linkplain …}` 等内联标签、`@param <name>` / `@throws <Type>` 的名称部分、块标签本身、HTML 标签（`<code>`/`<pre>` 连同内容）。
  - KDoc / Dartdoc：`[ref]`、反引号代码、代码块、块标签及其名称。
  - JSDoc / TSDoc：`{type}`、`@param {T} name` 的类型与参数名、`@example` 代码块、`{@link …}`、`@remarks` 等标签。
  - C# XML doc：`<see cref="…"/>`、`<paramref name="…"/>`、`<c>…</c>`、`<code>…</code>` 以及标签结构本身。
- 翻译流程：根据 index.json 中的编程语言与注释类型选择格式；校验失败重试 1 次，仍失败则标记为失败、不写入坏译文；正文全是受保护片段的注释不发请求、直接沿用原文。没有 index.json 时保持旧行为。
- LLM prompt（OpenAI 兼容与 Ollama，单条与批量）明确要求保留占位符（数量、顺序）与 Markdown 结构。
- 译文保留原注释的标签结构，IDE 端可以用同一套规则高亮。

## 影响

- 受影响规范：doc-translation（新增）
- 受影响代码：`core/docfmt`（新增）、`core/workflow/translate*.go`、`adapters/translator/{llm,ollama}.go`
- 行为差异：mock 后端对文档注释的输出由"前缀 + 整段原文"变为"外框内的正文加前缀"（仅测试后端）。
- 已知限制：Markdown 缩进式代码块未受保护；`a<b>` 这类文本可能被当作 HTML 标签保护；单行块注释译文变成多行时，新行不带原首行的列缩进。

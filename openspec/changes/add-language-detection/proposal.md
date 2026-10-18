# 变更：多语种识别（A5）

## 为什么

旧规则"含汉字即 zh-CN，否则视为源语言"无法区分日文、韩文、德文等，也无法区分简体与繁体；IDE 插件与桌面端需要每条注释可靠的原语言，用来决定显示什么、统计原语言占比，并允许用户纠正。

## 变更内容

- `core/langdetect.New(candidates)`：基于 lingua-go v1.4.0 的**裁剪内置副本**（`third_party/lingua`，Apache-2.0，附 NOTICE / README）。上游无论选几种语言都会嵌入全部 75 种语言模型（二进制约 +130 MB），本副本只保留 en/zh/ja/ko/de/fr/es/ru 且去掉 fivegram 模型，二进制约 +5.9 MB，准确率在回归语料上与完整模型一致。
- 识别前剥掉注释符号、受保护片段、代码、标识符、URL、路径等；剩余文本过短（< 3 个拉丁词或 < 4 个 CJK 字）时用字符集启发式（汉字 / 假名 / 谚文 / 西里尔 / 拉丁）；判断不了标记 `unknown`，不强行归类。简繁用 OpenCC 字表（简体独有 3716 字、繁体独有 4005 字）按多数判断。
- 候选语言集合可配置（`detectLanguages`，默认 en、zh-Hans、zh-Hant、ja、ko、de、fr、es、ru）；识别结果不在候选集内时为 `unknown`。
- 规范化 BCP 47 语言码与等价映射（`core/langcode`，与 IDEA 插件共享测试向量）。
- `map update`：注释按识别出的语言存放——与本地语言 / 源语言等价时用配置写法（如 `zh-CN`，老插件兼容），无法识别时用源语言写法（旧行为），第三种语言用规范码（如 `ja`）。结果写入 index.json 的 `lang` / `langSource` / `sourceKey`。
- 翻译：有 index.json 时目标为 {源语言, 本地语言} 中与原语言不等价且缺失的语言（第三种语言两者都补）；没有 index.json 时沿用旧规则。
- 用户纠正（`overrides.json`，A6）优先于识别结果；识别结果按文本 hash 缓存并在首次运行时并行预识别。

## 影响

- 受影响规范：language-detection（新增）
- 受影响代码：`core/langdetect`、`third_party/lingua`（新增）、`core/workflow/{map_update,translate,mapstate}.go`、`browse.go`
- 假设与限制：
  - 只含汉字的日文会被判为中文（与 lingua 一致），日文新字体与繁体字形重合时可能判为 zh-Hant；用户可在桌面端纠正。
  - 已有项目中先前存在源语言键下的非源语言注释保持原样（不搬家），新注释按新规则存放。
  - 翻译请求中的源语言名使用规范码（例如 zh-Hans），而非配置写法。

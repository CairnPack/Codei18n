# third_party/lingua —— 裁剪版 lingua-go

导入路径：`github.com/CairnPack/codei18n/third_party/lingua`，供 `core/langdetect` 识别注释语言使用。

## 来源

| 项目 | 值 |
|---|---|
| 上游 | https://github.com/pemistahl/lingua-go |
| 版本 | v1.4.0（2023-09-05） |
| 提交 | `320c87b00cfb62b6f6cee032f43caa9610b1d18b` |
| 许可证 | Apache-2.0（`LICENSE` 原样保留，修改说明见 `NOTICE`） |

## 为什么不直接依赖上游

上游用 `//go:embed language-models` 把 **75 种语言、约 123 MB** 的模型全部嵌入二进制，
与实际选用哪些语言无关（embed 不会被死代码消除），会让 codei18n 增大约 130 MB。

## 相对上游的改动

1. **模型裁剪**：`language-models/` 只保留 en、zh、ja、ko、de、fr、es、ru；
   拉丁/西里尔语言只保留 unigram ~ quadrigram，**删除 fivegram**（约占这些模型体积的 60%）。
   - 中日韩只有 unigram 模型（上游即如此，实际判定主要靠文字系统规则）。
   - 删除 fivegram 后，高精度模式中 5-gram 一路会按上游原有的回退逻辑落到 4/3/2/1-gram，代码无需改动。
     在 141 条 3~10 词的英/德/法/西注释语料上，与完整模型的识别结果完全一致；
     若再删除 quadrigram 则准确率明显下降（95/96 → 90/96），故保留。
   - 效果：模型 12.4 MB → 4.8 MB；8 种语言预加载耗时约 108 ms → 26 ms，常驻堆约 108 MB → 28 MB。
   - 低精度模式（只用 trigram）在短注释上会给出高置信度的错误结果（如 "TODO fix this" → 西班牙语 0.92），
     不适合代码注释，因此未采用"只保留 trigram"的更激进裁剪。
2. **去掉全部第三方依赖**（不修改 go.mod）：
   - `google.golang.org/protobuf` → `model_decode.go` 手写解码器（模型格式见文件内注释）；
   - `golang.org/x/exp/{maps,slices}` → 标准库 `maps`/`slices`；
   - `github.com/shopspring/decimal` → `float64` log-sum-exp（数学上等价且不会下溢）。
3. **删除**：`writer.go`（训练模型）、`model.go` 中的训练部分、`serialization/`、上游测试/示例/文档图片。
   `codei18n_test.go` 是新增的测试，覆盖上述改动点。
4. **小改动**：未内嵌的模型缓存为空表（避免反复读取 embed.FS）；新增 `AvailableLanguages()`。

`Language` 枚举仍包含全部 75 种语言，但只有 `AvailableLanguages()` 返回的语言有模型，
构建检测器时只应使用这些语言。纯 Go 实现，无需 cgo。

## 如何新增一种语言

1. 从上游 v1.4.0 的 `language-models/<iso639-1>/` 复制该语言的模型文件到本目录同名子目录
   （拉丁/西里尔等语言复制 unigrams ~ quadrigrams 四个 `.pb.bin.zip`；如需更高精度可同时复制 fivegrams）。
2. 运行 `go test ./third_party/lingua/`，并更新 `codei18n_test.go` 中 `TestAvailableLanguages` 的期望。
3. 在 `core/langdetect/detect.go` 的 `supportedCodes`、`codeToLingua`、`linguaToCode`
   中登记语言码，补充测试样本。
4. 重新测量二进制体积并更新本文档。每种拉丁语言约增加 0.8~1.2 MB。

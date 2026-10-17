# tree-sitter-dart 语法（vendored）

本目录是 Dart 语法的 cgo 绑定，供 `adapters/dart` 使用。
`github.com/smacker/go-tree-sitter` 没有自带 Dart 语法，所以这里按 smacker 各语言子包（如 `kotlin/`、`csharp/`）的方式平铺放置生成的 C 源码。

## 来源

| 项目 | 值 |
| --- | --- |
| 上游仓库 | https://github.com/UserNobody14/tree-sitter-dart |
| 提交 | `c8e7cbbd1589cc2ee1f9b5befa604dc7e953b0af`（2025-10-03，"Add support for extension type declarations (#83)"） |
| ABI | 14（`#define LANGUAGE_VERSION 14`，由 tree-sitter CLI v0.25.10 生成） |
| 许可证 | MIT，见同目录 `LICENSE`（上游原文，Copyright (c) 2020-2023 UserNobody14 and others） |

固定在这个提交的原因：smacker 运行时（`v0.0.0-20240827094217-dd81d9e9be82`）只接受 ABI 13 和 14。上游之后的提交都是 ABI 15，运行时会拒绝加载。
这个提交已支持 Dart 3 的 pattern、record、class 修饰符和 extension type。它不支持 dot shorthand（Dart 3.10）和 primary constructor（Dart 3.13）。这些语法会产生 ERROR 节点，但注释是 extras，仍能正常提取。

## 文件与改动

| 文件 | 来源 | 改动 |
| --- | --- | --- |
| `parser.c` | 上游 `src/parser.c`（sha256 `911fdf2f…a772f`） | 见下方 sed，共 3 处 |
| `scanner.c` | 上游 `src/scanner.c`（sha256 `07a7b781…84996`） | include 路径改为 `"parser.h"` |
| `parser.h` | **不是**上游文件。它是 smacker 模块中 `kotlin/parser.h` 的原样拷贝（与 smacker 运行时 `parser.h` 逐字节相同，sha256 `a3eb18ef…052aa`，来自 tree-sitter，MIT） | 无 |
| `binding.go` | 自写 | — |
| `LICENSE` | 上游 `LICENSE` | 无 |

使用 smacker 的 `parser.h`，是为了让 `TSLanguage` 等结构体布局与运行时完全一致。上游的 0.25 头文件只是在结构体末尾追加了字段，并改了几个名字，所以 `parser.c` 需要以下机械替换：

```sh
UP=https://raw.githubusercontent.com/UserNobody14/tree-sitter-dart/c8e7cbbd1589cc2ee1f9b5befa604dc7e953b0af
curl -sLO $UP/src/parser.c && curl -sLO $UP/src/scanner.c && curl -sLO $UP/LICENSE
# 1) include 路径  2) .abi_version → .version  3) TSMapSlice → TSFieldMapSlice
sed -i -e 's#"tree_sitter/parser.h"#"parser.h"#' \
       -e 's/\.abi_version = /.version = /' \
       -e 's/TSMapSlice/TSFieldMapSlice/g' parser.c
sed -i -e 's#"tree_sitter/parser.h"#"parser.h"#' scanner.c
cp "$(go env GOMODCACHE)/github.com/smacker/go-tree-sitter@v0.0.0-20240827094217-dd81d9e9be82/kotlin/parser.h" parser.h
```

`scanner.c` 不依赖 `array.h` 或 `alloc.h`，也不保存状态。导出的 C 符号只有 `tree_sitter_dart` 和 `tree_sitter_dart_external_scanner_*`，不会与 smacker 的其他语法冲突。

## 升级到更新的上游

上游 HEAD 是 ABI 15，不能直接拷贝。可以检出上游仓库，然后用 tree-sitter CLI（0.26/0.27 仍支持 `--abi 14`）运行 `tree-sitter generate --abi 14` 重新生成。生成后套用上面同样的头文件处理即可。

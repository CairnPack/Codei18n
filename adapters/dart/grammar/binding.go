// Package grammar 是 tree-sitter-dart 语法的 cgo 绑定，写法与 smacker/go-tree-sitter
// 自带的各语言子包（如 kotlin、csharp）一致：parser.c、scanner.c、parser.h 平铺在本目录，
// 由 cgo 自动编译。
//
// smacker 的运行时只支持 ABI ≤ 14，因此这里固定使用上游 ABI 14 的最后一个提交，
// 来源与改动见同目录 README.md，许可证见 LICENSE（MIT）。
package grammar

//#include "parser.h"
//TSLanguage *tree_sitter_dart();
import "C"
import (
	"unsafe"

	sitter "github.com/smacker/go-tree-sitter"
)

// GetLanguage 返回 Dart 语法。返回值指向 C 侧的静态数据，可在多个 goroutine 间共享。
func GetLanguage() *sitter.Language {
	ptr := unsafe.Pointer(C.tree_sitter_dart())
	return sitter.NewLanguage(ptr)
}

package adapters

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/CairnPack/codei18n/adapters/csharp"
	"github.com/CairnPack/codei18n/adapters/dart"
	"github.com/CairnPack/codei18n/adapters/golang"
	"github.com/CairnPack/codei18n/adapters/java"
	"github.com/CairnPack/codei18n/adapters/kotlin"
	"github.com/CairnPack/codei18n/adapters/rust"
	"github.com/CairnPack/codei18n/adapters/typescript"
	"github.com/CairnPack/codei18n/core"
)

// languageIDs 把扩展名映射到 index.json 中的编程语言标识。
var languageIDs = map[string]string{
	".go":     "go",
	".rs":     "rust",
	".js":     "javascript",
	".mjs":    "javascript",
	".cjs":    "javascript",
	".jsx":    "jsx",
	".ts":     "typescript",
	".mts":    "typescript",
	".cts":    "typescript",
	".tsx":    "tsx",
	".java":   "java",
	".kt":     "kotlin",
	".kts":    "kotlin",
	".cs":     "csharp",
	".dart":   "dart",
	".groovy": "groovy",
	".scala":  "scala",
}

// LanguageID 返回文件对应的编程语言标识；不支持时返回空串。
func LanguageID(filename string) string {
	return languageIDs[strings.ToLower(filepath.Ext(filename))]
}

// Supported 判断文件扩展名是否受支持。
func Supported(filename string) bool { return LanguageID(filename) != "" }

// GetAdapter 返回文件对应的 LanguageAdapter。每次调用返回新实例：
// tree-sitter 的 Parser 不是并发安全的，调用方不要在多个 goroutine 间共享同一个 adapter。
func GetAdapter(filename string) (core.LanguageAdapter, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".go":
		return golang.NewAdapter(), nil
	case ".rs":
		return rust.NewRustAdapter(), nil
	case ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts":
		return typescript.NewAdapter(), nil
	case ".java":
		return java.NewAdapter("java"), nil
	case ".kt", ".kts":
		return kotlin.NewAdapter(), nil
	case ".cs":
		return csharp.NewAdapter(), nil
	case ".dart":
		return dart.NewAdapter(), nil
	case ".groovy":
		return java.NewAdapter("groovy"), nil
	case ".scala":
		return java.NewAdapter("scala"), nil
	default:
		return nil, fmt.Errorf("unsupported file extension: %s", ext)
	}
}

package adapters

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/CairnPack/codei18n/adapters/golang"
	"github.com/CairnPack/codei18n/adapters/java"
	"github.com/CairnPack/codei18n/adapters/rust"
	"github.com/CairnPack/codei18n/adapters/typescript"
	"github.com/CairnPack/codei18n/core"
)

// GetAdapter returns the appropriate LanguageAdapter for the given file
func GetAdapter(filename string) (core.LanguageAdapter, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".go":
		return golang.NewAdapter(), nil
	case ".rs":
		return rust.NewRustAdapter(), nil
	case ".js", ".jsx", ".ts", ".tsx":
		return typescript.NewAdapter(), nil
	case ".java":
		return java.NewAdapter("java"), nil
	case ".kt":
		return java.NewAdapter("kotlin"), nil
	case ".groovy":
		return java.NewAdapter("groovy"), nil
	case ".scala":
		return java.NewAdapter("scala"), nil
	default:
		return nil, fmt.Errorf("unsupported file extension: %s", ext)
	}
}

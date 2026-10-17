package codei18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// 库代码（除 cmd/ 与 internal/log 外的所有非测试 Go 文件）不得打印、不得退出进程、不得使用终端 UI 依赖。
// spinner 与彩色输出只允许出现在 cmd/。
func TestLibraryCodeDoesNotPrintOrExit(t *testing.T) {
	forbiddenImports := map[string]bool{
		"github.com/briandowns/spinner":                true,
		"github.com/fatih/color":                       true,
		"github.com/spf13/viper":                       true,
		"github.com/CairnPack/codei18n/internal/log": true,
		"log": true,
	}
	forbiddenCalls := map[string]bool{
		"os.Exit": true, "fmt.Print": true, "fmt.Println": true, "fmt.Printf": true,
		"os.Stdout": true, "os.Stderr": true, "log.Fatal": true, "log.Fatalf": true,
	}
	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		slash := filepath.ToSlash(path)
		if d.IsDir() {
			switch slash {
			case "cmd", "internal/log", "tests", "specs", ".git", "openspec", "docs":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if forbiddenImports[p] {
				t.Errorf("%s 导入了 %s", slash, p)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && forbiddenCalls[id.Name+"."+sel.Sel.Name] {
				t.Errorf("%s 使用了 %s.%s", fset.Position(sel.Pos()), id.Name, sel.Sel.Name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

package golang

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"

	"github.com/CairnPack/codei18n/adapters/commentgroup"
	"github.com/CairnPack/codei18n/core/domain"
)

// goOptions 是 Go 的注释类型规则（docs/index-format.md §6.3）：Go 没有专门的文档注释标记，
// `//` 为 line，`/* */` 为 block。
var goOptions = commentgroup.Options{Language: "go"}

// Adapter 实现 Go 语言的 core.LanguageAdapter。
type Adapter struct{}

// NewAdapter 创建 Go 语言 adapter。
func NewAdapter() *Adapter {
	return &Adapter{}
}

// Language 返回语言标识 "go"。
func (a *Adapter) Language() string {
	return "go"
}

// Parse 解析 Go 源码并提取注释。src 为 nil 时从 file 读取。
// 每条物理注释（一个 // 行注释或一个 /* */ 块注释）先收集为 commentgroup.Raw，
// 再由 commentgroup.Build 按统一规则合并连续的独占行 // 注释（§6.1）；/* */ 永远单独成条。
func (a *Adapter) Parse(file string, src []byte) ([]*domain.Comment, error) {
	if src == nil {
		// 独占行判断需要源码字节，因此这里自行读取，而不是交给 go/parser
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		src = data
	}

	fset := token.NewFileSet()
	// 必须带 ParseComments 才能在 AST 中拿到注释
	f, err := parser.ParseFile(fset, file, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	return commentgroup.Build(src, file, collectRaws(fset, f), goOptions), nil
}

// collectRaws 按源码顺序收集所有物理注释及其符号。
// 符号解析规则与合并改造前完全一致：同一个 ast.CommentGroup 内的注释共享该组第一条注释
// 经 ast.CommentMap 关联到的节点符号，找不到时归属包级。这样单条注释的符号（进而 ID）不变。
func collectRaws(fset *token.FileSet, f *ast.File) []commentgroup.Raw {
	commentSymbols := commentSymbolMap(fset, f)

	var raws []commentgroup.Raw
	for _, cg := range f.Comments {
		if len(cg.List) == 0 {
			continue
		}

		// 使用组内第一条注释的符号
		symbol := commentSymbols[cg.List[0]]
		if symbol == "" {
			symbol = "package." + f.Name.Name // 默认归属包级
		}

		for _, c := range cg.List {
			pos := fset.Position(c.Pos())
			end := fset.Position(c.End())
			raws = append(raws, commentgroup.Raw{
				Text:      c.Text,
				StartLine: pos.Line,
				StartCol:  pos.Column,
				EndLine:   end.Line,
				EndCol:    end.Column,
				Symbol:    symbol,
			})
		}
	}
	return raws
}

// commentSymbolMap 借助 ast.NewCommentMap 把每条注释映射到其关联节点的符号。
func commentSymbolMap(fset *token.FileSet, f *ast.File) map[*ast.Comment]string {
	cmap := ast.NewCommentMap(fset, f, f.Comments)

	commentSymbols := make(map[*ast.Comment]string)
	for node, groups := range cmap {
		symbol := nodeSymbol(node)
		if symbol == "unknown" {
			continue // 保持默认（包级）
		}
		for _, cg := range groups {
			for _, c := range cg.List {
				commentSymbols[c] = symbol
			}
		}
	}
	return commentSymbols
}

// nodeSymbol 根据 AST 节点得出符号名。
func nodeSymbol(n ast.Node) string {
	switch t := n.(type) {
	case *ast.File:
		return "package." + t.Name.Name
	case *ast.FuncDecl:
		return "func." + t.Name.Name
	case *ast.GenDecl:
		// import、const、type、var
		return "decl." + t.Tok.String()
	case *ast.TypeSpec:
		return "type." + t.Name.Name
	case *ast.ValueSpec:
		if len(t.Names) > 0 {
			return "var." + t.Names[0].Name
		}
	}
	return "unknown"
}

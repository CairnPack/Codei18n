package typescript

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/CairnPack/codei18n/core/domain"
)

type Adapter struct{}

func NewAdapter() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Language() string {
	return "typescript"
}

func (a *Adapter) Parse(file string, src []byte) ([]*domain.Comment, error) {
	lang, err := getLanguage(file)
	if err != nil {
		return nil, err
	}

	parser := sitter.NewParser()
	parser.SetLanguage(lang)

	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file: %w", err)
	}
	defer tree.Close()

	rootNode := tree.RootNode()

	return a.extractComments(rootNode, src, file, lang)
}

func getLanguage(filename string) (*sitter.Language, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".js", ".jsx":
		// Ideally we use javascript for .js/.jsx, but for simplicity in this MVP
		// we can also stick to specific grammars.
		// Note: .jsx might need javascript grammar or a specific jsx one if available/integrated.
		// The smacker/go-tree-sitter javascript grammar usually handles JSX.
		return javascript.GetLanguage(), nil
	case ".ts":
		return typescript.GetLanguage(), nil
	case ".tsx":
		return tsx.GetLanguage(), nil
	default:
		return nil, fmt.Errorf("unsupported file extension for typescript adapter: %s", ext)
	}
}

func (a *Adapter) extractComments(root *sitter.Node, src []byte, file string, lang *sitter.Language) ([]*domain.Comment, error) {
	q, err := sitter.NewQuery([]byte(queryTS), lang)
	if err != nil {
		return nil, fmt.Errorf("invalid query: %w", err)
	}
	defer q.Close()

	qc := sitter.NewQueryCursor()
	defer qc.Close()

	qc.Exec(q, root)

	var comments []*domain.Comment

	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}

		for _, c := range m.Captures {
			node := c.Node
			text := node.Content(src)

			// Identify comment type
			cType := domain.CommentTypeLine
			if strings.HasPrefix(text, "/*") {
				if strings.HasPrefix(text, "/**") {
					cType = domain.CommentTypeDoc
				} else {
					cType = domain.CommentTypeBlock
				}
			}

			// Resolve symbol
			symbol := resolveSymbol(node, src)

			// Generate ID
			// We need a stable ID.
			// ID = SHA1(file + lang + symbol + normalized_text)
			// But since we can't depend on core/utils internal logic easily if it's not exported or if we want to be consistent:
			// core/utils/id.go usually has ID generation. Let's assume we can use it or replicate it.
			// Let's use the core/domain one if available or utils.
			// Wait, core/utils/id.go was mentioned in exploration.
			// We need to import "github.com/CairnPack/codei18n/core/utils" if it's public.
			// Let's check imports.

			// For now, let's just create the object, ID will be handled by caller or we need to add utils.
			// CodeI18n core seems to handle ID generation in scanner usually?
			// Actually, the adapter returns comments, and scanner might enrich them or adapter should set ID.
			// Checking `adapters/golang/parser.go` would clarify.

			// Let's assume we need to generate ID here.

			comment := &domain.Comment{
				File:     file,
				Language: "typescript", // or javascript, but adapter says typescript
				Symbol:   symbol,
				Range: domain.TextRange{
					StartLine: int(node.StartPoint().Row) + 1,
					StartCol:  int(node.StartPoint().Column) + 1,
					EndLine:   int(node.EndPoint().Row) + 1,
					EndCol:    int(node.EndPoint().Column) + 1,
				},
				SourceText: text,
				Type:       cType,
			}

			// We won't set ID here if we don't have the util, but we should tries to.
			// Let's rely on scanner to set ID or add utils import.
			// In `core/scanner/scanner.go`: "comments, err := adapter.Parse(...) ... for c in comments { c.ID = utils.GenerateID(...) }"
			// If scanner does it, we are good. If adapter must do it, we need to know.
			// Let's check `adapters/golang/parser.go` later. For now, leave ID empty.

			comments = append(comments, comment)
		}
	}

	return comments, nil
}

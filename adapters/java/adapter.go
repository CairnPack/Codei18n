package java

import (
	"context"
	"os"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/java"

	"github.com/CairnPack/codei18n/core/domain"
)

// Adapter implements the LanguageAdapter interface in Java.
// Use Tree-sitter to parse Java source code and extract comments and their context
type Adapter struct {
	parser   *sitter.Parser
	language string
}

// NewAdapter creates a new Java adapter instance
// Initialize the Tree-sitter parser and load the Java grammar
func NewAdapter(lang ...string) *Adapter {
	p := sitter.NewParser()
	p.SetLanguage(java.GetLanguage())
	l := "java"
	if len(lang) > 0 {
		l = lang[0]
	}
	return &Adapter{parser: p, language: l}
}

// Language returns the language identifier ("java")
func (a *Adapter) Language() string {
	return a.language
}

// Parse the provided Java source code and extract comments
// Returns a list of domain.Comment objects containing the comment text, location, and context symbols.
// If src is nil, read the source code from the file path.
func (a *Adapter) Parse(file string, src []byte) ([]*domain.Comment, error) {
	if src == nil {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		src = data
	}

	// Parse the source code using Tree-sitter
	tree, err := a.parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	return a.extractComments(tree.RootNode(), src, file)
}

// extractComments extracts comments from the parse tree
func (a *Adapter) extractComments(root *sitter.Node, src []byte, file string) ([]*domain.Comment, error) {
	// First, extract the package name
	packageName := extractPackageName(root, src)

	// Create query
	q, err := sitter.NewQuery([]byte(javaCommentQuery), java.GetLanguage())
	if err != nil {
		return nil, err
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

			// Identify comment types
			cType := domain.CommentTypeLine

			if strings.HasPrefix(text, "/*") {
				if strings.HasPrefix(text, "/**") {
					cType = domain.CommentTypeDoc
				} else {
					cType = domain.CommentTypeBlock
				}
			}

			// Parse the symbol path
			symbol := resolveSymbol(node, src, packageName)

			comment := &domain.Comment{
				File:     file,
				Language: a.language,
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

			comments = append(comments, comment)
		}
	}

	return comments, nil
}

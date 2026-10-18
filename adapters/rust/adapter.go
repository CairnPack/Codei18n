package rust

import (
	"context"
	"os"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/rust"

	"github.com/CairnPack/codei18n/core/domain"
)

// RustAdapter implements the LanguageAdapter interface for Rust language.
// It uses Tree-sitter to parse Rust source code and extract comments with their context.
type RustAdapter struct {
	parser *sitter.Parser
}

// NewRustAdapter creates a new instance of RustAdapter.
// It initializes the Tree-sitter parser with the Rust grammar.
func NewRustAdapter() *RustAdapter {
	p := sitter.NewParser()
	p.SetLanguage(rust.GetLanguage())
	return &RustAdapter{parser: p}
}

// Language returns the language identifier ("rust").
func (a *RustAdapter) Language() string {
	return "rust"
}

// Parse parses the provided Rust source code and extracts comments.
// It returns a list of domain.Comment structs containing comment text, position, and context symbol.
// If src is nil, it currently expects the caller to handle file reading (TODO: implement file reading).
func (a *RustAdapter) Parse(file string, src []byte) ([]*domain.Comment, error) {
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

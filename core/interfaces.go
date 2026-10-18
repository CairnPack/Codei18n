package core

import (
	"context"

	"github.com/CairnPack/codei18n/core/domain"
)

// LanguageAdapter defines the interface for language-specific AST parsers
type LanguageAdapter interface {
	// Language returns the language identifier (e.g., "go")
	Language() string

	// Parse parses the source code and extracts comments
	// file: the file path (used for ID generation context)
	// src: the source code content (if nil, read from file)
	Parse(file string, src []byte) ([]*domain.Comment, error)
}

// Translator defines the interface for translation services
type Translator interface {
	// Translate translates the text from source language to target language
	Translate(ctx context.Context, text, from, to string) (string, error)

	// TranslateBatch translates a batch of texts (optional optimization)
	TranslateBatch(ctx context.Context, texts []string, from, to string) ([]string, error)
}

package utils

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/CairnPack/codei18n/core/domain"
)

// GenerateCommentID calculates a stable ID for a comment
// Rule: SHA1(file_path + language + parent_symbol + normalized_text)
func GenerateCommentID(c *domain.Comment) string {
	normalizedText := NormalizeCommentText(c.SourceText)

	// Create the content to hash
	// Separator | is used to avoid collisions
	content := fmt.Sprintf(
		"%s|%s|%s|%s",
		c.File,
		c.Language,
		c.Symbol,
		normalizedText,
	)

	hasher := sha1.New()
	hasher.Write([]byte(content))
	return hex.EncodeToString(hasher.Sum(nil))
}

// NormalizeCommentText removes comment markers and whitespace to ensure stability
func NormalizeCommentText(text string) string {
	t := strings.TrimSpace(text)

	// Remove single line markers
	t = strings.TrimPrefix(t, "//")

	// Remove block markers
	t = strings.TrimPrefix(t, "/*")
	t = strings.TrimSuffix(t, "*/")

	// Normalize whitespace: replace sequences of whitespace with single space
	return strings.Join(strings.Fields(t), " ")
}

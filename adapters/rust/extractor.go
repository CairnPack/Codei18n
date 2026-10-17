package rust

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/rust"

	"github.com/CairnPack/codei18n/core/domain"
	"github.com/CairnPack/codei18n/core/utils"
)

// extractComments processes the AST and extracts all comments matching the query.
func (a *RustAdapter) extractComments(root *sitter.Node, src []byte, file string) ([]*domain.Comment, error) {
	q, err := sitter.NewQuery([]byte(rustCommentQuery), rust.GetLanguage())
	if err != nil {
		return nil, err
	}

	qc := sitter.NewQueryCursor()
	qc.Exec(q, root)
	defer qc.Close() // Ensure cursor is closed

	var comments []*domain.Comment

	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}

		for _, c := range m.Captures {
			node := c.Node
			content := node.Content(src)

			if isEmptyComment(content) {
				continue
			}

			// Find Owner and Symbol Path
			owner := FindOwnerNode(node, src)
			symbolPath := ResolveSymbolPath(owner, src)

			// Calculate Range (Trimming trailing newline if present for Line/Doc comments)
			startRow := int(node.StartPoint().Row) + 1
			startCol := int(node.StartPoint().Column) + 1
			endRow := int(node.EndPoint().Row) + 1
			endCol := int(node.EndPoint().Column) + 1

			// Fix for Tree-sitter including newline in Line/Doc comments
			// If it's a line/doc comment and includes newline (EndCol == 1 && EndRow > StartRow)
			// We want to snap it back to the same line.
			cType := getDomainCommentType(content)
			if (cType == domain.CommentTypeLine || cType == domain.CommentTypeDoc) &&
				strings.HasSuffix(content, "\n") {
				// Trim the newline from content
				content = strings.TrimSuffix(content, "\n")
				// Recalculate EndRow/EndCol based on trimmed content
				// Since it's a single line comment, EndRow is StartRow
				endRow = startRow
				// EndCol is StartCol + length (in bytes)
				endCol = startCol + len(content)
			}

			comment := &domain.Comment{
				SourceText: content,
				Symbol:     symbolPath,
				File:       file,
				Language:   "rust",
				Range: domain.TextRange{
					StartLine: startRow,
					StartCol:  startCol,
					EndLine:   endRow,
					EndCol:    endCol,
				},
				Type: cType,
			}
			comment.ID = utils.GenerateCommentID(comment)
			comments = append(comments, comment)
		}
	}

	return comments, nil
}

// getDomainCommentType maps raw comment content to domain.CommentType
func getDomainCommentType(content string) domain.CommentType {
	if strings.HasPrefix(content, "/*") {
		return domain.CommentTypeBlock
	}
	if strings.HasPrefix(content, "///") || strings.HasPrefix(content, "//!") {
		return domain.CommentTypeDoc
	}
	return domain.CommentTypeLine
}

func isEmptyComment(content string) bool {
	trimmed := strings.TrimSpace(content)
	switch {
	case strings.HasPrefix(trimmed, "///"):
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "///")) == ""
	case strings.HasPrefix(trimmed, "//!"):
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "//!")) == ""
	case strings.HasPrefix(trimmed, "//"):
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "//")) == ""
	case strings.HasPrefix(trimmed, "/*"):
		inner := strings.TrimPrefix(trimmed, "/*")
		inner = strings.TrimSuffix(inner, "*/")
		return strings.TrimSpace(inner) == ""
	default:
		return strings.TrimSpace(trimmed) == ""
	}
}

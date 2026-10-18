package typescript

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/CairnPack/codei18n/core/domain"
)

func TestAdapter_Parse_TypeScript(t *testing.T) {
	src := `
// Top level comment
const x = 1;

/**
 * A calculator class
 */
class Calculator {
	// Adds two numbers
	add(a: number, b: number): number {
		return a + b;
	}
}

// User interface
interface User {
	name: string;
}
`
	adapter := NewAdapter()
	comments, err := adapter.Parse("test.ts", []byte(src))
	assert.NoError(t, err)
	assert.Len(t, comments, 4)

	// Verify comments
	assert.Equal(t, "// Top level comment", comments[0].SourceText)
	// Symbol for top level might be x if it binds to next, or empty if too far or logic differs
	// Our logic binds to next named sibling. "const x = 1" is a lexical_declaration.
	// lexical_declaration contains variable_declarator.
	// In symbol.go: lexical_declaration -> variable_declarator -> name "x"
	assert.Equal(t, "x", comments[0].Symbol)
	assert.Equal(t, domain.CommentTypeLine, comments[0].Type)

	assert.Equal(t, "/**\n * A calculator class\n */", comments[1].SourceText)
	assert.Equal(t, "Calculator", comments[1].Symbol)
	assert.Equal(t, domain.CommentTypeDoc, comments[1].Type)

	assert.Equal(t, "// Adds two numbers", comments[2].SourceText)
	assert.Equal(t, "Calculator.add", comments[2].Symbol)
	assert.Equal(t, domain.CommentTypeLine, comments[2].Type)

	assert.Equal(t, "// User interface", comments[3].SourceText)
	assert.Equal(t, "User", comments[3].Symbol)
}

func TestAdapter_Parse_JavaScript(t *testing.T) {
	src := `
// Hello world
function hello() {
	console.log("hello");
}
`
	adapter := NewAdapter()
	comments, err := adapter.Parse("test.js", []byte(src))
	assert.NoError(t, err)
	assert.Len(t, comments, 1)

	assert.Equal(t, "// Hello world", comments[0].SourceText)
	assert.Equal(t, "hello", comments[0].Symbol)
}

func TestAdapter_Parse_JSX(t *testing.T) {
	src := `
// My Component
const MyComp = () => {
	return <div></div>
}
`
	adapter := NewAdapter()
	comments, err := adapter.Parse("test.jsx", []byte(src))
	assert.NoError(t, err)
	assert.Len(t, comments, 1)

	assert.Equal(t, "// My Component", comments[0].SourceText)
	assert.Equal(t, "MyComp", comments[0].Symbol)
}

// TestAdapter_Parse_Range: Check if the position information (StartCol and EndCol) is correctly set in the comment location.
func TestAdapter_Parse_Range(t *testing.T) {
	src := `export class Decorator {
    // 创建一个装饰类型，将原始注释隐藏并在前面显示翻译
    private translationDecorationType = vscode.window.createTextEditorDecorationType({
        // 让原始文本不可见且不占空间
        opacity: '0',
        letterSpacing: '-1000px', // 将字母间距设为极大的负值，压缩文本到几乎不可见
    });
}`
	adapter := NewAdapter()
	comments, err := adapter.Parse("test.ts", []byte(src))
	assert.NoError(t, err)
	assert.Greater(t, len(comments), 0, "应该至少有一个注释")

	// Validate that each comment has the correct Range information
	for i, comment := range comments {
		assert.Greater(t, comment.Range.StartLine, 0, "注释 %d 的 StartLine 应该大于 0", i)
		assert.Greater(t, comment.Range.StartCol, 0, "注释 %d 的 StartCol 应该大于 0", i)
		assert.Greater(t, comment.Range.EndLine, 0, "注释 %d 的 EndLine 应该大于 0", i)
		assert.Greater(t, comment.Range.EndCol, 0, "注释 %d 的 EndCol 应该大于 0", i)
		assert.GreaterOrEqual(t, comment.Range.EndLine, comment.Range.StartLine, "注释 %d 的 EndLine 应该 >= StartLine", i)
		if comment.Range.EndLine == comment.Range.StartLine {
			assert.Greater(t, comment.Range.EndCol, comment.Range.StartCol, "注释 %d 在同一行时 EndCol 应该 > StartCol", i)
		}
	}

	// Validate the specific position of the first comment
	// "// Create a decorator type to hide the original comment and display the translation in front of it" should be on the second line
	firstComment := comments[0]
	assert.Equal(t, 2, firstComment.Range.StartLine, "第一个注释应该在第 2 行")
	assert.Equal(t, 2, firstComment.Range.EndLine, "第一个注释应该在第 2 行")
	assert.Equal(t, 5, firstComment.Range.StartCol, "第一个注释应该从第 5 列开始（4个空格 + 1）")
	// EndCol should be the position of the comment's end.
	assert.Greater(t, firstComment.Range.EndCol, firstComment.Range.StartCol, "EndCol 应该大于 StartCol")
}

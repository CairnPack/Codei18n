package java

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/CairnPack/codei18n/core/domain"
)

func TestAdapter_Parse_Class(t *testing.T) {
	src := `package com.example;

// 计算器类
public class Calculator {
}
`
	adapter := NewAdapter()
	comments, err := adapter.Parse("test.java", []byte(src))
	assert.NoError(t, err)
	assert.Len(t, comments, 1)

	assert.Equal(t, "// 计算器类", comments[0].SourceText)
	assert.Equal(t, "com.example.Calculator", comments[0].Symbol)
	assert.Equal(t, domain.CommentTypeLine, comments[0].Type)
}

func TestAdapter_Parse_Method(t *testing.T) {
	src := `package com.example;

public class Calculator {
    // 计算两个数的和
    public int add(int a, int b) {
        return a + b;
    }
}
`
	adapter := NewAdapter()
	comments, err := adapter.Parse("test.java", []byte(src))
	assert.NoError(t, err)
	assert.Len(t, comments, 1)

	assert.Equal(t, "// 计算两个数的和", comments[0].SourceText)
	assert.Equal(t, "com.example.Calculator#add", comments[0].Symbol)
	assert.Equal(t, domain.CommentTypeLine, comments[0].Type)
}

func TestAdapter_Parse_Field(t *testing.T) {
	src := `package com.example;

public class Config {
    // 默认超时时间（毫秒）
    private int timeoutMs = 1000;
}
`
	adapter := NewAdapter()
	comments, err := adapter.Parse("test.java", []byte(src))
	assert.NoError(t, err)
	assert.Len(t, comments, 1)

	assert.Equal(t, "// 默认超时时间（毫秒）", comments[0].SourceText)
	assert.Equal(t, "com.example.Config#timeoutMs", comments[0].Symbol)
	assert.Equal(t, domain.CommentTypeLine, comments[0].Type)
}

func TestAdapter_Parse_Interface(t *testing.T) {
	src := `package com.example;

// 用户接口
public interface User {
    String getName();
}
`
	adapter := NewAdapter()
	comments, err := adapter.Parse("test.java", []byte(src))
	assert.NoError(t, err)
	assert.Len(t, comments, 1)

	assert.Equal(t, "// 用户接口", comments[0].SourceText)
	assert.Equal(t, "com.example.User", comments[0].Symbol)
	assert.Equal(t, domain.CommentTypeLine, comments[0].Type)
}

func TestAdapter_Parse_Enum(t *testing.T) {
	src := `package com.example;

// 账户类型
public enum AccountType {
    STANDARD,
    PREMIUM
}
`
	adapter := NewAdapter()
	comments, err := adapter.Parse("test.java", []byte(src))
	assert.NoError(t, err)
	assert.Len(t, comments, 1)

	assert.Equal(t, "// 账户类型", comments[0].SourceText)
	assert.Equal(t, "com.example.AccountType", comments[0].Symbol)
	assert.Equal(t, domain.CommentTypeLine, comments[0].Type)
}

func TestAdapter_Parse_BlockComment(t *testing.T) {
	src := `package com.example;

/* 这是一个块注释 */
public class Test {
}
`
	adapter := NewAdapter()
	comments, err := adapter.Parse("test.java", []byte(src))
	assert.NoError(t, err)
	assert.Len(t, comments, 1)

	assert.Equal(t, "/* 这是一个块注释 */", comments[0].SourceText)
	assert.Equal(t, "com.example.Test", comments[0].Symbol)
	assert.Equal(t, domain.CommentTypeBlock, comments[0].Type)
}

func TestAdapter_Parse_Javadoc(t *testing.T) {
	src := `package com.example;

/**
 * 计算器类
 * 提供基本的数学运算
 */
public class Calculator {
    /**
     * 计算两个数的和
     * @param a 第一个数
     * @param b 第二个数
     * @return 两数之和
     */
    public int add(int a, int b) {
        return a + b;
    }
}
`
	adapter := NewAdapter()
	comments, err := adapter.Parse("test.java", []byte(src))
	assert.NoError(t, err)
	assert.Len(t, comments, 2)

	// Class Javadoc
	assert.Contains(t, comments[0].SourceText, "/**")
	assert.Equal(t, "com.example.Calculator", comments[0].Symbol)
	assert.Equal(t, domain.CommentTypeDoc, comments[0].Type)

	// Method Javadoc
	assert.Contains(t, comments[1].SourceText, "/**")
	assert.Equal(t, "com.example.Calculator#add", comments[1].Symbol)
	assert.Equal(t, domain.CommentTypeDoc, comments[1].Type)
}

func TestAdapter_Parse_NoPackage(t *testing.T) {
	src := `
// 简单类
public class Simple {
}
`
	adapter := NewAdapter()
	comments, err := adapter.Parse("test.java", []byte(src))
	assert.NoError(t, err)
	assert.Len(t, comments, 1)

	assert.Equal(t, "// 简单类", comments[0].SourceText)
	assert.Equal(t, "Simple", comments[0].Symbol)
	assert.Equal(t, domain.CommentTypeLine, comments[0].Type)
}

func TestAdapter_Parse_FileLevel(t *testing.T) {
	src := `// Copyright (c) 2025 Example Corp
// 本文件包含示例代码
package com.example;

public class Test {
}
`
	adapter := NewAdapter()
	comments, err := adapter.Parse("test.java", []byte(src))
	assert.NoError(t, err)
	// 文件头两行 // 注释按连续行注释合并规则（docs/index-format.md §6.1）合并为一条
	assert.Len(t, comments, 1)
	assert.Equal(t, 2, comments[0].Parts)
	assert.Equal(t, "// Copyright (c) 2025 Example Corp\n// 本文件包含示例代码", comments[0].SourceText)
}

func TestAdapter_Parse_Range(t *testing.T) {
	src := `package com.example;

public class Test {
    // 测试方法
    public void test() {
    }
}
`
	adapter := NewAdapter()
	comments, err := adapter.Parse("test.java", []byte(src))
	assert.NoError(t, err)
	assert.Len(t, comments, 1)

	// Validate location information
	comment := comments[0]
	assert.Greater(t, comment.Range.StartLine, 0)
	assert.Greater(t, comment.Range.StartCol, 0)
	assert.Greater(t, comment.Range.EndLine, 0)
	assert.Greater(t, comment.Range.EndCol, 0)
	assert.GreaterOrEqual(t, comment.Range.EndLine, comment.Range.StartLine)
	if comment.Range.EndLine == comment.Range.StartLine {
		assert.Greater(t, comment.Range.EndCol, comment.Range.StartCol)
	}
}

func TestAdapter_Parse_SyntaxError(t *testing.T) {
	src := `package com.example;

public class Broken {
    // 未完成的方法定义
    public void doSomething( {
}
`
	adapter := NewAdapter()
	comments, err := adapter.Parse("test.java", []byte(src))
	// Tree-sitter should be able to handle errors, at least returning comments.
	assert.NoError(t, err)
	assert.Greater(t, len(comments), 0)

	// Validate that at least one comment can be extracted.
	found := false
	for _, c := range comments {
		if c.SourceText == "// 未完成的方法定义" {
			found = true
			break
		}
	}
	assert.True(t, found, "应该能提取到注释，即使代码有语法错误")
}

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/CairnPack/codei18n/adapters"
	"github.com/CairnPack/codei18n/core/domain"
)

func TestJavaAdapter_Integration(t *testing.T) {
	adapter, err := adapters.GetAdapter("sample.java")
	assert.NoError(t, err)
	assert.NotNil(t, adapter)
	assert.Equal(t, "java", adapter.Language())

	// Read the test file
	src := []byte(`package com.example;

// Calculator class
public class Calculator {
    // Add two numbers
    public int add(int a, int b) {
        return a + b;
    }
}
`)

	comments, err := adapter.Parse("test.java", src)
	assert.NoError(t, err)
	assert.Greater(t, len(comments), 0, "应该至少提取到一个注释")

	// Validate the comment content
	var foundClass, foundMethod bool
	for _, c := range comments {
		if c.SourceText == "// Calculator class" {
			foundClass = true
			assert.Equal(t, "com.example.Calculator", c.Symbol)
			assert.Equal(t, domain.CommentTypeLine, c.Type)
		}
		if c.SourceText == "// Add two numbers" {
			foundMethod = true
			assert.Equal(t, "com.example.Calculator#add", c.Symbol)
			assert.Equal(t, domain.CommentTypeLine, c.Type)
		}
	}

	assert.True(t, foundClass, "应该找到类注释")
	assert.True(t, foundMethod, "应该找到方法注释")
}

func TestJavaAdapter_RealFile(t *testing.T) {
	adapter, err := adapters.GetAdapter("sample.java")
	assert.NoError(t, err)

	// Parse the actual test file
	comments, err := adapter.Parse("testdata/sample.java", nil)
	assert.NoError(t, err)
	assert.Greater(t, len(comments), 0, "应该从实际文件中提取到注释")

	// Verify that the package name has been correctly extracted
	for _, c := range comments {
		if c.Symbol != "" {
			assert.Contains(t, c.Symbol, "com.example.demo", "符号路径应该包含包名")
		}
	}
}

func TestJavaAdapter_CommentTypes(t *testing.T) {
	src := []byte(`package test;

// Line comment
public class Test {
    /* Block comment */
    int x;
    
    /**
     * Javadoc comment
     */
    void method() {}
}
`)

	adapter, _ := adapters.GetAdapter("test.java")
	comments, err := adapter.Parse("test.java", src)
	assert.NoError(t, err)

	var lineCount, blockCount, docCount int
	for _, c := range comments {
		switch c.Type {
		case domain.CommentTypeLine:
			lineCount++
		case domain.CommentTypeBlock:
			blockCount++
		case domain.CommentTypeDoc:
			docCount++
		}
	}

	assert.Greater(t, lineCount, 0, "应该有行注释")
	assert.Greater(t, blockCount, 0, "应该有块注释")
	assert.Greater(t, docCount, 0, "应该有文档注释")
}

package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/CairnPack/codei18n/adapters"
	"github.com/CairnPack/codei18n/core/mapping"
	"github.com/CairnPack/codei18n/core/utils"
)

func TestJavaConvert_ApplyMode(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "Test.java")

	originalContent := `package com.example;

// Calculate sum
public class Calculator {
    // Add two numbers
    public int add(int a, int b) {
        return a + b;
    }
}
`
	err := os.WriteFile(testFile, []byte(originalContent), 0644)
	assert.NoError(t, err)

	// Get adapter
	adapter, err := adapters.GetAdapter(testFile)
	assert.NoError(t, err)
	assert.Equal(t, "java", adapter.Language())

	// Parse the comment
	comments, err := adapter.Parse(testFile, []byte(originalContent))
	assert.NoError(t, err)
	assert.Greater(t, len(comments), 0)

	// Create a mapping storage and add translations
	store := mapping.NewStore("")

	expectedTranslations := map[string]string{
		"// Calculate sum":   "计算总和",
		"// Add two numbers": "计算两个数的和",
	}

	// Generate ID for each comment and add translation
	for _, c := range comments {
		id := utils.GenerateCommentID(c)
		if trans, ok := expectedTranslations[c.SourceText]; ok {
			store.Set(id, "en", c.SourceText)
			store.Set(id, "zh-CN", trans)
			t.Logf("设置翻译: ID=%s, EN='%s', ZH='%s'", id, c.SourceText, trans)
		}
	}

	// Validate that a translation can be found
	foundCount := 0
	for _, c := range comments {
		id := utils.GenerateCommentID(c)
		zhText, ok := store.Get(id, "zh-CN")
		if ok && zhText != "" {
			foundCount++
			t.Logf("找到翻译: '%s' -> '%s'", c.SourceText, zhText)
		}
	}

	assert.Greater(t, foundCount, 0, "应该至少找到一个翻译")
}

func TestJavaConvert_RestoreMode(t *testing.T) {
	// Create a Java file containing Chinese comments.
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "Test.java")

	// Already converted to Chinese content
	convertedContent := `package com.example;

// 计算总和
public class Calculator {
    // 计算两个数的和
    public int add(int a, int b) {
        return a + b;
    }
}
`
	err := os.WriteFile(testFile, []byte(convertedContent), 0644)
	assert.NoError(t, err)

	// Get the adapter
	adapter, err := adapters.GetAdapter(testFile)
	assert.NoError(t, err)

	// Parse the comment (currently is Chinese)
	comments, err := adapter.Parse(testFile, []byte(convertedContent))
	assert.NoError(t, err)

	// Create a mapping storage to simulate the existing bilingual mapping
	store := mapping.NewStore("")

	// Build the original English version of the comment to generate the correct ID
	originalEnglishContent := `package com.example;

// Calculate sum
public class Calculator {
    // Add two numbers
    public int add(int a, int b) {
        return a + b;
    }
}
`

	// Parse the English version to get the correct ID
	englishComments, err := adapter.Parse(testFile, []byte(originalEnglishContent))
	assert.NoError(t, err)

	translations := map[string]string{
		"// Calculate sum":   "计算总和",
		"// Add two numbers": "计算两个数的和",
	}

	// Use English comments to store translations for IDs
	for _, c := range englishComments {
		id := utils.GenerateCommentID(c)
		if trans, ok := translations[c.SourceText]; ok {
			store.Set(id, "en", c.SourceText)
			store.Set(id, "zh-CN", trans)
		}
	}

	// Now you can reverse lookup English text through Chinese text.
	for _, c := range comments {
		// Normalize the current comment text
		normalizedCurrent := utils.NormalizeCommentText(c.SourceText)

		// Search for matching Chinese text in the mapping
		found := false
		for id, transMap := range store.GetMapping().Comments {
			zhText, hasZh := transMap["zh-CN"]
			enText, hasEn := transMap["en"]

			if hasZh && hasEn {
				normalizedZh := utils.NormalizeCommentText(zhText)
				if normalizedZh == normalizedCurrent {
					found = true
					t.Logf("反向查找成功: ZH='%s' -> EN='%s' (ID=%s)", zhText, enText, id)
					break
				}
			}
		}

		assert.True(t, found, "应该能通过中文文本找到对应的英文翻译: %s", c.SourceText)
	}
}

func TestJavaConvert_FileSupport(t *testing.T) {
	// Test the convert command to recognize .java files
	tmpDir := t.TempDir()

	testFiles := []string{
		"Test.java",
		"Sample.java",
		"Application.java",
	}

	for _, filename := range testFiles {
		path := filepath.Join(tmpDir, filename)
		content := "package test;\n// Comment\npublic class Test {}"
		err := os.WriteFile(path, []byte(content), 0644)
		assert.NoError(t, err)

		// Validate the adapter can be obtained
		adapter, err := adapters.GetAdapter(path)
		assert.NoError(t, err)
		assert.NotNil(t, adapter)
		assert.Equal(t, "java", adapter.Language())
	}
}

func TestJavaConvert_CommentMarkers(t *testing.T) {
	// Test whether different types of comment markers are handled correctly.
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "Test.java")

	content := `package com.example;

// Line comment
public class Test {
    /* Block comment */
    int x;
    
    /**
     * Javadoc comment
     */
    void method() {}
}
`
	err := os.WriteFile(testFile, []byte(content), 0644)
	assert.NoError(t, err)

	adapter, err := adapters.GetAdapter(testFile)
	assert.NoError(t, err)

	comments, err := adapter.Parse(testFile, []byte(content))
	assert.NoError(t, err)

	// Validate comment type and tag
	for _, c := range comments {
		switch c.Type {
		case "line":
			assert.True(t, strings.HasPrefix(c.SourceText, "//"),
				"行注释应该以 // 开头: %s", c.SourceText)
		case "block":
			assert.True(t, strings.HasPrefix(c.SourceText, "/*") &&
				strings.HasSuffix(c.SourceText, "*/"),
				"块注释应该以 /* 开头和 */ 结尾: %s", c.SourceText)
		case "doc":
			assert.True(t, strings.HasPrefix(c.SourceText, "/**"),
				"文档注释应该以 /** 开头: %s", c.SourceText)
		}
	}
}

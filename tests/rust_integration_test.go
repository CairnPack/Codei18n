package tests

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRustSupport(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	bin := GetBinaryPath(t)
	tempDir := t.TempDir()

	// Create Rust file
	absPath := CreateFile(t, tempDir, "lib.rs", LoadFixture(t, "lib.rs"))

	// Execute Scan
	cmd := exec.Command(bin, "scan", "--file", absPath, "--format", "json")
	cmd.Dir = tempDir
	output, err := cmd.CombinedOutput()
	// Note: If Rust support is not compiled in or fails, this might error.
	// But based on analysis, adapters/rust exists.
	require.NoError(t, err, "Rust scan failed: %s", string(output))

	// Parse JSON
	var result struct {
		File     string                   `json:"file"`
		Comments []map[string]interface{} `json:"comments"`
	}
	err = json.Unmarshal(output, &result)
	require.NoError(t, err)

	assert.True(t, strings.HasSuffix(result.File, "lib.rs"))
	assert.NotEmpty(t, result.Comments)

	// 按 docs/index-format.md §6.1 的连续行注释合并规则（标记相同、行相邻、起始列相同、独占一行的
	// 行注释合并为一条），原来逐行的 11 条记录变为 7 条：
	// 1. Inner doc（第 1-2 行，2 条 //! 合并）
	// 2. Inner doc（第 4 行）
	// 3. Outer doc（第 6 行）
	// 4. Line comment（第 8 行）
	// 5. Block comment（第 9-10 行，块注释不合并）
	// 6. Line comment（第 11-12 行，2 条 // 合并）
	// 7. Doc comment（第 15-18 行，4 条 /// 合并，第 16 行的空 /// 作为段落分隔留在组内）
	assert.Equal(t, 7, len(result.Comments), "连续行注释合并后应为 7 条")

	// Verify Language and Types
	foundInnerDoc := false
	foundOuterDoc := false

	for _, c := range result.Comments {
		// Check Language
		lang, ok := c["language"].(string)
		if ok {
			assert.Equal(t, "rust", lang) // Or however the adapter names it
		}

		src := c["sourceText"].(string)
		cType := c["type"].(string)

		if strings.Contains(src, "Inner doc comment") {
			foundInnerDoc = true
			// Rust doc comments might be classified as "doc" or "line" depending on implementation
			// Usually "doc" if supported
			if cType == "doc" {
				t.Log("Found Inner Doc as 'doc' type")
			}
		}
		if strings.Contains(src, "Outer doc comment") {
			foundOuterDoc = true
			if cType == "doc" {
				t.Log("Found Outer Doc as 'doc' type")
			}
		}
	}

	assert.True(t, foundInnerDoc, "Should find inner doc comment //!")
	assert.True(t, foundOuterDoc, "Should find outer doc comment ///")

	// Verify range logic for first comment (line 1)
	if len(result.Comments) > 0 {
		firstComment := result.Comments[0]
		rng := firstComment["range"].(map[string]interface{})
		startLine := int(rng["startLine"].(float64))
		endLine := int(rng["endLine"].(float64))

		assert.Equal(t, 1, startLine, "First comment should start on line 1")
		// 合并规则：第 1、2 行的 //! 合并为一条，因此结束于第 2 行
		assert.Equal(t, 2, endLine, "第 1-2 行的 //! 应合并为一条，结束于第 2 行")
	}

	// 第 15-18 行的 /// 按合并规则成为一条记录，符号仍为 PrecompileContract
	if len(result.Comments) >= 7 {
		c7 := result.Comments[6]
		assert.Equal(t, "PrecompileContract", c7["symbol"])
		assert.Contains(t, c7["sourceText"].(string), "A mapping of precompile contracts")
		assert.Contains(t, c7["sourceText"].(string), "This is an optimization")
		assert.Contains(t, c7["sourceText"].(string), "dynamic representation")
	}
}

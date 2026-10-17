package tests

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/CairnPack/codei18n/adapters"
	"github.com/CairnPack/codei18n/core/domain"
)

func TestJS_TS_Integration(t *testing.T) {
	// Path to test file
	testFile := filepath.Join("testdata", "typescript", "sample.ts")

	// Get adapter
	adapter, err := adapters.GetAdapter(testFile)
	assert.NoError(t, err)
	assert.NotNil(t, adapter)
	assert.Equal(t, "typescript", adapter.Language())

	// Read file
	// Since we are not running full scan command which reads file, we read it here manually for Parse
	// But in real integration test of 'scan' command, we might run the command itself.
	// Here we test the adapter integration via registry.

	src := []byte(`
class Calculator {
    // Adds two numbers
    add(a: number, b: number): number {
        return a + b;
    }
}
`)
	comments, err := adapter.Parse(testFile, src)
	assert.NoError(t, err)
	assert.NotEmpty(t, comments)

	// Check specific comment
	var found bool
	for _, c := range comments {
		if c.Symbol == "Calculator.add" {
			found = true
			assert.Equal(t, domain.CommentTypeLine, c.Type)
			assert.Contains(t, c.SourceText, "Adds two numbers")
		}
	}
	assert.True(t, found, "Should find comment for Calculator.add")
}

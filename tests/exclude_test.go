package tests

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/workflow"
)

func TestMapUpdate_RespectsExcludePatterns(t *testing.T) {
	// Setup temporary directory
	tempDir, err := os.MkdirTemp("", "codei18n_test_exclude")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create a valid source file
	srcDir := filepath.Join(tempDir, "src")
	os.Mkdir(srcDir, 0755)
	srcFile := filepath.Join(srcDir, "main.go")
	os.WriteFile(srcFile, []byte(`package main
// Valid comment
func main() {}`), 0644)

	// Create a node_modules directory which should be excluded
	nodeModulesDir := filepath.Join(tempDir, "node_modules", "lib")
	os.MkdirAll(nodeModulesDir, 0755)
	ignoredFile := filepath.Join(nodeModulesDir, "ignored.ts")
	os.WriteFile(ignoredFile, []byte(`
// Ignored comment
const x = 1;`), 0644)

	// Create config with excludePatterns
	cfg := &config.Config{
		SourceLanguage: "en",
		LocalLanguage:  "zh-CN",
		ExcludePatterns: []string{
			"node_modules/**",
		},
	}

	// Run Map Update Workflow
	// Use MapUpdate function

	// Create .codei18n dir in CURRENT WORKDIR (because MapUpdate uses hardcoded path relative to CWD mostly, or we should change CWD)
	// MapUpdate uses filepath.Join(".codei18n", "mappings.json") which is relative.
	// So we need to switch CWD to tempDir for this test to work properly without modifying MapUpdate signature yet.
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	os.Chdir(tempDir)

	storePath := filepath.Join(".codei18n", "mappings.json")
	os.MkdirAll(filepath.Dir(storePath), 0755)

	res, err := workflow.MapUpdate(cfg, tempDir, false)
	assert.NoError(t, err)
	assert.NotNil(t, res)

	// Check the store content
	content, err := os.ReadFile(storePath)
	assert.NoError(t, err)

	// We expect "Valid comment" to be present
	assert.Contains(t, string(content), "Valid comment")

	// We expect "Ignored comment" to be ABSENT
	// If the bug exists, this will likely fail (it will contain "Ignored comment")
	if assert.NotContains(t, string(content), "Ignored comment", "Should not contain comments from node_modules") {
		t.Log("Exclude patterns worked correctly!")
	} else {
		t.Log("Exclude patterns FAILED: found ignored comment")
	}
}

func TestConvert_RespectsExcludePatterns(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	bin := GetBinaryPath(t)

	// Setup temporary directory
	tempDir := t.TempDir()

	// Create a node_modules directory which should be excluded
	nodeModulesDir := filepath.Join(tempDir, "node_modules", "lib")
	os.MkdirAll(nodeModulesDir, 0755)
	ignoredFile := filepath.Join(nodeModulesDir, "ignored.ts")
	os.WriteFile(ignoredFile, []byte(`
// Ignored comment
const x = 1;`), 0644)

	// Create a valid source file
	srcDir := filepath.Join(tempDir, "src")
	os.MkdirAll(srcDir, 0755)
	validFile := filepath.Join(srcDir, "valid.ts")
	os.WriteFile(validFile, []byte(`
// Valid comment
const y = 2;`), 0644)

	// Create config with excludePatterns
	cfg := &config.Config{
		SourceLanguage: "en",
		LocalLanguage:  "zh-CN",
		ExcludePatterns: []string{
			"node_modules/**",
		},
	}

	// Create .codei18n directory and write config
	codei18nDir := filepath.Join(tempDir, ".codei18n")
	os.MkdirAll(codei18nDir, 0755)
	cfgBytes, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(codei18nDir, "config.json"), cfgBytes, 0644)

	// Run convert --dry-run
	cmd := exec.Command(bin, "convert", "--to", "zh-CN", "--dry-run", "--dir", ".", "--verbose")
	cmd.Dir = tempDir

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "Convert command failed: %s", string(out))
	output := string(out)

	// Since we haven't created mappings, convert won't do much, but it scans files.
	// In verbose mode, or if it errors on finding files, we might see output.
	// But relying on logs is brittle.

	// Let's create mappings manually so convert has something to do
	// We need IDs.
	// Valid comment ID: SHA1(...)
	// Ignored comment ID: SHA1(...)

	// But easier way: Check if ignored file is even looked at.
	// The log "Scanning directory failed" would appear if bad.
	// Or just trust my previous unit test for map update which uses same exclude logic?
	// MapUpdate uses scanner.Directory. Convert uses manual walk.
	// So I MUST test Convert logic.

	// If I put "Ignored comment" in mapping, convert should NOT try to replace it if file is excluded.
	// If file is excluded, it's not in 'files' list. processFile is not called.
	// So no "Converting..." log.

	// Let's rely on Dry Run output.
	// If files are processed, log.Info("preparing to process %d files...") is printed.
	// Since verbose is on (flag passed to binary? --verbose is global flag).
	// I passed --verbose.

	// Check for "Preparing to process 1 file..." (should be 1, not 2).
	if assert.Contains(t, output, "准备处理 1 个文件") {
		t.Log("Convert correctly identified 1 file")
	} else {
		t.Logf("Convert output did not match expectation: %s", output)
	}

	assert.NotContains(t, output, "ignored.ts")
}

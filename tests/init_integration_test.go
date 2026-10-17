package tests

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitEnhancedFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	bin := GetBinaryPath(t)
	tempDir := t.TempDir()

	// 1. Setup a fake Git repo
	require.NoError(t, exec.Command("git", "init", tempDir).Run())

	// Create a sample file
	sampleContent := `package main
// Test Init
func main() {}`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.go"), []byte(sampleContent), 0644))

	// 2. Run init with translation (using mock provider to avoid API calls)
	cmd := exec.Command(bin, "init", "--with-translate", "--provider", "mock")
	cmd.Dir = tempDir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	output := string(out)

	t.Logf("Init Output: %s", output)

	// 3. Verify Config
	require.FileExists(t, filepath.Join(tempDir, ".codei18n", "config.json"))
	configContent, _ := os.ReadFile(filepath.Join(tempDir, ".codei18n", "config.json"))
	assert.Contains(t, string(configContent), "\"translationProvider\": \"mock\"")

	// 4. Verify Map Update happened
	require.FileExists(t, filepath.Join(tempDir, ".codei18n", "mappings.json"))
	mappingContent, _ := os.ReadFile(filepath.Join(tempDir, ".codei18n", "mappings.json"))
	assert.Contains(t, string(mappingContent), "Test Init")

	// 5. Verify Translation happened
	// JSON marshaling escapes '>', so we unmarshal to verify content properly
	var mappingData map[string]interface{}
	require.NoError(t, json.Unmarshal(mappingContent, &mappingData))

	comments := mappingData["comments"].(map[string]interface{})
	var foundTranslation bool
	for _, v := range comments {
		langs := v.(map[string]interface{})
		if val, ok := langs["zh-CN"]; ok {
			strVal := val.(string)
			if strings.Contains(strVal, "MOCK en->zh-CN") {
				foundTranslation = true
				break
			}
		}
	}
	assert.True(t, foundTranslation, "Should contain mock translation")

	// 6. Verify Git Integration
	// Check .gitignore
	gitignorePath := filepath.Join(tempDir, ".gitignore")
	require.FileExists(t, gitignorePath)
	gitIgnore, _ := os.ReadFile(gitignorePath)
	assert.Contains(t, string(gitIgnore), ".codei18n/")

	// Check Hook (only if we are on a system where hooks are installed to .git/hooks)
	// Since we did git init, .git/hooks should exist.
	hookPath := filepath.Join(tempDir, ".git", "hooks", "pre-commit")
	require.FileExists(t, hookPath)
	hookContent, _ := os.ReadFile(hookPath)
	assert.Contains(t, string(hookContent), "CodeI18n Pre-commit Hook")
}

func TestInitGlobalConfigInheritance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	bin := GetBinaryPath(t)
	tempDir := t.TempDir()

	// Create a fake global config in a subfolder
	fakeHome := filepath.Join(tempDir, "fake_home")
	os.MkdirAll(filepath.Join(fakeHome, ".codei18n"), 0755)

	globalConfig := `{
		"sourceLanguage": "fr",
		"localLanguage": "de",
		"translationProvider": "openai",
		"translationConfig": {
			"api_key": "secret-key",
			"model": "gpt-4"
		}
	}`
	os.WriteFile(filepath.Join(fakeHome, ".codei18n", "config.json"), []byte(globalConfig), 0644)

	// Run init in a project dir, pointing to fake config via --config flag?
	// The init command logic uses config.LoadConfig() which looks at HOME.
	// We cannot easily mock HOME env var for the subprocess unless we set it.
	projectDir := filepath.Join(tempDir, "project")
	os.MkdirAll(projectDir, 0755)

	cmd := exec.Command(bin, "init")
	cmd.Dir = projectDir
	// Mock HOME to point to fake_home（Windows 上 os.UserHomeDir 读取 USERPROFILE）
	cmd.Env = append(os.Environ(), "HOME="+fakeHome, "USERPROFILE="+fakeHome)

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	// Verify project config
	projectConfigPath := filepath.Join(projectDir, ".codei18n", "config.json")
	require.FileExists(t, projectConfigPath)

	content, _ := os.ReadFile(projectConfigPath)
	sContent := string(content)

	// Should inherit non-sensitive fields
	assert.Contains(t, sContent, "\"sourceLanguage\": \"fr\"")
	assert.Contains(t, sContent, "\"localLanguage\": \"de\"")
	assert.Contains(t, sContent, "\"model\": \"gpt-4\"")

	// Should NOT inherit sensitive fields
	assert.NotContains(t, sContent, "secret-key")
	assert.NotContains(t, sContent, "api_key")
}

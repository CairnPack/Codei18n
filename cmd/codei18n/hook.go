package main

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/CairnPack/codei18n/internal/log"
)

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "管理 Git Hooks",
}

var hookInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "安装 pre-commit hook",
	Run: func(cmd *cobra.Command, args []string) {
		runHookInstall()
	},
}

var hookUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "卸载 pre-commit hook",
	Run: func(cmd *cobra.Command, args []string) {
		runHookUninstall()
	},
}

func init() {
	rootCmd.AddCommand(hookCmd)
	hookCmd.AddCommand(hookInstallCmd)
	hookCmd.AddCommand(hookUninstallCmd)
}

func runHookInstall() {
	if err := installPreCommitHook(); err != nil {
		log.Fatal("安装 pre-commit hook 失败: %v", err)
	}
	if err := installCommitMsgHook(); err != nil {
		log.Fatal("安装 commit-msg hook 失败: %v", err)
	}
	log.Success("Git Hooks 安装成功")
}

func installPreCommitHook() error {
	gitDir := ".git"
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return os.ErrNotExist
	}

	hookPath := filepath.Join(gitDir, "hooks", "pre-commit")

	// Create hook content
	// Pre-commit workflow:
	// 1. Update mappings for staged files (adds new Chinese comments)
	// 2. Translate missing English translations
	// 3. Convert comments to English
	// 4. Re-stage modified files
	hookContent := `#!/bin/sh
# CodeI18n Pre-commit Hook
# Converts comments to English before committing

echo "CodeI18n: Checking for non-English comments..."

# Get staged Go files
FILES=$(git diff --cached --name-only --diff-filter=ACM | grep "\.go$")

if [ -z "$FILES" ]; then
    exit 0
fi

# Determine executable
CODEI18N_CMD="codei18n"
if ! command -v $CODEI18N_CMD &> /dev/null; then
    echo "CodeI18n: command not found. Skipping."
    exit 0
fi

# Step 1: Update mappings for new comments
echo "CodeI18n: Updating mappings..."
$CODEI18N_CMD map update --scan-dir .

# Step 2: Translate any missing English translations
echo "CodeI18n: Translating missing entries..."
$CODEI18N_CMD translate --provider openai 2>/dev/null || echo "CodeI18n: Translation skipped (check API config: provider=openai/llm/ollama)"

# Step 3: Convert staged files to English
for file in $FILES; do
    echo "CodeI18n: Processing $file..."
    $CODEI18N_CMD convert --to en --file "$file"
    
    # Add back to staging
    git add "$file"
done

echo "CodeI18n: Done."
exit 0
`

	if err := os.WriteFile(hookPath, []byte(hookContent), 0755); err != nil {
		return err
	}
	return nil
}

func installCommitMsgHook() error {
	gitDir := ".git"
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return os.ErrNotExist
	}

	hookPath := filepath.Join(gitDir, "hooks", "commit-msg")

	hookContent := `#!/bin/sh
# CodeI18n Commit-Msg Hook
# Translates Chinese commit messages to English

COMMIT_MSG_FILE=$1
COMMIT_MSG=$(cat "$COMMIT_MSG_FILE")

# Check for Chinese characters using Perl
if echo "$COMMIT_MSG" | perl -C -ne 'exit 0 if /\p{Han}/; exit 1'; then
    echo "CodeI18n: Detected Chinese in commit message. Translating..." >&2
    
    CODEI18N_CMD="codei18n"
    # Check if codei18n is in PATH
    if ! command -v $CODEI18N_CMD >/dev/null 2>&1; then
        # Fallback to go run if in project root
        if [ -f "go.mod" ] && [ -d "cmd/codei18n" ]; then
             CODEI18N_CMD="go run cmd/codei18n/*.go"
        else
             echo "CodeI18n: command not found. Skipping translation." >&2
             exit 0
        fi
    fi

    # Translate
    TRANSLATED=$(echo "$COMMIT_MSG" | $CODEI18N_CMD translate -s zh-CN -t en 2>/dev/null)
    
    if [ $? -eq 0 ] && [ -n "$TRANSLATED" ]; then
        echo "$TRANSLATED" > "$COMMIT_MSG_FILE"
        echo "CodeI18n: Translated to: $TRANSLATED" >&2
    else
        echo "CodeI18n: Translation failed. Keeping original message." >&2
    fi
fi
`

	if err := os.WriteFile(hookPath, []byte(hookContent), 0755); err != nil {
		return err
	}
	return nil
}

func runHookUninstall() {
	uninstallHook("pre-commit")
	uninstallHook("commit-msg")
	log.Success("Git Hooks 已卸载")
}

func uninstallHook(name string) {
	hookPath := filepath.Join(".git", "hooks", name)
	if err := os.Remove(hookPath); err != nil {
		if !os.IsNotExist(err) {
			log.Warn("卸载 %s hook 失败: %v", name, err)
		}
	}
}

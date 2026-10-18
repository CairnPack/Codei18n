// Package gitint 负责与 Git 的集成：安装 / 卸载 hook、维护 .gitignore。
package gitint

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotGitRepo 表示目录不是 Git 仓库根目录。
var ErrNotGitRepo = errors.New("不是 Git 仓库")

// IsRepo 判断 root 下是否有 .git。
func IsRepo(root string) bool {
	_, err := os.Stat(filepath.Join(root, ".git"))
	return err == nil
}

// EnsureGitignore 确保 .gitignore 中包含 .codei18n/；返回是否做了修改。
func EnsureGitignore(root string) (bool, error) {
	path := filepath.Join(root, ".gitignore")
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	s := string(content)
	if strings.Contains(s, ".codei18n/") {
		return false, nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if len(content) > 0 && !strings.HasSuffix(s, "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return false, err
		}
	}
	if _, err := f.WriteString(".codei18n/\n"); err != nil {
		return false, err
	}
	return true, nil
}

// InstallHooks 安装 pre-commit 与 commit-msg hook。
func InstallHooks(root string) error {
	if err := InstallPreCommit(root); err != nil {
		return err
	}
	return InstallCommitMsg(root)
}

// InstallPreCommit 安装 pre-commit hook。
func InstallPreCommit(root string) error { return writeHook(root, "pre-commit", PreCommitHook) }

// InstallCommitMsg 安装 commit-msg hook。
func InstallCommitMsg(root string) error { return writeHook(root, "commit-msg", CommitMsgHook) }

// Uninstall 删除 codei18n 安装的 hook；返回删除失败的错误（不存在不算错误）。
func Uninstall(root, name string) error {
	err := os.Remove(filepath.Join(root, ".git", "hooks", name))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// HooksInstalled 判断两个 hook 是否都由 codei18n 安装。
func HooksInstalled(root string) bool {
	for _, name := range []string{"pre-commit", "commit-msg"} {
		data, err := os.ReadFile(filepath.Join(root, ".git", "hooks", name))
		if err != nil || !strings.Contains(string(data), "CodeI18n") {
			return false
		}
	}
	return true
}

func writeHook(root, name, content string) error {
	if !IsRepo(root) {
		return os.ErrNotExist
	}
	dir := filepath.Join(root, ".git", "hooks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o755)
}

// PreCommitHook 是 pre-commit hook 脚本：更新映射 → 翻译缺失条目 → 把暂存的文件转换为英文。
// 翻译使用项目配置中选用的 provider 与系统凭据库中的令牌，无需额外设置。
const PreCommitHook = `#!/bin/sh
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
$CODEI18N_CMD translate 2>/dev/null || echo "CodeI18n: Translation skipped (check: codei18n auth status)"

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

// CommitMsgHook 是 commit-msg hook 脚本：把中文提交信息翻译为英文。
const CommitMsgHook = `#!/bin/sh
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

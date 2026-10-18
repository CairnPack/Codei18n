package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/CairnPack/codei18n"
	"github.com/CairnPack/codei18n/core/secrets"
)

// fakeOpenAI 模拟 OpenAI 兼容的 chat completions 接口：校验 Authorization，返回 "译文:" 前缀。
func fakeOpenAI(t *testing.T, token string, calls *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"error":{"message":"bad key"}}`)
			return
		}
		calls.Add(1)
		var req struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		content := "译文"
		if strings.Contains(req.Messages[0].Content, "JSON array") {
			// 批量请求：按输入数组长度返回
			start := strings.Index(req.Messages[0].Content, "Input:\n")
			var in []string
			json.Unmarshal([]byte(req.Messages[0].Content[start+len("Input:\n"):]), &in)
			out := make([]string, len(in))
			for i := range in {
				out[i] = "译文"
			}
			b, _ := json.Marshal(out)
			content = string(b)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "x", "object": "chat.completion", "created": time.Now().Unix(), "model": "m",
			"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": content}, "finish_reason": "stop"}},
		})
	}))
}

// cleanEnv 返回去掉令牌环境变量、并把用户目录指向 home 的环境。
func cleanEnv(home string, extra ...string) []string {
	var env []string
	for _, kv := range os.Environ() {
		k := strings.ToUpper(strings.SplitN(kv, "=", 2)[0])
		if k == "OPENAI_API_KEY" || k == "CODEI18N_API_KEY" || k == "HOME" || k == "USERPROFILE" || k == "GOCOVERDIR" {
			continue
		}
		env = append(env, kv)
	}
	return append(append(env, "HOME="+home, "USERPROFILE="+home), extra...)
}

// 在"桌面端"（core API）保存的令牌，CLI translate 与 git hook 无需任何设置即可使用。
func TestTokenSavedViaCoreIsUsedByCLIAndGitHook(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	if runtime.GOOS != "windows" || os.Getenv("CODEI18N_SKIP_CREDMAN") != "" {
		t.Skip("需要 Windows 凭据管理器")
	}
	bin := GetBinaryPath(t)
	token := fmt.Sprintf("it-token-%d-ABCD", time.Now().UnixNano())
	profile := fmt.Sprintf("it-%d", time.Now().UnixNano())
	var calls atomic.Int32
	srv := fakeOpenAI(t, token, &calls)
	defer srv.Close()

	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	globalDir := filepath.Join(home, ".codei18n")
	require.NoError(t, os.MkdirAll(globalDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(globalDir, "config.json"), []byte(fmt.Sprintf(
		`{"profile":%q,"profiles":{%q:{"provider":"openai","baseUrl":%q,"model":"m"}}}`, profile, profile, srv.URL+"/v1")), 0o644))

	// 1. 像桌面端一样通过 core API 保存令牌（真实凭据管理器）
	g, err := codei18n.OpenGlobal(codei18n.WithGlobalDir(globalDir))
	require.NoError(t, err)
	require.NoError(t, g.SetProfileToken(profile, codei18n.NewSecret([]byte(token))))
	t.Cleanup(func() { secrets.System().Delete(secrets.ProfileAccount(profile)) })

	// 2. CLI translate：环境中没有任何令牌变量
	proj := filepath.Join(tmp, "proj")
	require.NoError(t, os.MkdirAll(proj, 0o755))
	require.NoError(t, exec.Command("git", "init", "-q", proj).Run())
	require.NoError(t, os.WriteFile(filepath.Join(proj, "a.go"), []byte("package a\n\n// Hello world\nfunc A() {}\n"), 0o644))
	run := func(args ...string) string {
		cmd := exec.Command(bin, args...)
		cmd.Dir = proj
		cmd.Env = cleanEnv(home)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return string(out)
	}
	run("init")
	status := run("auth", "status")
	require.Contains(t, status, "生效来源: 全局令牌")
	require.Contains(t, status, "••••ABCD")
	require.NotContains(t, status, token)
	out := run("translate")
	require.Contains(t, out, "翻译完成")
	require.NotContains(t, out, token)
	require.Greater(t, calls.Load(), int32(0))

	// 3. git hook：pre-commit 调用 PATH 中的 codei18n translate，同样无需设置
	binDir := filepath.Join(tmp, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	data, err := os.ReadFile(bin)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "codei18n.exe"), data, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(proj, "b.go"), []byte("package a\n\n// Second comment\nfunc B() {}\n"), 0o644))
	before := calls.Load()
	git := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = proj
		cmd.Env = cleanEnv(home, "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return string(out)
	}
	git("add", "a.go", "b.go")
	hookOut := git("commit", "-m", "test commit")
	require.NotContains(t, hookOut, "Translation skipped", hookOut)
	require.Greater(t, calls.Load(), before, "pre-commit hook 应调用翻译接口: %s", hookOut)
	mappings, err := os.ReadFile(filepath.Join(proj, ".codei18n", "mappings.json"))
	require.NoError(t, err)
	require.Contains(t, string(mappings), "Second comment")
	require.NotContains(t, string(mappings), token)
}

package codei18n

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/secrets"
)

type authFixture struct {
	t      *testing.T
	root   string
	global string
	store  *secrets.Memory
	env    map[string]string
	logs   *captureLogger
}

type captureLogger struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (l *captureLogger) add(level, f string, a ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(&l.buf, level+" "+f+"\n", a...)
}
func (l *captureLogger) Debugf(f string, a ...any) { l.add("DEBUG", f, a...) }
func (l *captureLogger) Infof(f string, a ...any)  { l.add("INFO", f, a...) }
func (l *captureLogger) Warnf(f string, a ...any)  { l.add("WARN", f, a...) }
func (l *captureLogger) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func newAuthFixture(t *testing.T) *authFixture {
	f := &authFixture{t: t, root: t.TempDir(), global: t.TempDir(), store: secrets.NewMemory(), env: map[string]string{}, logs: &captureLogger{}}
	return f
}

func (f *authFixture) opts() []Option {
	return []Option{WithGlobalDir(f.global), WithSecretStore(f.store), WithLogger(f.logs),
		WithEnv(func(k string) string { return f.env[k] })}
}

func (f *authFixture) project() *Project {
	p, err := Open(f.root, f.opts()...)
	if err != nil {
		f.t.Fatal(err)
	}
	if !p.Initialized() {
		if err := p.Init(InitOptions{}); err != nil {
			f.t.Fatal(err)
		}
	}
	return p
}

func (f *authFixture) globalCfg(s string) { writeFile(f.t, filepath.Join(f.global, "config.json"), s) }

func (f *authFixture) status(p *Project) *TokenStatus {
	st, err := p.TokenStatus()
	if err != nil {
		f.t.Fatal(err)
	}
	return st
}

func TestTokenPriority(t *testing.T) {
	f := newAuthFixture(t)
	f.globalCfg(`{"profile":"ds","profiles":{"ds":{"provider":"openai"}}}`)
	p := f.project()

	st := f.status(p)
	if st.Source != TokenSourceNone || !st.Required || st.Profile != "ds" {
		t.Fatalf("%+v", st)
	}
	_, _, err := p.resolveToken(mustCfg(t, p))
	if !errors.Is(err, ErrNoToken) || !strings.Contains(err.Error(), "codei18n auth set --profile ds") || !strings.Contains(err.Error(), EnvAPIKey) {
		t.Fatalf("错误信息应说明在哪里设置: %v", err)
	}

	g, _ := OpenGlobal(f.opts()...)
	if err := g.SetProfileToken("ds", NewSecret([]byte("global-token-AAAA1111"))); err != nil {
		t.Fatal(err)
	}
	st = f.status(p)
	if st.Source != TokenSourceGlobal || st.Last4 != "1111" || !st.GlobalSaved || st.GlobalAccount != "profile/ds" {
		t.Fatalf("%+v", st)
	}

	if err := p.SetProjectToken("ds", NewSecret([]byte("project-token-BBBB2222"))); err != nil {
		t.Fatal(err)
	}
	st = f.status(p)
	if st.Source != TokenSourceProject || st.Last4 != "2222" || !st.ProjectSaved || st.GlobalLast4 != "1111" {
		t.Fatalf("%+v", st)
	}

	f.env[EnvLegacyAPIKey] = "legacy-env-CCCC3333"
	if st = f.status(p); st.Source != TokenSourceEnv || st.EnvVar != EnvLegacyAPIKey || st.Last4 != "3333" {
		t.Fatalf("%+v", st)
	}
	f.env[EnvAPIKey] = "env-token-DDDD4444"
	if st = f.status(p); st.EnvVar != EnvAPIKey || st.Last4 != "4444" {
		t.Fatalf("CODEI18N_API_KEY 应优先: %+v", st)
	}

	// 删除项目令牌后回退到全局
	delete(f.env, EnvAPIKey)
	delete(f.env, EnvLegacyAPIKey)
	if err := p.DeleteProjectToken("ds"); err != nil {
		t.Fatal(err)
	}
	if st = f.status(p); st.Source != TokenSourceGlobal {
		t.Fatalf("%+v", st)
	}
}

func mustCfg(t *testing.T, p *Project) *config.Effective {
	cfg, err := p.Config()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestAPIKeyRefAndNonRequiredProviders(t *testing.T) {
	f := newAuthFixture(t)
	f.globalCfg(`{"profile":"a","profiles":{"a":{"apiKeyRef":"shared/team"},"local":{"provider":"ollama"}}}`)
	f.store.Set("shared/team", NewSecret([]byte("team-token-EEEE5555")))
	p := f.project()
	if st := f.status(p); st.Source != TokenSourceGlobal || st.GlobalAccount != "shared/team" || st.Last4 != "5555" {
		t.Fatalf("%+v", st)
	}
	p.UpdateConfig(func(c *config.File) error { c.Profile = config.Ptr("local"); return nil })
	if st := f.status(p); st.Required {
		t.Fatalf("ollama 不需要令牌: %+v", st)
	}
}

func TestLegacyPlaintextUsedWithWarningAndMigrated(t *testing.T) {
	f := newAuthFixture(t)
	f.globalCfg(`{"translationConfig":{"api_key":"global-plain-FFFF6666","model":"m"},"profiles":{"x":{"token":"profile-plain-GGGG7777"}}}`)
	p := f.project()
	if err := p.UpdateConfig(func(c *config.File) error {
		c.TranslationConfig = map[string]string{"apiKey": "project-plain-HHHH8888", "secret": "other-plain-IIII9999"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	st := f.status(p)
	if st.Source != TokenSourceFile || st.Last4 != "8888" || len(st.Plaintext) != 4 {
		t.Fatalf("%+v", st)
	}
	if !strings.Contains(f.logs.String(), "明文令牌") || strings.Contains(f.logs.String(), "HHHH8888") {
		t.Fatalf("应警告且不含明文: %s", f.logs.String())
	}

	migrated, err := p.MigratePlaintextTokens()
	if err != nil || len(migrated) != 2 {
		t.Fatalf("%v %v", migrated, err)
	}
	data, _ := os.ReadFile(p.ConfigPath())
	if strings.Contains(string(data), "plain") {
		t.Fatalf("明文应已从项目配置删除: %s", data)
	}
	if st = f.status(p); st.Source != TokenSourceProject || st.Last4 != "8888" {
		t.Fatalf("迁移后应来自项目令牌: %+v", st)
	}

	g, _ := OpenGlobal(f.opts()...)
	gm, err := g.MigratePlaintextTokens()
	if err != nil || len(gm) != 2 {
		t.Fatalf("%v %v", gm, err)
	}
	gdata, _ := os.ReadFile(g.Path())
	if strings.Contains(string(gdata), "plain") || !strings.Contains(string(gdata), `"model": "m"`) {
		t.Fatalf("全局明文应删除且保留其他字段: %s", gdata)
	}
	if s, _ := f.store.Get("profile/default"); s.Last4() != "6666" {
		t.Fatal("顶层明文应迁到 profile/default")
	}
	if s, _ := f.store.Get("profile/x"); s.Last4() != "7777" {
		t.Fatal("profile 明文应迁到 profile/x")
	}
}

func TestProfileManagementMovesTokens(t *testing.T) {
	f := newAuthFixture(t)
	g, _ := OpenGlobal(f.opts()...)
	if err := g.SaveProfile("a", config.ProviderFields{Model: config.Ptr("m")}); err != nil {
		t.Fatal(err)
	}
	if err := g.SaveProfile("bad name", config.ProviderFields{}); err == nil {
		t.Fatal("非法名称应报错")
	}
	g.Update(func(c *config.File) error { c.Profile = config.Ptr("a"); return nil })
	g.SetProfileToken("a", NewSecret([]byte("token-a-JJJJ0000")))
	p := f.project()
	p.SetProjectToken("a", NewSecret([]byte("token-pa-KKKK0000")))

	if err := g.CopyProfile("a", "b"); err != nil {
		t.Fatal(err)
	}
	if saved, _, _ := g.ProfileTokenStatus("b"); saved {
		t.Fatal("复制不应复制令牌")
	}
	if err := g.RenameProfile("a", "c"); err != nil {
		t.Fatal(err)
	}
	if saved, last4, _ := g.ProfileTokenStatus("c"); !saved || last4 != "0000" {
		t.Fatal("重命名应迁移全局令牌")
	}
	cfgID := mustCfg(t, p).ProjectID
	if _, err := f.store.Get(secrets.ProjectAccount(cfgID, "c")); err != nil {
		t.Fatal("重命名应迁移项目令牌")
	}
	file, _ := g.File()
	if *file.Profile != "c" {
		t.Fatal("全局选择应同步更新")
	}
	if err := g.DeleteProfile("c"); err != nil {
		t.Fatal(err)
	}
	if left, _ := f.store.List(""); len(left) != 0 {
		t.Fatalf("删除配置档应删除其令牌: %v", left)
	}
	if err := g.DeleteProfile("c"); !errors.Is(err, ErrProfileNotFound) {
		t.Fatal(err)
	}
}

func TestTokenSizeLimit(t *testing.T) {
	f := newAuthFixture(t)
	g, _ := OpenGlobal(f.opts()...)
	if err := g.SetProfileToken("a", NewSecret([]byte(strings.Repeat("x", secrets.MaxSize+1)))); !errors.Is(err, secrets.ErrTooLarge) {
		t.Fatal(err)
	}
}

// 防泄漏：翻译后端返回的错误回显了 Authorization 头与完整 key，结果、失败记录、日志、进度事件、
// 磁盘文件中都不能出现令牌或其片段。
func TestTokenNeverLeaksThroughBackendErrors(t *testing.T) {
	const token = "sk-proj-SuperSecretValue1234567890XYZW"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		msg := fmt.Sprintf("Incorrect API key provided: %s. Header was %s. tail=%s", token, auth, token[len(token)-12:])
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": msg, "type": "invalid_request_error"}})
	}))
	defer srv.Close()

	f := newAuthFixture(t)
	f.globalCfg(fmt.Sprintf(`{"profiles":{"p":{"provider":"openai","baseUrl":%q,"model":"gpt-x"}},"profile":"p"}`, srv.URL+"/v1"))
	g, _ := OpenGlobal(f.opts()...)
	if err := g.SetProfileToken("p", NewSecret([]byte(token))); err != nil {
		t.Fatal(err)
	}
	p := f.project()
	writeFile(t, filepath.Join(f.root, "a.go"), "package a\n\n// Hello one\nfunc A() {}\n\n// Hello two\nfunc B() {}\n")
	if _, err := p.UpdateMappings(context.Background(), MapOptions{}); err != nil {
		t.Fatal(err)
	}
	var events strings.Builder
	var mu sync.Mutex
	res, err := p.Translate(context.Background(), TranslateOptions{Progress: func(e Event) {
		mu.Lock()
		fmt.Fprintf(&events, "%+v\n", e)
		mu.Unlock()
	}})
	if err != nil {
		t.Fatal(err)
	}
	if res.FailCount != 2 || len(res.Failed) != 2 {
		t.Fatalf("%+v", res)
	}
	_, textErr := p.TranslateText(context.Background(), "hi", "", "")

	var all strings.Builder
	fmt.Fprintf(&all, "%+v\n%s\n%s\n%v\n", res, events.String(), f.logs.String(), textErr)
	filepath.Walk(f.root, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			data, _ := os.ReadFile(path)
			all.Write(data)
		}
		return nil
	})
	out := all.String()
	if !strings.Contains(out, "Incorrect API key") {
		t.Fatalf("应保留可读的错误信息: %s", out)
	}
	for _, frag := range []string{token, "SuperSecretValue", "1234567890XYZW", token[len(token)-12:]} {
		if strings.Contains(out, frag) {
			t.Fatalf("令牌片段 %q 泄漏:\n%s", frag, out)
		}
	}
}

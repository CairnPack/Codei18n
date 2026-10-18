package codei18n

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CairnPack/codei18n/adapters/translator"
	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/index"
	"github.com/CairnPack/codei18n/core/mapping"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newProject 创建一个带若干 Go 文件的已初始化项目，全局配置目录隔离在临时目录。
func newProject(t *testing.T, files map[string]string, opts ...Option) *Project {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		writeFile(t, filepath.Join(root, name), content)
	}
	opts = append([]Option{WithGlobalDir(t.TempDir())}, opts...)
	p, err := Open(root, opts...)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Init(InitOptions{Provider: config.Ptr("mock")}); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestOpenRequiresDirectory(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("不存在的目录应报错")
	}
}

func TestInitWritesProjectIDAndRefusesOverwrite(t *testing.T) {
	p := newProject(t, nil)
	cfg, err := p.Config()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ProjectID) != 36 {
		t.Fatalf("projectId: %q", cfg.ProjectID)
	}
	if err := p.Init(InitOptions{}); !errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("got %v", err)
	}
	if err := p.Init(InitOptions{Force: true, LocalLanguage: config.Ptr("ja")}); err != nil {
		t.Fatal(err)
	}
	cfg2, _ := p.Config()
	if cfg2.ProjectID != cfg.ProjectID || cfg2.LocalLanguage.Value != "ja" {
		t.Fatalf("force 应保留 projectId: %+v", cfg2)
	}
}

func TestConfigLayersWithGlobalDir(t *testing.T) {
	global := t.TempDir()
	writeFile(t, filepath.Join(global, "config.json"), `{"localLanguage":"de","profiles":{"p":{"model":"m1"}},"profile":"p"}`)
	root := t.TempDir()
	p, _ := Open(root, WithGlobalDir(global), WithOverrides(config.Overrides{SourceLanguage: config.Ptr("fr")}))
	cfg, err := p.Config()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LocalLanguage.Source != config.LayerGlobal || cfg.SourceLanguage.Source != config.LayerOverride {
		t.Fatalf("%+v", cfg)
	}
	if cfg.Provider.Model.Value != "m1" {
		t.Fatalf("%+v", cfg.Provider.Model)
	}
}

func TestUpdateMappingsWritesIndexAndMappings(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.go":    "package main\n\n// Hello world\nfunc main() {}\n",
		"sub/zh.go":  "package sub\n\n// 你好世界\nfunc F() {}\n",
		"skip/a.txt": "not code",
	})
	var phases sync.Map
	res, err := p.UpdateMappings(context.Background(), MapOptions{Progress: func(e Event) { phases.Store(e.Phase, true) }})
	if err != nil {
		t.Fatal(err)
	}
	if res.TotalComments != 2 || res.AddedCount != 2 {
		t.Fatalf("%+v", res)
	}
	for _, ph := range []Phase{PhaseDiscover, PhaseScan, PhaseMap, PhaseDone} {
		if _, ok := phases.Load(ph); !ok {
			t.Errorf("缺少进度阶段 %s", ph)
		}
	}
	idx, err := index.Read(filepath.Join(p.DataDir(), index.FileName))
	if err != nil || idx == nil {
		t.Fatalf("index: %v", err)
	}
	zh := idx.Files["sub/zh.go"]
	if zh == nil || len(zh.Comments) != 1 {
		t.Fatalf("index files: %v", idx.Files)
	}
	c := zh.Comments[0]
	if c.Lang != "zh-Hans" || c.SourceKey != "zh-CN" || c.Parts != 1 || c.Type != "line" {
		t.Fatalf("%+v", c)
	}
	if c.Range != (index.Range{StartLine: 3, StartCol: 1, EndLine: 3, EndCol: 8}) {
		t.Fatalf("range 应为 UTF-16 列: %+v", c.Range)
	}
	if c.TextHash != index.TextHash("// 你好世界") {
		t.Fatal("textHash")
	}
	if idx.Files["main.go"].ContentHash != index.ContentHash([]byte("package main\n\n// Hello world\nfunc main() {}\n")) {
		t.Fatal("contentHash")
	}
	if !strings.HasPrefix(idx.Generator, "codei18n/") {
		t.Fatal(idx.Generator)
	}

	// mappings.json 格式保持不变：ID → 语言 → 文本
	store := mapping.NewStore(filepath.Join(p.DataDir(), mapping.FileName))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if text, ok := store.Get(c.ID, "zh-CN"); !ok || text != "// 你好世界" {
		t.Fatalf("mapping: %q", text)
	}
}

func TestMultipleProjectsInParallel(t *testing.T) {
	var projects []*Project
	for i := 0; i < 4; i++ {
		projects = append(projects, newProject(t, map[string]string{
			"a.go": fmt.Sprintf("package a\n\n// project %d\nfunc A() {}\n", i),
		}))
	}
	var wg sync.WaitGroup
	for _, p := range projects {
		wg.Add(1)
		go func(p *Project) {
			defer wg.Done()
			if _, err := p.UpdateMappings(context.Background(), MapOptions{}); err != nil {
				t.Error(err)
			}
			if _, err := p.Translate(context.Background(), TranslateOptions{}); err != nil {
				t.Error(err)
			}
		}(p)
	}
	wg.Wait()
	for i, p := range projects {
		recs, err := p.Comments()
		if err != nil || len(recs) != 1 {
			t.Fatalf("%d: %v %v", i, recs, err)
		}
		if want := fmt.Sprintf("// project %d", i); recs[0].Original != want {
			t.Fatalf("项目之间串了数据: %q", recs[0].Original)
		}
		if !strings.Contains(recs[0].Translation("zh"), "MOCK en->zh-CN") {
			t.Fatalf("%+v", recs[0].Texts)
		}
	}
}

// blockingTranslator 每批翻译都会等待 release 信号，便于测试取消。
type blockingTranslator struct {
	batches atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (b *blockingTranslator) Translate(ctx context.Context, text, from, to string) (string, error) {
	r, err := b.TranslateBatch(ctx, []string{text}, from, to)
	if err != nil {
		return "", err
	}
	return r[0], nil
}

func (b *blockingTranslator) TranslateBatch(ctx context.Context, texts []string, from, to string) ([]string, error) {
	b.batches.Add(1)
	b.started <- struct{}{}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-b.release:
	}
	return translator.NewMockTranslator().TranslateBatch(ctx, texts, from, to)
}

func TestTranslateCancelStopsWithinOneBatchAndPersists(t *testing.T) {
	files := map[string]string{}
	for i := 0; i < 30; i++ {
		files[fmt.Sprintf("f%02d.go", i)] = fmt.Sprintf("package f\n\n// comment %d\nfunc F%d() {}\n", i, i)
	}
	bt := &blockingTranslator{started: make(chan struct{}, 100), release: make(chan struct{})}
	p := newProject(t, files, WithTranslator(bt))
	if err := p.UpdateConfig(func(f *config.File) error { f.BatchSize = config.Ptr(5); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpdateMappings(context.Background(), MapOptions{}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	type out struct {
		res *TranslateResult
		err error
	}
	done := make(chan out, 1)
	go func() {
		res, err := p.Translate(ctx, TranslateOptions{Concurrency: 1})
		done <- out{res, err}
	}()

	<-bt.started            // 第 1 批开始
	bt.release <- struct{}{} // 第 1 批完成
	<-bt.started            // 第 2 批开始（进行中）
	cancel()

	var o out
	select {
	case o = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("取消后没有及时返回")
	}
	if !errors.Is(o.err, context.Canceled) || o.res == nil || !o.res.Canceled {
		t.Fatalf("%+v %v", o.res, o.err)
	}
	if o.res.SuccessCount != 5 {
		t.Fatalf("应只完成第 1 批: %+v", o.res)
	}
	if n := bt.batches.Load(); n != 2 {
		t.Fatalf("取消后不应再派发新批次，实际 %d 批", n)
	}
	st, err := p.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.Translated != 5 || st.Pending != 25 {
		t.Fatalf("已完成的结果应已落盘: %+v", st)
	}
}

func TestBusyAndEditing(t *testing.T) {
	bt := &blockingTranslator{started: make(chan struct{}, 10), release: make(chan struct{})}
	p := newProject(t, map[string]string{"a.go": "package a\n\n// Hello\nfunc A() {}\n"}, WithTranslator(bt))
	if _, err := p.UpdateMappings(context.Background(), MapOptions{}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := p.Translate(context.Background(), TranslateOptions{}); done <- err }()
	<-bt.started
	recs, _ := p.Comments()
	if err := p.SetTranslation(recs[0].ID, "zh-CN", "你好"); !errors.Is(err, ErrBusy) {
		t.Fatalf("翻译进行中编辑应返回 ErrBusy: %v", err)
	}
	bt.release <- struct{}{}
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	if err := p.SetTranslation(recs[0].ID, "zh", "手动译文"); err != nil {
		t.Fatal(err)
	}
	recs, _ = p.Comments()
	if recs[0].Texts["zh-CN"] != "手动译文" {
		t.Fatalf("应按等价语言码写回已有的 zh-CN 键: %+v", recs[0].Texts)
	}
	if err := p.SetTranslation("nope", "zh-CN", "x"); !errors.Is(err, ErrUnknownComment) {
		t.Fatal(err)
	}
}

func TestSetCommentLanguageSurvivesRescan(t *testing.T) {
	p := newProject(t, map[string]string{"a.go": "package a\n\n// 漢字だけ\nfunc A() {}\n"})
	if _, err := p.UpdateMappings(context.Background(), MapOptions{}); err != nil {
		t.Fatal(err)
	}
	recs, _ := p.Comments()
	id := recs[0].ID
	if recs[0].SourceKey != "zh-CN" {
		t.Fatalf("%+v", recs[0])
	}
	if err := p.SetCommentLanguage(id, "en"); err != nil {
		t.Fatal(err)
	}
	check := func() {
		t.Helper()
		recs, _ := p.Comments()
		r := recs[0]
		if r.Lang != "en" || r.LangSource != index.LangSourceOverride || r.SourceKey != "en" || r.Original != "// 漢字だけ" {
			t.Fatalf("%+v", r)
		}
		if _, ok := r.Texts["zh-CN"]; ok {
			t.Fatalf("原文应从 zh-CN 键移走: %+v", r.Texts)
		}
	}
	check()
	if _, err := p.UpdateMappings(context.Background(), MapOptions{}); err != nil {
		t.Fatal(err)
	}
	check()
}

func TestStatusQuickLoadWithoutScan(t *testing.T) {
	p := newProject(t, map[string]string{"a.go": "package a\n\n// Hello\nfunc A() {}\n"})
	st, err := p.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Initialized || st.HasMappings || st.HasIndex || st.Comments != 0 {
		t.Fatalf("快速载入不应触发扫描: %+v", st)
	}
	if _, err := p.UpdateMappings(context.Background(), MapOptions{}); err != nil {
		t.Fatal(err)
	}
	st, _ = p.Status()
	if !st.HasIndex || st.Comments != 1 || st.Pending != 1 || st.Languages["unknown"] != 1 {
		t.Fatalf("%+v", st)
	}
}

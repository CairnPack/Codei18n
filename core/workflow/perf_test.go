package workflow

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/CairnPack/codei18n/adapters/translator"
	"github.com/CairnPack/codei18n/core/mapping"
)

// countingTranslator 统计实际发给后端的文本数与批次数。
type countingTranslator struct {
	texts   atomic.Int32
	batches atomic.Int32
}

func (c *countingTranslator) Translate(ctx context.Context, text, from, to string) (string, error) {
	c.texts.Add(1)
	return translator.NewMockTranslator().Translate(ctx, text, from, to)
}

func (c *countingTranslator) TranslateBatch(ctx context.Context, texts []string, from, to string) ([]string, error) {
	c.batches.Add(1)
	c.texts.Add(int32(len(texts)))
	return translator.NewMockTranslator().TranslateBatch(ctx, texts, from, to)
}

// 归一化后相同的原文只请求一次，结果写回所有对应 ID。
func TestTranslateDedupesIdenticalTexts(t *testing.T) {
	files := map[string]string{}
	for i := 0; i < 5; i++ {
		files[fmt.Sprintf("f%d.go", i)] = fmt.Sprintf("package f\n\n// Returns the value.\nfunc F%d() {}\n\n//   Returns   the value.\nfunc G%d() {}\n", i, i)
	}
	env := setupProject(t, files)
	ct := &countingTranslator{}
	env.Translator = ct
	if _, err := UpdateMappings(context.Background(), env, MapUpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	res, err := TranslateMissing(context.Background(), env, TranslateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.TotalTasks != 10 || res.SuccessCount != 10 || res.Requests != 1 || ct.texts.Load() != 1 {
		t.Fatalf("10 条同文注释应只请求 1 次: %+v texts=%d", res, ct.texts.Load())
	}
	store := mapping.NewStore(env.MappingsPath())
	store.Load()
	for _, id := range store.IDs() {
		if v, _ := store.Get(id, "zh-CN"); !strings.Contains(v, "MOCK en->zh-CN") {
			t.Fatalf("%s 未写回译文: %q", id, v)
		}
	}
}

// 保存节流：不再每批全量重写；结束时必定保存，最终内容完整。
func TestTranslateSaveThrottling(t *testing.T) {
	files := map[string]string{}
	for i := 0; i < 60; i++ {
		files[fmt.Sprintf("f%02d.go", i)] = fmt.Sprintf("package f\n\n// Comment number %d\nfunc F%d() {}\n", i, i)
	}
	env := setupProject(t, files)
	env.Translator = &countingTranslator{}
	if _, err := UpdateMappings(context.Background(), env, MapUpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	// BatchSize 默认为 10 → 6 批；每 3 批保存一次 → 后台最多 2 次 + 结束 1 次，远少于每批一次。
	res, err := TranslateMissing(context.Background(), env, TranslateOptions{Concurrency: 1, SaveEvery: 3})
	if err != nil {
		t.Fatal(err)
	}
	if res.Saves < 1 || res.Saves > 3 {
		t.Fatalf("保存次数应在 1..3 之间: %d", res.Saves)
	}
	store := mapping.NewStore(env.MappingsPath())
	store.Load()
	if n := PendingCount(store, "en", "zh-CN"); n != 0 {
		t.Fatalf("最终落盘应包含全部 60 条译文，仍缺 %d 条", n)
	}
}

// 无改动重扫不重写 mappings.json / index.json，避免 IDE 插件无谓重载；有改动时才写。
func TestNoChangeRescanDoesNotRewriteFiles(t *testing.T) {
	env := setupProject(t, map[string]string{"a.go": "package a\n\n// Hello\nfunc A() {}\n"})
	first, err := UpdateMappings(context.Background(), env, MapUpdateOptions{})
	if err != nil || !first.MappingsWritten || !first.IndexWritten {
		t.Fatalf("%+v %v", first, err)
	}
	st1, _ := os.Stat(env.MappingsPath())
	ix1, _ := os.Stat(env.IndexPath())
	second, err := UpdateMappings(context.Background(), env, MapUpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if second.MappingsWritten || second.IndexWritten || second.CachedFiles != 1 {
		t.Fatalf("无改动时不应写文件，且应命中缓存: %+v", second)
	}
	st2, _ := os.Stat(env.MappingsPath())
	ix2, _ := os.Stat(env.IndexPath())
	if !st1.ModTime().Equal(st2.ModTime()) || !ix1.ModTime().Equal(ix2.ModTime()) {
		t.Fatal("文件 mtime 不应变化")
	}
	os.WriteFile(env.Root+"/b.go", []byte("package a\n\n// World\nfunc B() {}\n"), 0o644)
	third, _ := UpdateMappings(context.Background(), env, MapUpdateOptions{})
	if !third.MappingsWritten || !third.IndexWritten || third.AddedCount != 1 {
		t.Fatalf("新增注释后应写文件: %+v", third)
	}
}

// 孤儿条目：默认只报告数量，--prune 才删除；扫描子目录时拒绝 prune。
func TestPruneOrphans(t *testing.T) {
	env := setupProject(t, map[string]string{
		"a.go":     "package a\n\n// Keep me\nfunc A() {}\n",
		"sub/b.go": "package sub\n\n// In sub\nfunc B() {}\n",
	})
	if _, err := UpdateMappings(context.Background(), env, MapUpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	store := mapping.NewStore(env.MappingsPath())
	store.Load()
	store.Set("orphan-1", "en", "// gone")
	store.Set("orphan-2", "en", "// gone too")
	store.Save()

	res, err := UpdateMappings(context.Background(), env, MapUpdateOptions{})
	if err != nil || res.Orphans != 2 || res.Pruned != 0 {
		t.Fatalf("默认不删除，只报告: %+v %v", res, err)
	}
	if _, err := UpdateMappings(context.Background(), env, MapUpdateOptions{ScanDir: env.Root + "/sub", Prune: true}); err == nil {
		t.Fatal("扫描子目录时应拒绝 prune")
	}
	res, err = UpdateMappings(context.Background(), env, MapUpdateOptions{Prune: true})
	if err != nil || res.Pruned != 2 || res.Orphans != 2 {
		t.Fatalf("%+v %v", res, err)
	}
	store = mapping.NewStore(env.MappingsPath())
	store.Load()
	if store.Has("orphan-1") || len(store.IDs()) != 2 {
		t.Fatalf("孤儿应被删除，其余保留: %v", store.IDs())
	}
}

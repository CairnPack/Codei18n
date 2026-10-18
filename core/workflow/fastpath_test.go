package workflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/index"
	"github.com/CairnPack/codei18n/core/mapping"
)

// 快速路径（无改动直接返回）只能在所有输入都没变时生效；任一输入变化都必须走完整路径。
func TestFastPathInvalidation(t *testing.T) {
	env := setupProject(t, map[string]string{"a.go": "package a\n\n// Hello\nfunc A() {}\n"})
	old := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(env.Root, "a.go"), old, old)
	run := func() *MapUpdateResult {
		t.Helper()
		r, err := UpdateMappings(context.Background(), env, MapUpdateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := run(); r.Unchanged {
		t.Fatal("首次不应走快速路径")
	}
	if r := run(); !r.Unchanged || r.TotalComments != 1 || r.Orphans != 0 {
		t.Fatalf("无改动应走快速路径: %+v", r)
	}

	// 1. mappings.json 被外部修改（例如翻译、手动编辑）
	store := mapping.NewStore(env.MappingsPath())
	store.Load()
	store.Set("orphan", "en", "// gone")
	time.Sleep(10 * time.Millisecond)
	store.Save()
	if r := run(); r.Unchanged || r.Orphans != 1 {
		t.Fatalf("mappings 变化后应重新计算: %+v", r)
	}
	if r := run(); !r.Unchanged || r.Orphans != 1 {
		t.Fatalf("快速路径应保留孤儿数: %+v", r)
	}

	// 2. 用户纠正原语言（overrides.json 变化）
	ov, _ := index.ReadOverrides(env.OverridesPath())
	idx, _ := index.Read(env.IndexPath())
	var id string
	for _, f := range idx.Files {
		id = f.Comments[0].ID
	}
	ov.Languages[id] = "ja"
	index.WriteOverrides(env.OverridesPath(), ov)
	if r := run(); r.Unchanged {
		t.Fatal("overrides 变化后应重新计算")
	}
	idx, _ = index.Read(env.IndexPath())
	for _, f := range idx.Files {
		if f.Comments[0].Lang != "ja" || f.Comments[0].LangSource != index.LangSourceOverride {
			t.Fatalf("纠正应写入索引: %+v", f.Comments[0])
		}
	}

	// 3. 语言配置变化
	run()
	env.Config = config.Resolve(nil, &config.File{LocalLanguage: config.Ptr("ja")}, config.Overrides{})
	if r := run(); r.Unchanged {
		t.Fatal("本地语言变化后应重新计算")
	}

	// 4. 源码变化
	run()
	os.WriteFile(filepath.Join(env.Root, "a.go"), []byte("package a\n\n// Hello there\nfunc A() {}\n"), 0o644)
	if r := run(); r.Unchanged || r.AddedCount != 1 {
		t.Fatalf("源码变化后应重新计算: %+v", r)
	}

	// 5. index.json 被删除
	run()
	os.Remove(env.IndexPath())
	if r := run(); r.Unchanged || !r.IndexWritten {
		t.Fatalf("index.json 被删除后应重新写出: %+v", r)
	}

	// 6. Prune 且存在孤儿时不能走快速路径
	run()
	if r, _ := UpdateMappings(context.Background(), env, MapUpdateOptions{Prune: true}); r.Unchanged || r.Pruned == 0 {
		t.Fatalf("prune 应执行: %+v", r)
	}
}

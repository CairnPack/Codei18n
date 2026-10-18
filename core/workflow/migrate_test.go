package workflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/domain"
	"github.com/CairnPack/codei18n/core/mapping"
	"github.com/CairnPack/codei18n/core/utils"
)

func setupProject(t *testing.T, files map[string]string) *Env {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		p := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &Env{Root: root, Config: config.Resolve(nil, nil, config.Overrides{})}
}

func legacyID(file, lang, symbol, text string) string {
	return utils.GenerateCommentID(&domain.Comment{File: file, Language: lang, Symbol: symbol, SourceText: text})
}

// 合并规则上线前逐行入库的译文，合并后按行连接继承到新 ID。
func TestMigrateFromLegacyPerLineIDs(t *testing.T) {
	env := setupProject(t, map[string]string{"a.go": "package a\n\n// First line\n//\n// Third line\nfunc A() {}\n"})
	store := mapping.NewStore(env.MappingsPath())
	sym := "func.A"
	store.Set(legacyID("a.go", "go", sym, "// First line"), "en", "// First line")
	store.Set(legacyID("a.go", "go", sym, "// First line"), "zh-CN", "// 第一行")
	store.Set(legacyID("a.go", "go", sym, "// Third line"), "en", "// Third line")
	store.Set(legacyID("a.go", "go", sym, "// Third line"), "zh-CN", "// 第三行")
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}

	res, err := UpdateMappings(context.Background(), env, MapUpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.MigratedCount != 1 {
		t.Fatalf("%+v", res)
	}
	after := mapping.NewStore(env.MappingsPath())
	after.Load()
	newID := legacyID("a.go", "go", sym, "// First line\n//\n// Third line")
	got, ok := after.Get(newID, "zh-CN")
	if !ok || got != "// 第一行\n//\n// 第三行" {
		t.Fatalf("合并后的译文应按行连接: %q", got)
	}
	if en, _ := after.Get(newID, "en"); en != "// First line\n//\n// Third line" {
		t.Fatalf("原文: %q", en)
	}
}

// 符号规则变化导致 ID 改变时，从唯一的同原文孤儿条目继承译文；有歧义时不迁移。
func TestMigrateByOrphanText(t *testing.T) {
	env := setupProject(t, map[string]string{"a.go": "package a\n\n// Unique text\nfunc A() {}\n\n// Dup text\nfunc B() {}\n"})
	store := mapping.NewStore(env.MappingsPath())
	store.Set("old-1", "en", "// Unique text")
	store.Set("old-1", "zh-CN", "// 唯一")
	store.Set("old-2", "en", "// Dup text")
	store.Set("old-2", "zh-CN", "// 重复甲")
	store.Set("old-3", "en", "// Dup text")
	store.Set("old-3", "zh-CN", "// 重复乙")
	store.Save()

	res, err := UpdateMappings(context.Background(), env, MapUpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.MigratedCount != 1 || res.AddedCount != 2 {
		t.Fatalf("%+v", res)
	}
	after := mapping.NewStore(env.MappingsPath())
	after.Load()
	if v, _ := after.Get(legacyID("a.go", "go", "func.A", "// Unique text"), "zh-CN"); v != "// 唯一" {
		t.Fatalf("应继承唯一孤儿的译文: %q", v)
	}
	if _, ok := after.Get(legacyID("a.go", "go", "func.B", "// Dup text"), "zh-CN"); ok {
		t.Fatal("多个候选时不应迁移")
	}
	if !after.Has("old-1") {
		t.Fatal("旧条目应保留，由 --prune 清理")
	}
}

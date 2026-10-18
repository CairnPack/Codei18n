package workflow

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/internal/benchgen"
)

// 工作流层基准：完整的 map update（扫描 + 合并映射 + 写 mappings.json 与 index.json）。
//
//	go test -run x -bench MapUpdate -benchtime 5x -cpu 8 ./core/workflow/
var (
	wfBenchOnce sync.Once
	wfBenchDir  string
)

func wfBenchProject(b *testing.B) string {
	wfBenchOnce.Do(func() {
		dir, err := os.MkdirTemp("", "codei18n-wfbench-*")
		if err != nil {
			b.Fatal(err)
		}
		if err := benchgen.Generate(dir, 2000, 1, benchgen.Languages); err != nil {
			b.Fatal(err)
		}
		old := time.Now().Add(-time.Hour)
		filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				os.Chtimes(p, old, old)
			}
			return nil
		})
		wfBenchDir = dir
	})
	return wfBenchDir
}

// BenchmarkMapUpdateFirst 首次 map update：没有映射、没有缓存。
func BenchmarkMapUpdateFirst(b *testing.B) {
	dir := wfBenchProject(b)
	env := &Env{Root: dir, Config: config.Resolve(nil, nil, config.Overrides{})}
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		os.RemoveAll(filepath.Join(dir, ".codei18n"))
		b.StartTimer()
		if _, err := UpdateMappings(context.Background(), env, MapUpdateOptions{Workers: 8}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMapUpdateNoChange 无改动时再次 map update（命中缓存、不写文件）。
func BenchmarkMapUpdateNoChange(b *testing.B) {
	dir := wfBenchProject(b)
	env := &Env{Root: dir, Config: config.Resolve(nil, nil, config.Overrides{})}
	os.RemoveAll(filepath.Join(dir, ".codei18n"))
	if _, err := UpdateMappings(context.Background(), env, MapUpdateOptions{Workers: 8}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := UpdateMappings(context.Background(), env, MapUpdateOptions{Workers: 8})
		if err != nil || res.MappingsWritten || res.IndexWritten {
			b.Fatalf("%v %+v", err, res)
		}
	}
}

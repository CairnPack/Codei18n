package scanner

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/CairnPack/codei18n/internal/benchgen"
)

// 基准使用约 2000 个文件的中型样例项目（internal/benchgen 确定性生成）。
// 设置 CODEI18N_BENCH_DIR 可以指定一个已生成的目录（go run ./scripts/genbench -out <dir>）。
//
//	go test -run x -bench . -benchtime 5x -cpu 8 ./core/scanner/
var (
	benchOnce sync.Once
	benchDir  string
)

func benchProject(b *testing.B) string {
	if d := os.Getenv("CODEI18N_BENCH_DIR"); d != "" {
		return d
	}
	benchOnce.Do(func() {
		dir, err := os.MkdirTemp("", "codei18n-bench-*")
		if err != nil {
			b.Fatal(err)
		}
		if err := genFiles(dir, 2000); err != nil {
			b.Fatal(err)
		}
		benchDir = dir
	})
	return benchDir
}

// genFiles 生成样例项目，并把 mtime 调到一小时前（真实项目中的文件不会恰好在缓存写入的同一时刻被修改）。
func genFiles(dir string, n int) error {
	if err := benchgen.Generate(dir, n, 1, benchgen.Languages); err != nil {
		return err
	}
	old := time.Now().Add(-time.Hour)
	return filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		return os.Chtimes(p, old, old)
	})
}

func benchScan(b *testing.B, workers int) {
	dir := benchProject(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, err := Scan(context.Background(), Options{Dir: dir, Workers: workers})
		if err != nil {
			b.Fatal(err)
		}
		b.ReportMetric(float64(len(r.Files)), "files")
	}
}

// BenchmarkScanFull_1Worker 首次全量扫描（单 worker，等价于旧版的顺序扫描 + Query 复用）。
func BenchmarkScanFull_1Worker(b *testing.B) { benchScan(b, 1) }

// BenchmarkScanFull_8Workers 首次全量扫描（8 个 worker）。
func BenchmarkScanFull_8Workers(b *testing.B) { benchScan(b, 8) }

// BenchmarkScanNoChange 无改动时的增量重扫（8 个 worker，含读写缓存文件）。
func BenchmarkScanNoChange(b *testing.B) {
	dir := benchProject(b)
	cachePath := filepath.Join(b.TempDir(), "scan.gob")
	warm := OpenCache(cachePath)
	if _, err := Scan(context.Background(), Options{Dir: dir, Workers: 8, Cache: warm}); err != nil {
		b.Fatal(err)
	}
	warm.Save()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := OpenCache(cachePath)
		if _, err := Scan(context.Background(), Options{Dir: dir, Workers: 8, Cache: c}); err != nil {
			b.Fatal(err)
		}
		if err := c.Save(); err != nil {
			b.Fatal(err)
		}
		b.ReportMetric(float64(c.Hits()), "cached")
	}
}

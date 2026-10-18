package scanner

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/CairnPack/codei18n/internal/benchgen"
)

func genProject(t testing.TB, files int) string {
	t.Helper()
	dir := t.TempDir()
	if err := benchgen.Generate(dir, files, 7, benchgen.Languages); err != nil {
		t.Fatal(err)
	}
	return dir
}

func ids(r *Result) []string {
	var out []string
	for _, c := range r.Comments() {
		out = append(out, c.File+"|"+c.ID)
	}
	return out
}

// 并行扫描与单 worker 扫描结果完全一致（顺序确定）。
func TestParallelMatchesSequential(t *testing.T) {
	dir := genProject(t, 120)
	seq, err := Scan(context.Background(), Options{Dir: dir, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	par, err := Scan(context.Background(), Options{Dir: dir, Workers: 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(seq.Files) != 120 || !reflect.DeepEqual(ids(seq), ids(par)) {
		t.Fatalf("并行结果与顺序结果不同: %d vs %d", len(ids(seq)), len(ids(par)))
	}
}

func TestScanCancel(t *testing.T) {
	dir := genProject(t, 200)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, Options{Dir: dir}); err == nil {
		t.Fatal("已取消的 ctx 应返回错误")
	}
}

func TestIncrementalCache(t *testing.T) {
	dir := genProject(t, 40)
	cachePath := filepath.Join(t.TempDir(), "scan.gob")
	scan := func() (*Result, *Cache) {
		t.Helper()
		c := OpenCache(cachePath)
		r, err := Scan(context.Background(), Options{Dir: dir, Cache: c})
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Save(); err != nil {
			t.Fatal(err)
		}
		return r, c
	}
	first, c1 := scan()
	if c1.Hits() != 0 {
		t.Fatal("首次扫描不应命中缓存")
	}
	// 让文件的 mtime 早于缓存写入时间 2 秒以上，排除"同周期改动"的保护逻辑。
	old := time.Now().Add(-time.Hour)
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			os.Chtimes(p, old, old)
		}
		return nil
	})
	scan() // 刷新缓存中的 mtime
	second, c2 := scan()
	if c2.Hits() != 40 || !reflect.DeepEqual(ids(first), ids(second)) {
		t.Fatalf("无变化时应全部命中缓存且结果一致: hits=%d", c2.Hits())
	}

	// 修改一个文件内容（保持大小与 mtime 不变）：hash 复核前 size+mtime 快筛会命中，
	// 因此这里改用不同长度的内容并更新 mtime，验证只重新解析这一个文件。
	var target string
	for _, f := range second.Files {
		if filepath.Ext(f.Path) == ".go" {
			target = filepath.Join(dir, filepath.FromSlash(f.Path))
			break
		}
	}
	data, _ := os.ReadFile(target)
	os.WriteFile(target, append(data, []byte("\n// Appended comment\n")...), 0o644)
	os.Chtimes(target, old.Add(time.Minute), old.Add(time.Minute))
	third, c3 := scan()
	if c3.Hits() != 39 {
		t.Fatalf("只应重新解析被修改的文件: hits=%d", c3.Hits())
	}
	if len(third.Comments()) != len(second.Comments())+1 {
		t.Fatal("新增注释应被发现")
	}

	// 内容相同但 mtime 变化（例如 touch）：hash 复核后复用缓存。
	os.Chtimes(target, old.Add(2*time.Minute), old.Add(2*time.Minute))
	_, c4 := scan()
	if c4.Hits() != 40 {
		t.Fatalf("内容未变只改 mtime 应复用缓存: hits=%d", c4.Hits())
	}

	// 删除文件后缓存中不再保留它。
	os.Remove(target)
	_, c5 := scan()
	if c5.Hits() != 39 || len(OpenCache(cachePath).loaded) != 39 {
		t.Fatalf("删除的文件应从缓存中消失: %d", len(OpenCache(cachePath).loaded))
	}
}

// 刚写入缓存后（同一时钟周期内）被修改、且大小不变的文件，必须重新计算 hash，不能只信 size+mtime。
func TestRacyCleanFileIsRehashed(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.go")
	os.WriteFile(p, []byte("package a\n\n// Alpha\nfunc A() {}\n"), 0o644)
	cachePath := filepath.Join(t.TempDir(), "scan.gob")
	c := OpenCache(cachePath)
	Scan(context.Background(), Options{Dir: dir, Cache: c})
	c.Save()
	st, _ := os.Stat(p)
	os.WriteFile(p, []byte("package a\n\n// Gamma\nfunc A() {}\n"), 0o644) // 同样长度
	os.Chtimes(p, st.ModTime(), st.ModTime())
	c2 := OpenCache(cachePath)
	r, _ := Scan(context.Background(), Options{Dir: dir, Cache: c2})
	if got := r.Comments()[0].SourceText; got != "// Gamma" {
		t.Fatalf("同周期内的修改被漏掉: %q", got)
	}
}

func TestCacheVersionMismatchIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scan.gob")
	os.WriteFile(path, []byte("garbage"), 0o644)
	if c := OpenCache(path); len(c.loaded) != 0 {
		t.Fatal("损坏的缓存应被丢弃")
	}
}

package scanner

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/CairnPack/codei18n/adapters"
	"github.com/CairnPack/codei18n/core"
	"github.com/CairnPack/codei18n/core/domain"
	"github.com/CairnPack/codei18n/core/events"
	"github.com/CairnPack/codei18n/core/index"
	"github.com/CairnPack/codei18n/core/logging"
	"github.com/CairnPack/codei18n/core/utils"
)

// Options 配置一次目录扫描。
type Options struct {
	// Dir 是扫描根目录；注释的 File 字段与 ID 中的路径相对于它。
	Dir string
	// Exclude 是 doublestar 排除规则（相对于 Dir，使用 /）。
	Exclude []string
	Logger  logging.Logger
	// Progress 接收 discover / scan 阶段的进度。
	Progress events.Func
	// Workers 并行解析的 worker 数，<=0 时为 runtime.NumCPU()。
	Workers int
	// Cache 非 nil 时复用未变化文件的上次解析结果（增量扫描），并记录本次结果；调用方负责 Save。
	Cache *Cache
	// CacheKeyPrefix 是缓存键的前缀（扫描目录相对于项目根目录的路径），使缓存键与扫描目录无关。
	CacheKeyPrefix string
}

// FileResult 是单个文件的扫描结果。
type FileResult struct {
	// Path 相对于扫描根目录，使用 /。
	Path        string
	Language    string
	ContentHash string
	Comments    []*domain.Comment
}

// Result 是一次扫描的结果。
type Result struct {
	Files []*FileResult
	// Errors 记录读取或解析失败的文件（相对路径 → 错误），这些文件被跳过。
	Errors map[string]error
}

// Comments 返回所有文件的注释（已生成 ID），按文件路径与位置排序。
func (r *Result) Comments() []*domain.Comment {
	var out []*domain.Comment
	for _, f := range r.Files {
		out = append(out, f.Comments...)
	}
	return out
}

// Scan 遍历目录并提取所有受支持文件中的注释。ctx 取消后尽快返回 ctx.Err()。
func Scan(ctx context.Context, opts Options) (*Result, error) {
	log := logging.OrNop(opts.Logger)
	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	files, err := discover(ctx, opts, workers)
	if err != nil {
		return nil, err
	}
	opts.Progress.Emit(events.Event{Phase: events.PhaseDiscover, Done: len(files), Total: len(files)})
	workers = min(workers, max(1, len(files)))

	results := make([]*FileResult, len(files))
	errs := make([]error, len(files))
	jobs := make(chan int)
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		done int
	)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 每个 worker 持有自己的 adapter：tree-sitter Parser 不是并发安全的。
			own := map[string]core.LanguageAdapter{}
			for i := range jobs {
				f := files[i]
				results[i], errs[i] = scanOne(opts, own, f)
				mu.Lock()
				done++
				e := events.Event{Phase: events.PhaseScan, Done: done, Total: len(files), File: f.rel}
				if errs[i] != nil {
					e.Err = errs[i].Error()
				}
				opts.Progress.Emit(e)
				mu.Unlock()
			}
		}()
	}
dispatch:
	for i := range files {
		select {
		case <-ctx.Done():
			break dispatch
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	res := &Result{Errors: map[string]error{}}
	for i, f := range files {
		if errs[i] != nil {
			log.Warnf("解析文件 %s 失败: %v", f.rel, errs[i])
			res.Errors[f.rel] = errs[i]
			continue
		}
		res.Files = append(res.Files, results[i])
	}
	return res, nil
}

// found 是遍历时发现的一个待扫描文件。size 与 mtime 来自目录项（Windows 上无需额外 stat）。
type found struct {
	rel   string
	size  int64
	mtime int64
}

// scanOne 解析单个文件：缓存命中（size+mtime 相同，或内容 hash 相同）时复用上次结果。
func scanOne(opts Options, own map[string]core.LanguageAdapter, f found) (*FileResult, error) {
	rel := f.rel
	key := opts.CacheKeyPrefix + rel
	if opts.Cache != nil {
		if e, trust := opts.Cache.lookup(key, rel, f.size, f.mtime); e != nil && trust {
			if fr, err := e.result(rel); err == nil {
				opts.Cache.hit(key, e, f.size, f.mtime)
				return fr, nil
			}
		}
	}
	content, err := os.ReadFile(filepath.Join(opts.Dir, filepath.FromSlash(rel)))
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}
	if opts.Cache != nil {
		if e, _ := opts.Cache.lookup(key, rel, f.size, f.mtime); e != nil && e.ContentHash == index.ContentHash(content) {
			if fr, err := e.result(rel); err == nil {
				opts.Cache.hit(key, e, f.size, f.mtime)
				return fr, nil
			}
		}
	}
	ext := strings.ToLower(filepath.Ext(rel))
	adapter := own[ext]
	if adapter == nil {
		if adapter, err = adapters.GetAdapter(rel); err != nil {
			return nil, err
		}
		own[ext] = adapter
	}
	fr, err := parseWith(adapter, rel, content)
	if err != nil {
		return nil, err
	}
	if opts.Cache != nil {
		opts.Cache.store(key, rel, f.size, f.mtime, fr)
	}
	return fr, nil
}

// discover 并发遍历 Dir，返回所有受支持且未被排除的文件（按相对路径排序）。
func discover(ctx context.Context, opts Options, workers int) ([]found, error) {
	var (
		mu       sync.Mutex
		files    []found
		firstErr error
		pending  sync.WaitGroup
	)
	dirs := make(chan string, 1024)
	fail := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
	}
	visit := func(relDir string) {
		defer pending.Done()
		if ctx.Err() != nil {
			fail(ctx.Err())
			return
		}
		entries, err := os.ReadDir(filepath.Join(opts.Dir, filepath.FromSlash(relDir)))
		if err != nil {
			fail(err)
			return
		}
		var local []found
		for _, d := range entries {
			rel := d.Name()
			if relDir != "." {
				rel = relDir + "/" + d.Name()
			}
			if Excluded(rel, opts.Exclude) {
				continue
			}
			if d.IsDir() {
				if !skipDir(d.Name()) {
					pending.Add(1)
					select {
					case dirs <- rel:
					default:
						go func() { dirs <- rel }() // 队列满时不阻塞当前 worker，避免死锁
					}
				}
				continue
			}
			if !d.Type().IsRegular() || !adapters.Supported(d.Name()) {
				continue
			}
			info, err := d.Info()
			if err != nil {
				fail(err)
				continue
			}
			local = append(local, found{rel: rel, size: info.Size(), mtime: info.ModTime().UnixNano()})
		}
		mu.Lock()
		files = append(files, local...)
		mu.Unlock()
	}

	if _, err := os.Stat(opts.Dir); err != nil {
		return nil, err
	}
	pending.Add(1)
	dirs <- "."
	allDone := make(chan struct{})
	go func() { pending.Wait(); close(allDone) }()
	var wg sync.WaitGroup
	for w := 0; w < max(1, workers); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case d := <-dirs:
					visit(d)
				case <-allDone:
					return
				}
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	return files, nil
}

// Excluded 判断相对路径是否命中任一排除规则。
func Excluded(relPath string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, _ := doublestar.Match(pattern, relPath); matched {
			return true
		}
	}
	return false
}

func skipDir(name string) bool {
	return name == ".git" || name == "vendor" || name == ".codei18n"
}

// ScanFile 读取并解析单个文件。rel 是写入注释与 ID 的相对路径。
func ScanFile(path, rel string) (*FileResult, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}
	return ParseContent(rel, content)
}

// ParseContent 解析内存中的文件内容，生成 ID 与索引坐标。
func ParseContent(rel string, content []byte) (*FileResult, error) {
	adapter, err := adapters.GetAdapter(rel)
	if err != nil {
		return nil, err
	}
	return parseWith(adapter, rel, content)
}

// parseWith 用给定的 adapter 解析内容，生成 ID 与索引坐标。
func parseWith(adapter core.LanguageAdapter, rel string, content []byte) (*FileResult, error) {
	comments, err := adapter.Parse(rel, content)
	if err != nil {
		return nil, err
	}
	lines := index.Lines(content)
	for _, c := range comments {
		if c.ID == "" {
			c.ID = utils.GenerateCommentID(c)
		}
		r := index.RangeFromBytes(lines, c.Range.StartLine, c.Range.StartCol, c.Range.EndLine, c.Range.EndCol)
		c.IndexRange = domain.TextRange{StartLine: r.StartLine, StartCol: r.StartCol, EndLine: r.EndLine, EndCol: r.EndCol}
		c.TextHash = index.TextHash(c.SourceText)
	}
	return &FileResult{
		Path:        rel,
		Language:    adapters.LanguageID(rel),
		ContentHash: index.ContentHash(content),
		Comments:    comments,
	}, nil
}

// FromStdin 从标准输入读取文件内容并扫描。
func FromStdin(filename string) ([]*domain.Comment, error) {
	src, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("读取 stdin 失败: %w", err)
	}
	adapter, err := adapters.GetAdapter(filename)
	if err != nil {
		return nil, err
	}
	return adapter.Parse(filename, src)
}

// SingleFile 扫描单个文件。
func SingleFile(filename string) ([]*domain.Comment, error) {
	adapter, err := adapters.GetAdapter(filename)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read file failed: %w", err)
	}
	return adapter.Parse(filename, content)
}

// Directory 递归扫描目录。
//
// Deprecated: 使用 Scan，它支持取消、进度与日志注入。
func Directory(dir string, excludePatterns ...string) ([]*domain.Comment, error) {
	res, err := Scan(context.Background(), Options{Dir: dir, Exclude: excludePatterns})
	if err != nil {
		return nil, err
	}
	return res.Comments(), nil
}

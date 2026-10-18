package scanner

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/CairnPack/codei18n/adapters"
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
	files, err := discover(ctx, opts)
	if err != nil {
		return nil, err
	}
	opts.Progress.Emit(events.Event{Phase: events.PhaseDiscover, Done: len(files), Total: len(files)})

	res := &Result{Errors: map[string]error{}}
	for i, rel := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		fr, err := ScanFile(filepath.Join(opts.Dir, filepath.FromSlash(rel)), rel)
		if err != nil {
			log.Warnf("解析文件 %s 失败: %v", rel, err)
			res.Errors[rel] = err
			opts.Progress.Emit(events.Event{Phase: events.PhaseScan, Done: i + 1, Total: len(files), File: rel, Err: err.Error()})
			continue
		}
		res.Files = append(res.Files, fr)
		opts.Progress.Emit(events.Event{Phase: events.PhaseScan, Done: i + 1, Total: len(files), File: rel})
	}
	return res, nil
}

// discover 枚举 Dir 下所有受支持且未被排除的文件（相对路径，排序后返回）。
func discover(ctx context.Context, opts Options) ([]string, error) {
	var files []string
	err := filepath.WalkDir(opts.Dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relPath, err := filepath.Rel(opts.Dir, path)
		if err != nil {
			relPath = path
		}
		relPath = filepath.ToSlash(relPath)
		if Excluded(relPath, opts.Exclude) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if relPath != "." && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if adapters.Supported(path) {
			files = append(files, relPath)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
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

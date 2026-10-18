package workflow

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/domain"
	"github.com/CairnPack/codei18n/core/events"
	"github.com/CairnPack/codei18n/core/index"
	"github.com/CairnPack/codei18n/core/langcode"
	"github.com/CairnPack/codei18n/core/langdetect"
	"github.com/CairnPack/codei18n/core/mapping"
	"github.com/CairnPack/codei18n/core/scanner"
	"github.com/CairnPack/codei18n/core/utils"
)

// MapUpdateOptions 配置一次 map update。
type MapUpdateOptions struct {
	// ScanDir 是扫描目录，默认为项目根目录。注释 ID 中的路径相对于它（沿用旧规则）。
	ScanDir string
	// DryRun 只计算，不写任何文件。
	DryRun bool
	// Detector 识别注释原语言；nil 时使用默认识别器。
	Detector langdetect.Detector
}

// MapUpdateResult 是 map update 的结果。
type MapUpdateResult struct {
	TotalComments int
	AddedCount    int
	Files         int
	StorePath     string
	IndexPath     string
	// FileErrors 记录解析失败而被跳过的文件。
	FileErrors map[string]string
}

// UpdateMappings 扫描源码，把新注释加入 mappings.json，并原子写出 index.json。
func UpdateMappings(ctx context.Context, env *Env, opts MapUpdateOptions) (*MapUpdateResult, error) {
	log := env.log()
	scanDir := opts.ScanDir
	if scanDir == "" {
		scanDir = env.Root
	}
	detector := opts.Detector
	if detector == nil {
		detector = defaultDetector()
	}
	cfg := env.Config

	log.Infof("正在扫描目录: %s", scanDir)
	scan, err := scanner.Scan(ctx, scanner.Options{
		Dir:      scanDir,
		Exclude:  cfg.ExcludePatterns.Value,
		Logger:   env.Logger,
		Progress: env.Progress,
	})
	if err != nil {
		return nil, fmt.Errorf("扫描失败: %w", err)
	}

	store, err := env.loadStore()
	if err != nil {
		return nil, fmt.Errorf("加载映射文件失败: %w", err)
	}
	overrides, err := index.ReadOverrides(env.OverridesPath())
	if err != nil {
		return nil, err
	}
	store.SetLanguages(cfg.SourceLanguage.Value, cfg.LocalLanguage.Value)

	prefix, inRoot := relPrefix(env.Root, scanDir)
	if !inRoot {
		log.Warnf("扫描目录 %s 不在项目根目录 %s 内，跳过 index.json", scanDir, env.Root)
	}
	idx := index.New()
	idx.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	idx.Generator = env.Generator
	idx.SourceLanguage = cfg.SourceLanguage.Value
	idx.LocalLanguage = cfg.LocalLanguage.Value

	result := &MapUpdateResult{
		StorePath:  env.MappingsPath(),
		IndexPath:  env.IndexPath(),
		Files:      len(scan.Files),
		FileErrors: map[string]string{},
	}
	for rel, e := range scan.Errors {
		result.FileErrors[rel] = e.Error()
	}

	total := len(scan.Comments())
	done := 0
	for _, f := range scan.Files {
		entry := &index.File{ContentHash: f.ContentHash, Language: f.Language}
		for _, c := range f.Comments {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			lang := resolveLang(c, detector, overrides)
			key := StorageKey(cfg, lang.Lang)
			if !store.Has(c.ID) {
				store.Set(c.ID, key, c.SourceText)
				result.AddedCount++
				if key != cfg.SourceLanguage.Value {
					log.Debugf("检测到 %s 注释: ID=%s", lang.Lang, c.ID)
				}
			} else {
				key = existingSourceKey(store, c, key)
			}
			entry.Comments = append(entry.Comments, index.Comment{
				ID:         c.ID,
				Range:      index.Range(c.IndexRange),
				Type:       string(c.Type),
				Lang:       lang.Lang,
				LangSource: lang.Source,
				SourceKey:  key,
				TextHash:   index.TextHash(c.SourceText),
				Parts:      c.PartCount(),
				Symbol:     c.Symbol,
			})
			done++
		}
		if len(entry.Comments) > 0 {
			idx.Files[prefix+f.Path] = entry
		}
		env.Progress.Emit(events.Event{Phase: events.PhaseMap, Done: done, Total: total, File: f.Path})
	}
	result.TotalComments = total

	if opts.DryRun {
		return result, nil
	}
	env.Progress.Emit(events.Event{Phase: events.PhaseSave, Done: 0, Total: 1})
	if err := store.Save(); err != nil {
		return nil, fmt.Errorf("保存映射文件失败: %w", err)
	}
	if inRoot {
		if err := index.Write(env.IndexPath(), idx); err != nil {
			return nil, fmt.Errorf("保存位置索引失败: %w", err)
		}
	}
	env.Progress.Emit(events.Event{Phase: events.PhaseDone, Done: 1, Total: 1})
	return result, nil
}

func defaultDetector() langdetect.Detector { return langdetect.Legacy{} }

// resolveLang 返回注释的原语言：用户纠正优先，其次是识别结果。
func resolveLang(c *domain.Comment, d langdetect.Detector, o *index.Overrides) langdetect.Result {
	if lang, ok := o.Languages[c.ID]; ok && lang != "" {
		return langdetect.Result{Lang: langcode.Canonical(lang), Source: index.LangSourceOverride}
	}
	return d.Detect(c.SourceText)
}

// StorageKey 决定原语言为 lang 的注释原文在 mappings.json 中使用的语言键：
// 与本地语言等价时用本地语言的配置写法，否则（含无法识别）使用源语言的配置写法，与旧版行为一致。
func StorageKey(cfg *config.Effective, lang string) string {
	local := cfg.LocalLanguage.Value
	if langcode.Equivalent(lang, local) {
		return local
	}
	return cfg.SourceLanguage.Value
}

// existingSourceKey 对已存在的条目，找出原文实际所在的键（文本与源码一致者），优先使用规则给出的键。
func existingSourceKey(store *mapping.Store, c *domain.Comment, preferred string) string {
	langs := store.Languages(c.ID)
	want := utils.NormalizeCommentText(c.SourceText)
	if v, ok := langs[preferred]; ok && utils.NormalizeCommentText(v) == want {
		return preferred
	}
	for k, v := range langs {
		if utils.NormalizeCommentText(v) == want {
			return k
		}
	}
	return preferred
}

// relPrefix 返回 scanDir 相对于 root 的前缀（以 / 结尾，或为空），以及 scanDir 是否在 root 内。
func relPrefix(root, scanDir string) (string, bool) {
	absRoot, err1 := filepath.Abs(root)
	absScan, err2 := filepath.Abs(scanDir)
	if err1 != nil || err2 != nil {
		return "", false
	}
	rel, err := filepath.Rel(absRoot, absScan)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return "", true
	}
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel + "/", true
}

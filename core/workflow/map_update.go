package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	"github.com/CairnPack/codei18n/internal/atomicfile"
)

// MapUpdateOptions 配置一次 map update。
type MapUpdateOptions struct {
	// ScanDir 是扫描目录，默认为项目根目录。注释 ID 中的路径相对于它（沿用旧规则）。
	ScanDir string
	// DryRun 只计算，不写任何文件。
	DryRun bool
	// Detector 识别注释原语言；nil 时使用默认识别器。
	Detector langdetect.Detector
	// Prune 删除源码中已不存在的条目（孤儿）。只在扫描整个项目根目录时允许。
	Prune bool
	// NoCache 禁用增量缓存（每个文件都重新解析、每条注释都重新识别语言）。
	NoCache bool
	// Workers 并行解析的 worker 数，<=0 时为 CPU 核数。
	Workers int
}

// MapUpdateResult 是 map update 的结果。
type MapUpdateResult struct {
	TotalComments int
	AddedCount    int
	// MigratedCount 是新增条目中，从旧 ID（合并前的逐行 ID，或符号规则变化前的 ID）迁移了译文的条数。
	MigratedCount int
	Files         int
	StorePath     string
	IndexPath     string
	// FileErrors 记录解析失败而被跳过的文件。
	FileErrors map[string]string
	// Orphans 是源码中已不存在、可以用 Prune 清理的条目数；扫描子目录时无法判断，为 -1。
	Orphans int
	// Pruned 是本次删除的孤儿条目数。
	Pruned int
	// CachedFiles 是复用增量缓存（未重新解析）的文件数。
	CachedFiles int
	// MappingsWritten / IndexWritten 表示是否写了文件（内容无变化时不写，避免 IDE 无谓重载）。
	MappingsWritten bool
	IndexWritten    bool
	// Unchanged 表示命中了"无改动"快速路径：扫描全部命中缓存且输入文件均未变化，没有重新计算。
	Unchanged bool
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
		detector = defaultDetector(env)
	}
	cfg := env.Config

	prefix, inRoot := relPrefix(env.Root, scanDir)
	wholeRoot := inRoot && prefix == ""
	if opts.Prune && !wholeRoot {
		return nil, fmt.Errorf("--prune 只能在扫描整个项目根目录时使用（当前扫描目录: %s）", scanDir)
	}
	useCache := !opts.NoCache && inRoot
	var cache *scanner.Cache
	if useCache {
		cache = scanner.OpenCache(filepath.Join(env.CacheDir(), ScanCacheFileName))
	}

	log.Infof("正在扫描目录: %s", scanDir)
	scan, err := scanner.Scan(ctx, scanner.Options{
		Dir:            scanDir,
		Exclude:        cfg.ExcludePatterns.Value,
		Logger:         env.Logger,
		Progress:       env.Progress,
		Workers:        opts.Workers,
		Cache:          cache,
		CacheKeyPrefix: prefix,
	})
	if err != nil {
		return nil, fmt.Errorf("扫描失败: %w", err)
	}

	result := &MapUpdateResult{
		StorePath:  env.MappingsPath(),
		IndexPath:  env.IndexPath(),
		Files:      len(scan.Files),
		FileErrors: map[string]string{},
		Orphans:    -1,
	}
	for rel, e := range scan.Errors {
		result.FileErrors[rel] = e.Error()
	}
	if cache != nil {
		result.CachedFiles = cache.Hits()
	}

	// 快速路径：全部命中缓存，且映射、索引、纠正记录、语言设置、识别器都与上次一致 → 结果必然相同。
	digest := scanDigest(prefix, scan)
	var state *mapState
	if useCache {
		state = readMapState(env.CacheDir())
	}
	if state != nil && cache.Hits() == len(scan.Files) && len(scan.Errors) == 0 &&
		state.Prefix == prefix && state.ScanDigest == digest && state.Detector != "" &&
		state.Detector == detectorKey(detector) &&
		state.Source == cfg.SourceLanguage.Value && state.Local == cfg.LocalLanguage.Value &&
		state.Mappings == statOf(env.MappingsPath()) && state.Index == statOf(env.IndexPath()) &&
		state.Overrides == statOf(env.OverridesPath()) && !(opts.Prune && state.Orphans > 0) {
		result.TotalComments, result.Unchanged = state.Total, true
		if wholeRoot {
			result.Orphans = state.Orphans
		}
		env.Progress.Emit(events.Event{Phase: events.PhaseDone, Done: 1, Total: 1})
		return result, nil
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
	if !inRoot {
		log.Warnf("扫描目录 %s 不在项目根目录 %s 内，跳过 index.json", scanDir, env.Root)
	}
	cacheDir := ""
	if useCache {
		cacheDir = env.CacheDir()
	}
	langs := openLangCache(cacheDir, detector)

	idx := index.New()
	idx.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	idx.Generator = env.Generator
	idx.SourceLanguage = cfg.SourceLanguage.Value
	idx.LocalLanguage = cfg.LocalLanguage.Value

	all := scan.Comments()
	for _, c := range all {
		if c.TextHash == "" {
			c.TextHash = index.TextHash(c.SourceText)
		}
	}
	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	langs.prefetch(detector, all, func(c *domain.Comment) bool { return overrides.Languages[c.ID] != "" }, workers)
	var mig *migrator // 只有出现新 ID 时才构建孤儿表
	total, done := len(all), 0
	for _, f := range scan.Files {
		entry := &index.File{ContentHash: f.ContentHash, Language: f.Language, Comments: make([]index.Comment, 0, len(f.Comments))}
		for _, c := range f.Comments {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if c.TextHash == "" {
				c.TextHash = index.TextHash(c.SourceText)
			}
			lang := resolveLang(c, detector, overrides, langs)
			key := StorageKey(cfg, lang.Lang)
			if !store.Has(c.ID) {
				if mig == nil {
					mig = newMigrator(store, all)
				}
				if mig.migrate(c) {
					result.MigratedCount++
				}
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
				TextHash:   c.TextHash,
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

	// 孤儿：mappings 中有、本次扫描中没有的 ID。只有扫描整个根目录时才能判断。
	if wholeRoot {
		seen := make(map[string]bool, total)
		for _, c := range all {
			seen[c.ID] = true
		}
		var orphans []string
		for _, id := range store.IDs() {
			if !seen[id] {
				orphans = append(orphans, id)
			}
		}
		result.Orphans = len(orphans)
		if opts.Prune {
			for _, id := range orphans {
				store.Delete(id)
				delete(overrides.Languages, id)
			}
			result.Pruned = len(orphans)
		}
	}

	if opts.DryRun {
		return result, nil
	}
	env.Progress.Emit(events.Event{Phase: events.PhaseSave, Done: 0, Total: 1})
	if store.Dirty() || !statOf(env.MappingsPath()).Exists {
		if err := store.Save(); err != nil {
			return nil, fmt.Errorf("保存映射文件失败: %w", err)
		}
		result.MappingsWritten = true
	}
	if result.Pruned > 0 {
		if err := index.WriteOverrides(env.OverridesPath(), overrides); err != nil {
			return nil, err
		}
	}
	idxDigest := indexDigest(idx)
	if inRoot {
		unchanged := state != nil && state.IndexDigest == idxDigest && state.Index == statOf(env.IndexPath()) && state.Index.Exists
		if !unchanged {
			if err := index.Write(env.IndexPath(), idx); err != nil {
				return nil, fmt.Errorf("保存位置索引失败: %w", err)
			}
			result.IndexWritten = true
		}
	}
	if useCache {
		if err := cache.Save(); err != nil {
			log.Warnf("保存扫描缓存失败: %v", err)
		}
		if err := langs.save(); err != nil {
			log.Warnf("保存语种识别缓存失败: %v", err)
		}
		ensureCacheIgnored(env.CacheDir())
		newState := &mapState{
			Prefix: prefix, ScanDigest: digest, IndexDigest: idxDigest,
			Source: cfg.SourceLanguage.Value, Local: cfg.LocalLanguage.Value, Detector: detectorKey(detector),
			Mappings: statOf(env.MappingsPath()), Index: statOf(env.IndexPath()), Overrides: statOf(env.OverridesPath()),
			Total: total, Orphans: max(result.Orphans-result.Pruned, 0),
		}
		if len(scan.Errors) > 0 {
			newState.ScanDigest = "" // 有解析失败的文件时不走快速路径，下次重新报告错误
		}
		if err := writeMapState(env.CacheDir(), newState); err != nil {
			log.Warnf("保存映射状态失败: %v", err)
		}
	}
	env.Progress.Emit(events.Event{Phase: events.PhaseDone, Done: 1, Total: 1})
	return result, nil
}

// ensureCacheIgnored 在缓存目录中放置内容为 "*" 的 .gitignore：即使用户选择把 .codei18n 入库，缓存也不会被提交。
func ensureCacheIgnored(dir string) {
	path := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(path); err == nil {
		return
	}
	_ = atomicfile.WriteFile(path, []byte("# CodeI18n 增量扫描缓存，可随时删除\n*\n"), 0o644)
}

// defaultDetector 按配置的候选语言构建多语种识别器；候选集合不受支持时警告并退回旧规则。
func defaultDetector(env *Env) langdetect.Detector {
	d, err := langdetect.New(env.Config.DetectLanguages.Value)
	if err != nil {
		env.log().Warnf("语种识别候选集合无效，退回旧规则（含汉字视为中文）: %v", err)
		return langdetect.Legacy{}
	}
	return d
}

// resolveLang 返回注释的原语言：用户纠正优先，其次是识别结果（按文本 hash 缓存）。
func resolveLang(c *domain.Comment, d langdetect.Detector, o *index.Overrides, cache *langCache) langdetect.Result {
	if lang, ok := o.Languages[c.ID]; ok && lang != "" {
		return langdetect.Result{Lang: langcode.Canonical(lang), Source: index.LangSourceOverride}
	}
	return cache.detect(d, c.TextHash, c.SourceText)
}

// StorageKey 决定原语言为 lang 的注释原文在 mappings.json 中使用的语言键：
// 与本地语言 / 源语言等价时用配置里的写法（例如 zh-CN），保持老插件兼容；无法识别时用源语言写法（旧行为）；
// 其他语言（例如 ja）用规范化语言码。
func StorageKey(cfg *config.Effective, lang string) string {
	local, source := cfg.LocalLanguage.Value, cfg.SourceLanguage.Value
	switch {
	case langcode.Equivalent(lang, local):
		return local
	case langcode.Equivalent(lang, source), lang == "", lang == langcode.Unknown:
		return source
	default:
		return langcode.Canonical(lang)
	}
}

// existingSourceKey 对已存在的条目，找出原文实际所在的键（文本与源码一致者），优先使用规则给出的键。
func existingSourceKey(store *mapping.Store, c *domain.Comment, preferred string) string {
	if v, ok := store.Get(c.ID, preferred); ok && v == c.SourceText {
		return preferred // 常见情况：原文原样存放在规则键下
	}
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

package workflow

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CairnPack/codei18n/adapters"
	"github.com/CairnPack/codei18n/core"
	"github.com/CairnPack/codei18n/core/domain"
	"github.com/CairnPack/codei18n/core/events"
	"github.com/CairnPack/codei18n/core/mapping"
	"github.com/CairnPack/codei18n/core/scanner"
	"github.com/CairnPack/codei18n/core/utils"
	"github.com/CairnPack/codei18n/internal/atomicfile"
)

// ConvertOptions 配置源码注释改写。
type ConvertOptions struct {
	// File 指定单个文件；为空时遍历 Dir。
	File string
	// Dir 是遍历目录，默认 "."。相对路径按旧版行为参与 ID 计算。
	Dir string
	// To 是目标语言（源语言 = 还原，本地语言 = 应用翻译）。
	To string
	// DryRun 只计算，不写文件。
	DryRun bool
}

// ConvertResult 是改写结果。
type ConvertResult struct {
	// Files 是处理过的文件。
	Files []string
	// Changed 是内容发生变化（或 DryRun 下将会变化）的文件。
	Changed []string
	// Missing 是找不到目标语言文本的注释数。
	Missing int
}

// Convert 在源码中原地改写注释为目标语言。所有文件写入均为原子写。
func Convert(ctx context.Context, env *Env, opts ConvertOptions) (*ConvertResult, error) {
	log := env.log()
	if opts.To == "" {
		return nil, fmt.Errorf("必须指定目标语言")
	}
	if opts.Dir == "" {
		opts.Dir = "."
	}
	store, err := env.loadStore()
	if err != nil {
		return nil, fmt.Errorf("加载映射文件失败: %w", err)
	}
	log.Debugf("Loaded store with %d comments", len(store.GetMapping().Comments))

	files, err := convertTargets(ctx, env, opts)
	if err != nil {
		return nil, err
	}
	log.Infof("准备处理 %d 个文件...", len(files))

	res := &ConvertResult{}
	for i, f := range files {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		adapter, err := adapters.GetAdapter(f.path)
		if err != nil {
			log.Warnf("无法获取适配器 %s: %v", f.path, err)
			continue
		}
		changed, missing := convertFile(env, f, adapter, store, opts)
		res.Files = append(res.Files, f.path)
		if changed {
			res.Changed = append(res.Changed, f.path)
		}
		res.Missing += missing
		env.Progress.Emit(events.Event{Phase: events.PhaseConvert, Done: i + 1, Total: len(files), File: f.path})
	}

	if err := store.Save(); err != nil {
		log.Warnf("保存映射文件失败: %v", err)
	} else {
		log.Debugf("Mapping store updated.")
	}
	return res, nil
}

type convertTarget struct {
	path string // 读写用的路径
	id   string // 参与 ID 计算的路径
}

func convertTargets(ctx context.Context, env *Env, opts ConvertOptions) ([]convertTarget, error) {
	log := env.log()
	if opts.File != "" {
		if !adapters.Supported(opts.File) {
			log.Warnf("跳过不支持的文件: %s", opts.File)
			return nil, nil
		}
		return []convertTarget{{path: opts.File, id: opts.File}}, nil
	}
	var out []convertTarget
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
		if scanner.Excluded(relPath, env.Config.ExcludePatterns.Value) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if relPath != "." && (name == ".git" || name == "vendor" || name == ".codei18n") {
				return filepath.SkipDir
			}
			return nil
		}
		if adapters.Supported(path) {
			id := path // 旧版行为：相对目录时用遍历得到的路径
			if filepath.IsAbs(opts.Dir) {
				id = relPath
			}
			out = append(out, convertTarget{path: path, id: id})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("扫描目录失败: %w", err)
	}
	return out, nil
}

// convertFile 改写单个文件，返回是否有变化以及缺失翻译的注释数。
func convertFile(env *Env, f convertTarget, adapter core.LanguageAdapter, store *mapping.Store, opts ConvertOptions) (bool, int) {
	log := env.log()
	source, local := env.Config.SourceLanguage.Value, env.Config.LocalLanguage.Value
	src, err := os.ReadFile(f.path)
	if err != nil {
		log.Warnf("读取文件 %s 失败: %v", f.path, err)
		return false, 0
	}
	comments, err := adapter.Parse(f.id, src)
	if err != nil {
		log.Warnf("解析文件 %s 失败: %v", f.path, err)
		return false, 0
	}
	log.Debugf("Convert: To='%s', Source='%s', Local='%s'", opts.To, source, local)

	type replacement struct {
		startOffset, endOffset int
		newText                string
	}
	var replacements []replacement
	missing := map[string]bool{}

	lines := strings.Split(string(src), "\n")
	lineOffsets := make([]int, len(lines))
	offset := 0
	for i, line := range lines {
		lineOffsets[i] = offset
		offset += len(line) + 1
	}

	for _, c := range comments {
		if c.ID == "" {
			c.ID = utils.GenerateCommentID(c)
		}
		var targetText string
		var found bool
		normalizedCurrent := utils.NormalizeCommentText(c.SourceText)

		if opts.To == source {
			// 还原模式：先看当前文本是否已经是源语言
			if transMap, exists := store.GetMapping().Comments[c.ID]; exists {
				if enText, hasEn := transMap[source]; hasEn && utils.NormalizeCommentText(enText) == normalizedCurrent {
					targetText, found = enText, true
				}
			}
			if !found {
				// 反查：当前文本是本地语言译文
				for id, transMap := range store.Snapshot().Comments {
					zhText, hasZh := transMap[local]
					enText, hasEn := transMap[source]
					if !hasZh || !hasEn || utils.NormalizeCommentText(zhText) != normalizedCurrent {
						continue
					}
					targetText, found = enText, true
					log.Debugf("Found by reverse lookup: ID=%s", id)
					// 还原为源语言后，为新的源语言注释生成 ID，并把双语数据迁移过去
					tempC := *c
					tempC.SourceText = enText
					if newID := utils.GenerateCommentID(&tempC); newID != id {
						store.Set(newID, source, enText)
						store.Set(newID, local, zhText)
						store.Delete(id)
					}
					break
				}
			}
			if !found {
				log.Warnf("未找到注释的英文翻译: '%s'", normalizedCurrent)
				missing[c.ID] = true
			}
		} else if val, ok := store.Get(c.ID, opts.To); ok && val != "" {
			targetText, found = val, true
		}

		if !found || utils.NormalizeCommentText(targetText) == normalizedCurrent {
			continue
		}
		startLineIdx, endLineIdx := c.Range.StartLine-1, c.Range.EndLine-1
		if startLineIdx < 0 || endLineIdx >= len(lineOffsets) {
			continue
		}
		replacements = append(replacements, replacement{
			startOffset: lineOffsets[startLineIdx] + c.Range.StartCol - 1,
			endOffset:   lineOffsets[endLineIdx] + c.Range.EndCol - 1,
			newText:     withMarkers(c, targetText, indentOf(lines[startLineIdx], c.Range.StartCol)),
		})
	}

	sort.Slice(replacements, func(i, j int) bool { return replacements[i].startOffset > replacements[j].startOffset })
	newSrc := string(src)
	for _, r := range replacements {
		if r.startOffset < 0 || r.endOffset > len(newSrc) || r.startOffset > r.endOffset {
			continue
		}
		newSrc = newSrc[:r.startOffset] + r.newText + newSrc[r.endOffset:]
	}
	changed := newSrc != string(src)
	if opts.DryRun {
		return changed, 0
	}
	if changed {
		mode := os.FileMode(0o644)
		if st, err := os.Stat(f.path); err == nil {
			mode = st.Mode().Perm()
		}
		if err := atomicfile.WriteFile(f.path, []byte(newSrc), mode); err != nil {
			log.Warnf("写入文件 %s 失败: %v", f.path, err)
			return false, len(missing)
		}
		log.Infof("已处理 %s", f.path)
	}
	return changed, len(missing)
}

// indentOf 返回注释所在行、注释之前的缩进（只在其为纯空白时）。
func indentOf(line string, startCol int) string {
	if startCol-1 > len(line) {
		return ""
	}
	prefix := line[:startCol-1]
	if strings.TrimSpace(prefix) != "" {
		return ""
	}
	return prefix
}

// withMarkers 在目标文本缺少注释符号时按原注释类型补上；多行行注释的每一行都补标记。
func withMarkers(c *domain.Comment, targetText, indent string) string {
	marker := ""
	switch {
	case c.Type == domain.CommentTypeDoc && strings.HasPrefix(c.SourceText, "///"):
		marker = "///"
	case c.Type == domain.CommentTypeDoc && strings.HasPrefix(c.SourceText, "//!"):
		marker = "//!"
	case c.Type == domain.CommentTypeLine:
		marker = "//"
	}
	if marker != "" {
		if strings.HasPrefix(targetText, marker) {
			return targetText
		}
		lines := strings.Split(strings.ReplaceAll(targetText, "\r\n", "\n"), "\n")
		for i, l := range lines {
			if l == "" {
				lines[i] = marker
			} else {
				lines[i] = marker + " " + l
			}
		}
		return strings.Join(lines, "\n"+indent)
	}
	if c.Type == domain.CommentTypeBlock && !strings.HasPrefix(targetText, "/*") {
		return "/* " + targetText + " */"
	}
	return targetText
}

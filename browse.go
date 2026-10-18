package codei18n

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/CairnPack/codei18n/core/index"
	"github.com/CairnPack/codei18n/core/langcode"
	"github.com/CairnPack/codei18n/core/mapping"
	"github.com/CairnPack/codei18n/core/workflow"
)

// Status 是项目的"快速载入"状态：只读 mappings、index 与缓存，不触发扫描。
type Status struct {
	Initialized bool `json:"initialized"`
	HasMappings bool `json:"hasMappings"`
	HasIndex    bool `json:"hasIndex"`
	// Comments 是 mappings.json 中的注释条数。
	Comments int `json:"comments"`
	// Translated 是本地语言文本已存在的条数。
	Translated int `json:"translated"`
	// Pending 是待翻译任务数（与 Translate 的任务数一致）。
	Pending int `json:"pending"`
	// Failed 是上次翻译失败的条数。
	Failed int `json:"failed"`
	// Languages 统计 index.json 中各原语言的注释数。
	Languages map[string]int `json:"languages"`
	// IndexGeneratedAt 是 index.json 的生成时间。
	IndexGeneratedAt string `json:"indexGeneratedAt,omitempty"`
}

// Status 读取项目状态。
func (p *Project) Status() (*Status, error) {
	env, err := p.env(nil)
	if err != nil {
		return nil, err
	}
	st := &Status{Initialized: p.Initialized(), Languages: map[string]int{}}
	if _, err := os.Stat(env.MappingsPath()); err == nil {
		st.HasMappings = true
	}
	store := mapping.NewStore(env.MappingsPath())
	if err := store.Load(); err != nil {
		return nil, fmt.Errorf("加载映射文件失败: %w", err)
	}
	local := env.Config.LocalLanguage.Value
	m := store.Snapshot()
	st.Comments = len(m.Comments)
	for _, langs := range m.Comments {
		if k := langcode.FindKey(langs, local); k != "" && langs[k] != "" {
			st.Translated++
		}
	}
	st.Pending = workflow.PendingCount(store, env.Config.SourceLanguage.Value, local)
	if failures, err := workflow.LoadFailures(env); err == nil {
		st.Failed = len(failures)
	}
	idx, err := index.Read(env.IndexPath())
	if err == nil && idx != nil {
		st.HasIndex = true
		st.IndexGeneratedAt = idx.GeneratedAt
		for _, f := range idx.Files {
			for _, c := range f.Comments {
				st.Languages[c.Lang]++
			}
		}
	}
	return st, nil
}

// CommentRecord 是注释浏览器中的一条记录：index.json 的位置信息 + mappings.json 的文本。
type CommentRecord struct {
	ID string `json:"id"`
	// File 相对于项目根目录；没有 index.json 时为空。
	File string `json:"file"`
	// Language 是编程语言标识。
	Language   string      `json:"language"`
	Type       string      `json:"type"`
	Range      index.Range `json:"range"`
	Parts      int         `json:"parts"`
	Symbol     string      `json:"symbol"`
	Lang       string      `json:"lang"`
	LangSource string      `json:"langSource"`
	// SourceKey 是原文所在的 mappings 语言键。
	SourceKey string `json:"sourceKey"`
	// Original 是原文。
	Original string `json:"original"`
	// Texts 是全部语言的文本（含原文）。
	Texts map[string]string `json:"texts"`
	// Error 是上次翻译失败的错误信息（已脱敏），没有失败时为空。
	Error string `json:"error,omitempty"`
}

// Translation 返回该记录在 lang（按等价语言码查找）下的文本。
func (r *CommentRecord) Translation(lang string) string {
	if k := langcode.FindKey(r.Texts, lang); k != "" {
		return r.Texts[k]
	}
	return ""
}

// Comments 返回项目中全部注释记录，按文件与位置排序；不在 index.json 中的条目排在最后。
func (p *Project) Comments() ([]CommentRecord, error) {
	env, err := p.env(nil)
	if err != nil {
		return nil, err
	}
	store := mapping.NewStore(env.MappingsPath())
	if err := store.Load(); err != nil {
		return nil, fmt.Errorf("加载映射文件失败: %w", err)
	}
	m := store.Snapshot()
	idx, err := index.Read(env.IndexPath())
	if err != nil {
		idx = nil // 索引损坏或版本不认识时只显示 mappings
	}
	failures := map[string]string{}
	if fs, err := workflow.LoadFailures(env); err == nil {
		for _, f := range fs {
			failures[f.ID] = f.Error
		}
	}

	seen := map[string]bool{}
	var out []CommentRecord
	if idx != nil {
		paths := make([]string, 0, len(idx.Files))
		for path := range idx.Files {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		for _, path := range paths {
			f := idx.Files[path]
			for _, c := range f.Comments {
				texts, ok := m.Comments[c.ID]
				if !ok || seen[c.ID] {
					continue
				}
				seen[c.ID] = true
				out = append(out, CommentRecord{
					ID: c.ID, File: path, Language: f.Language, Type: c.Type, Range: c.Range, Parts: c.Parts,
					Symbol: c.Symbol, Lang: c.Lang, LangSource: c.LangSource, SourceKey: c.SourceKey,
					Original: texts[c.SourceKey], Texts: texts, Error: failures[c.ID],
				})
			}
		}
	}
	rest := make([]string, 0)
	for id := range m.Comments {
		if !seen[id] {
			rest = append(rest, id)
		}
	}
	sort.Strings(rest)
	source := env.Config.SourceLanguage.Value
	for _, id := range rest {
		texts := m.Comments[id]
		key := source
		if texts[key] == "" {
			for k, v := range texts {
				if v != "" {
					key = k
					break
				}
			}
		}
		out = append(out, CommentRecord{ID: id, Lang: langcode.Unknown, LangSource: index.LangSourceFallback,
			SourceKey: key, Original: texts[key], Texts: texts, Parts: 1, Error: failures[id]})
	}
	return out, nil
}

// ErrUnknownComment 表示 mappings.json 中没有该 ID。
var ErrUnknownComment = errors.New("未知的注释 ID")

// SetTranslation 手动设置某条注释某种语言的文本，原子写回 mappings.json。text 为空时删除该语言文本。
func (p *Project) SetTranslation(id, lang, text string) error {
	if err := p.lock(); err != nil {
		return err
	}
	defer p.busy.Unlock()
	env, err := p.env(nil)
	if err != nil {
		return err
	}
	store := mapping.NewStore(env.MappingsPath())
	if err := store.Load(); err != nil {
		return err
	}
	langs := store.Languages(id)
	if langs == nil {
		return ErrUnknownComment
	}
	key := lang
	if k := langcode.FindKey(langs, lang); k != "" {
		key = k
	}
	if text == "" {
		store.DeleteLang(id, key)
	} else {
		store.Set(id, key, text)
	}
	return store.Save()
}

// SetCommentLanguage 纠正某条注释识别出的原语言。结果持久化到 overrides.json，下次扫描不会覆盖；
// 同时把原文移到新语言对应的 mappings 键下，并更新 index.json 中的条目。lang 为空表示撤销纠正
// （撤销后的语言在下次 map update 时重新识别）。
func (p *Project) SetCommentLanguage(id, lang string) error {
	if err := p.lock(); err != nil {
		return err
	}
	defer p.busy.Unlock()
	env, err := p.env(nil)
	if err != nil {
		return err
	}
	store := mapping.NewStore(env.MappingsPath())
	if err := store.Load(); err != nil {
		return err
	}
	langs := store.Languages(id)
	if langs == nil {
		return ErrUnknownComment
	}
	ov, err := index.ReadOverrides(env.OverridesPath())
	if err != nil {
		return err
	}
	canonical := langcode.Canonical(lang)
	if canonical == "" {
		delete(ov.Languages, id)
		return index.WriteOverrides(env.OverridesPath(), ov)
	}
	ov.Languages[id] = canonical

	idx, err := index.Read(env.IndexPath())
	if err != nil {
		return err
	}
	var entry *index.Comment
	if idx != nil {
		_, entry = idx.Find(id)
	}
	oldKey := env.Config.SourceLanguage.Value
	if entry != nil && entry.SourceKey != "" {
		oldKey = entry.SourceKey
	}
	newKey := workflow.StorageKey(env.Config, canonical)
	if newKey != oldKey {
		original := langs[oldKey]
		// 旧键下的文本是原文；新键下如果已有（错误方向的）译文，以原文为准覆盖。
		store.DeleteLang(id, oldKey)
		store.Set(id, newKey, original)
	}
	if entry != nil {
		entry.Lang, entry.LangSource, entry.SourceKey = canonical, index.LangSourceOverride, newKey
	}
	if err := index.WriteOverrides(env.OverridesPath(), ov); err != nil {
		return err
	}
	if err := store.Save(); err != nil {
		return err
	}
	if idx != nil {
		return index.Write(env.IndexPath(), idx)
	}
	return nil
}

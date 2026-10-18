package workflow

import (
	"strings"

	"github.com/CairnPack/codei18n/core/domain"
	"github.com/CairnPack/codei18n/core/mapping"
	"github.com/CairnPack/codei18n/core/utils"
)

// migrator 在新注释 ID 不存在于 mappings 时，尝试从旧 ID 继承已有译文，避免规则调整后重复付费翻译：
//
//  1. 合并迁移：连续行注释合并规则（docs/index-format.md §6.1）上线前，每行是一条独立记录。
//     用 PartSymbols / PartTexts 复算各行的旧 ID；各行都存在时，把每种语言的各行文本按行连接后写入新 ID。
//  2. 原文迁移：符号解析规则变化（例如 .kt 从 Java 语法改用 Kotlin 语法）会改变 ID。若恰好有一条
//     "孤儿"条目（本次扫描中不再出现的 ID）的某个语言文本与新注释原文归一化后相同，复制它的全部语言文本。
//     有多条候选时不迁移（宁缺毋滥）。
//
// 旧条目保留不删，由 map update --prune 统一清理。
type migrator struct {
	store  *mapping.Store
	orphan map[string][]string // 归一化原文 → 孤儿 ID
}

func newMigrator(store *mapping.Store, scanned []*domain.Comment) *migrator {
	seen := make(map[string]bool, len(scanned))
	for _, c := range scanned {
		seen[c.ID] = true
	}
	m := &migrator{store: store, orphan: map[string][]string{}}
	for id, langs := range store.Snapshot().Comments {
		if seen[id] {
			continue
		}
		texts := map[string]bool{}
		for _, v := range langs {
			if n := utils.NormalizeCommentText(v); n != "" && !texts[n] {
				texts[n] = true
				m.orphan[n] = append(m.orphan[n], id)
			}
		}
	}
	return m
}

// migrate 为新注释 c 继承旧译文，返回是否迁移成功。调用方随后仍会把原文写入规则键。
func (m *migrator) migrate(c *domain.Comment) bool {
	if m.mergeFromParts(c) {
		return true
	}
	ids := m.orphan[utils.NormalizeCommentText(c.SourceText)]
	if len(ids) != 1 {
		return false
	}
	langs := m.store.Languages(ids[0])
	for k, v := range langs {
		m.store.Set(c.ID, k, v)
	}
	return len(langs) > 0
}

func (m *migrator) mergeFromParts(c *domain.Comment) bool {
	n := c.PartCount()
	if n < 2 || len(c.PartSymbols) != n || len(c.PartTexts) != n {
		return false
	}
	parts := make([]map[string]string, n)
	for i := range n {
		old := &domain.Comment{File: c.File, Language: c.Language, Symbol: c.PartSymbols[i], SourceText: c.PartTexts[i]}
		parts[i] = m.store.Languages(utils.GenerateCommentID(old))
		if parts[i] == nil {
			// 空行（单独的 //）在旧规则下可能被丢弃或从未入库：用原文本身补位
			if utils.NormalizeCommentText(c.PartTexts[i]) == "" {
				parts[i] = map[string]string{}
				continue
			}
			return false
		}
	}
	var langs map[string]string
	for _, p := range parts {
		if len(p) > 0 {
			langs = p
			break
		}
	}
	migrated := false
	for lang := range langs {
		lines := make([]string, n)
		ok := true
		for i, p := range parts {
			v, has := p[lang]
			switch {
			case has:
				lines[i] = v
			case len(p) == 0: // 补位的空行
				lines[i] = c.PartTexts[i]
			default:
				ok = false
			}
		}
		if ok {
			m.store.Set(c.ID, lang, strings.Join(lines, "\n"))
			migrated = true
		}
	}
	return migrated
}

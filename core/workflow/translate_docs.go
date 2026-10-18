package workflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/CairnPack/codei18n/core"
	"github.com/CairnPack/codei18n/core/docfmt"
	"github.com/CairnPack/codei18n/core/index"
)

// payload 返回实际发给翻译器的文本：文档注释为去掉外框、受保护片段换成占位符后的正文。
func (u *unit) payload() string {
	if u.prep != nil {
		return u.prep.Body()
	}
	return u.text
}

// prepareDocs 根据 index.json 中的编程语言与注释类型，为文档注释准备结构化翻译（docs/index-format.md、core/docfmt）。
// 返回仍需翻译的 unit，以及正文全是受保护片段、直接沿用原文的 unit。没有 index.json 时保持原样（整段翻译）。
func prepareDocs(env *Env, idx *index.Index, units []*unit) (todo, direct []*unit) {
	if idx == nil {
		return units, nil
	}
	formats := map[string]docfmt.Format{}
	for _, f := range idx.Files {
		for _, c := range f.Comments {
			if fm := docfmt.For(f.Language, c.Type); fm != docfmt.None {
				formats[c.ID] = fm
			}
		}
	}
	for _, u := range units {
		fm := formats[u.tasks[0].id]
		if fm == docfmt.None {
			todo = append(todo, u)
			continue
		}
		p, err := docfmt.Prepare(fm, u.text)
		if err != nil {
			env.log().Debugf("文档注释无法结构化（%v），按整段翻译: %s", err, u.tasks[0].id)
			todo = append(todo, u)
			continue
		}
		u.prep = p
		if !p.HasProse() {
			direct = append(direct, u)
			continue
		}
		todo = append(todo, u)
	}
	return todo, direct
}

// ErrPlaceholders 表示译文的占位符在重试后仍与原文不一致。
var ErrPlaceholders = errors.New("译文丢失或打乱了受保护片段（占位符校验失败）")

// restoreUnits 把批量翻译结果还原为完整注释。文档注释校验占位符：不一致时单条重试 1 次，仍失败返回错误。
func restoreUnits(ctx context.Context, trans core.Translator, batch []*unit, results []string) ([]string, []error) {
	outs := make([]string, len(batch))
	errs := make([]error, len(batch))
	for i, u := range batch {
		if u.prep == nil {
			outs[i] = results[i]
			continue
		}
		out, err := u.prep.Restore(results[i])
		if errors.Is(err, docfmt.ErrPlaceholderMismatch) {
			retry, terr := trans.Translate(ctx, u.prep.Body(), u.fromLang, u.toLang)
			if terr != nil {
				errs[i] = terr
				continue
			}
			out, err = u.prep.Restore(retry)
		}
		if err != nil {
			errs[i] = fmt.Errorf("%w: %v", ErrPlaceholders, err)
			continue
		}
		outs[i] = out
	}
	return outs, errs
}

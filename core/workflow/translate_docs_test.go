package workflow

import (
	"context"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/CairnPack/codei18n/adapters/translator"
	"github.com/CairnPack/codei18n/core/mapping"
)

const javaDoc = "package p;\n\nclass A {\n    /**\n     * Returns the {@code sum} of two values.\n     * @param a the first {@link Number}\n     */\n    int f(int a) { return a; }\n\n    /** {@inheritDoc} */\n    int g() { return 0; }\n}\n"

var placeholderRe = regexp.MustCompile(`⟦[0-9]+⟧`)

// flakyTranslator 前 dropFirst 次请求会丢掉占位符（模拟 LLM 不守规则）。
type flakyTranslator struct {
	calls     atomic.Int32
	dropFirst int32
	seen      []string
}

func (f *flakyTranslator) Translate(ctx context.Context, text, from, to string) (string, error) {
	f.seen = append(f.seen, text)
	n := f.calls.Add(1)
	out, _ := translator.NewMockTranslator().Translate(ctx, text, from, to)
	if n <= f.dropFirst {
		out = placeholderRe.ReplaceAllString(out, "")
	}
	return out, nil
}

func (f *flakyTranslator) TranslateBatch(ctx context.Context, texts []string, from, to string) ([]string, error) {
	out := make([]string, len(texts))
	for i, t := range texts {
		out[i], _ = f.Translate(ctx, t, from, to)
	}
	return out, nil
}

func translatedJavadoc(t *testing.T, env *Env) (string, bool) {
	t.Helper()
	store := mapping.NewStore(env.MappingsPath())
	store.Load()
	for _, id := range store.IDs() {
		if src, _ := store.Get(id, "en"); strings.Contains(src, "Returns the") {
			return store.Get(id, "zh-CN")
		}
	}
	t.Fatal("未找到 Javadoc 条目")
	return "", false
}

// 受保护片段（{@code}、{@link}、@param 名称、注释外框）不发给翻译器，译文中原样保留。
func TestDocCommentStructuredTranslation(t *testing.T) {
	env := setupProject(t, map[string]string{"A.java": javaDoc})
	ft := &flakyTranslator{}
	env.Translator = ft
	if _, err := UpdateMappings(context.Background(), env, MapUpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	res, err := TranslateMissing(context.Background(), env, TranslateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.SuccessCount != 2 || res.Requests != 1 {
		t.Fatalf("{@inheritDoc} 全是受保护片段，不应发请求: %+v", res)
	}
	for _, s := range ft.seen {
		if strings.Contains(s, "{@code") || strings.Contains(s, "/**") || strings.Contains(s, "@param a") {
			t.Fatalf("受保护片段被发给了翻译器: %q", s)
		}
	}
	zh, ok := translatedJavadoc(t, env)
	want := "/**\n     * [MOCK en->zh-CN] Returns the {@code sum} of two values.\n     * @param a the first {@link Number}\n     */"
	if !ok || zh != want {
		t.Fatalf("译文应保留标签结构与外框:\n%s\nwant:\n%s", zh, want)
	}
}

// 占位符校验失败时重试 1 次；重试成功则写入。
func TestDocCommentRetryOnPlaceholderMismatch(t *testing.T) {
	env := setupProject(t, map[string]string{"A.java": javaDoc})
	ft := &flakyTranslator{dropFirst: 1}
	env.Translator = ft
	UpdateMappings(context.Background(), env, MapUpdateOptions{})
	res, err := TranslateMissing(context.Background(), env, TranslateOptions{})
	if err != nil || res.FailCount != 0 || ft.calls.Load() != 2 {
		t.Fatalf("应重试一次后成功: %+v calls=%d err=%v", res, ft.calls.Load(), err)
	}
	if zh, _ := translatedJavadoc(t, env); !strings.Contains(zh, "{@code sum}") {
		t.Fatal(zh)
	}
}

// 重试后仍失败：标记为失败，不写入坏译文。
func TestDocCommentFailsAfterRetry(t *testing.T) {
	env := setupProject(t, map[string]string{"A.java": javaDoc})
	env.Translator = &flakyTranslator{dropFirst: 100}
	UpdateMappings(context.Background(), env, MapUpdateOptions{})
	res, err := TranslateMissing(context.Background(), env, TranslateOptions{})
	if err != nil || res.FailCount != 1 || len(res.Failed) != 1 || !strings.Contains(res.Failed[0].Error, "占位符") {
		t.Fatalf("%+v %v", res, err)
	}
	if _, ok := translatedJavadoc(t, env); ok {
		t.Fatal("失败的条目不应写入译文")
	}
}

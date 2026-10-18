package workflow

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/index"
	"github.com/CairnPack/codei18n/core/langdetect"
	"github.com/CairnPack/codei18n/core/mapping"
)

const multiLang = "package a\n\n" +
	"// この関数は設定ファイルを読み込みます\nfunc A() {}\n\n" +
	"// 读取配置文件并返回解析后的结果\nfunc B() {}\n\n" +
	"// Liest die Konfigurationsdatei und gibt das Ergebnis zurück\nfunc C() {}\n\n" +
	"// Reads the configuration file and returns the parsed result\nfunc D() {}\n"

func entries(t *testing.T, env *Env) map[string]index.Comment {
	t.Helper()
	idx, err := index.Read(env.IndexPath())
	if err != nil || idx == nil {
		t.Fatal(err)
	}
	out := map[string]index.Comment{}
	for _, f := range idx.Files {
		for _, c := range f.Comments {
			out[c.Symbol] = c
		}
	}
	return out
}

// 原语言决定存放键与翻译方向：源语言注释补本地语言，本地语言注释补源语言，第三种语言两者都补。
func TestDetectedLanguagesDriveStorageAndTranslation(t *testing.T) {
	env := setupProject(t, map[string]string{"a.go": multiLang})
	env.Config = config.Resolve(nil, &config.File{DetectLanguages: []string{"en", "zh-Hans", "ja"}}, config.Overrides{})
	if _, err := UpdateMappings(context.Background(), env, MapUpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	e := entries(t, env)
	check := func(sym, lang, key string) {
		t.Helper()
		if e[sym].Lang != lang || e[sym].SourceKey != key {
			t.Errorf("%s: lang=%s key=%s, want %s/%s", sym, e[sym].Lang, e[sym].SourceKey, lang, key)
		}
	}
	check("func.A", "ja", "ja")
	check("func.B", "zh-Hans", "zh-CN")
	check("func.C", "unknown", "en") // 德语不在候选集合内：不强行归类，按旧规则存源语言键
	check("func.D", "en", "en")

	env.Translator = &countingTranslator{}
	res, err := TranslateMissing(context.Background(), env, TranslateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// A: ja→zh-CN、ja→en；B: zh-CN→en；C: en→zh-CN（旧规则）；D: en→zh-CN
	if res.TotalTasks != 5 || res.SuccessCount != 5 {
		t.Fatalf("%+v", res)
	}
	store := mapping.NewStore(env.MappingsPath())
	store.Load()
	a := store.Languages(e["func.A"].ID)
	if a["zh-CN"] != "[MOCK ja->zh-CN] // この関数は設定ファイルを読み込みます" || a["en"] == "" {
		t.Fatalf("日语注释应翻译为本地语言与源语言: %v", a)
	}
	if b := store.Languages(e["func.B"].ID); b["en"] != "[MOCK zh-Hans->en] // 读取配置文件并返回解析后的结果" {
		t.Fatalf("中文注释应补源语言: %v", b)
	}
}

// countingDetector 统计 Detect 调用次数。
type countingDetector struct {
	langdetect.Legacy
	calls atomic.Int32
}

func (d *countingDetector) Detect(s string) langdetect.Result {
	d.calls.Add(1)
	return d.Legacy.Detect(s)
}

// 识别结果按文本 hash 缓存：即使走完整路径（例如 mappings 被翻译修改过），未变化的注释也不重新识别。
func TestLanguageDetectionIsCached(t *testing.T) {
	env := setupProject(t, map[string]string{"a.go": multiLang})
	det := &countingDetector{}
	UpdateMappings(context.Background(), env, MapUpdateOptions{Detector: det})
	if det.calls.Load() != 4 {
		t.Fatalf("首次应识别 4 条: %d", det.calls.Load())
	}
	store := mapping.NewStore(env.MappingsPath())
	store.Load()
	store.Set("x", "en", "// x")
	time.Sleep(10 * time.Millisecond)
	store.Save() // 让快速路径失效
	det.calls.Store(0)
	res, err := UpdateMappings(context.Background(), env, MapUpdateOptions{Detector: det})
	if err != nil || res.Unchanged {
		t.Fatalf("应走完整路径: %+v %v", res, err)
	}
	if det.calls.Load() != 0 {
		t.Fatalf("未变化的注释不应重新识别: %d", det.calls.Load())
	}
}

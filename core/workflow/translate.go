package workflow

import (
	"context"
	"time"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/CairnPack/codei18n/core"
	"github.com/CairnPack/codei18n/core/events"
	"github.com/CairnPack/codei18n/core/index"
	"github.com/CairnPack/codei18n/core/langcode"
	"github.com/CairnPack/codei18n/core/mapping"
)

// TranslateOptions 配置翻译工作流。
type TranslateOptions struct {
	// Concurrency 并发批次数；<=0 时使用配置值。
	Concurrency int
	// Provider / Model / BatchSize 是旧版的临时覆盖，新代码请通过 config.Overrides 传入。
	Provider  string
	Model     string
	BatchSize int
	// OnlyIDs 非空时只翻译这些注释（用于失败重试）。
	OnlyIDs []string
	// SaveEvery 每完成多少批保存一次 mappings.json（默认 10）；SaveInterval 距上次保存超过该时长也保存（默认 5 秒）。
	// 结束（含取消）时必定保存。保存在独立 goroutine 中进行，不阻塞翻译。
	SaveEvery    int
	SaveInterval time.Duration
}

// TranslateResult 是翻译工作流的结果。
type TranslateResult struct {
	SuccessCount int
	FailCount    int
	TotalTasks   int
	// Failed 列出失败的条目（错误信息已脱敏）。
	Failed []Failure
	// Canceled 表示任务被取消；已完成的结果已落盘。
	Canceled bool
	// Requests 是去重后实际发给翻译后端的文本数（归一化后相同的原文只请求一次）。
	Requests int
	// Saves 是翻译过程中写 mappings.json 的次数（含结束时的一次）。
	Saves int
}

// Failure 是一条翻译失败记录。
type Failure struct {
	ID    string `json:"id"`
	From  string `json:"from"`
	To    string `json:"to"`
	Error string `json:"error"`
}

type task struct {
	id       string
	text     string
	fromLang string
	toLang   string
}

// TranslateMissing 翻译 mappings.json 中缺失的条目。ctx 取消后不再派发新批次，
// 等待进行中的批次结束（其请求也会随 ctx 取消），保存已完成的结果后返回 ctx.Err()。
func TranslateMissing(ctx context.Context, env *Env, opts TranslateOptions) (*TranslateResult, error) {
	log := env.log()
	trans, err := env.translator()
	if err != nil {
		return nil, fmt.Errorf("初始化翻译引擎失败: %w", err)
	}
	store, err := env.loadStore()
	if err != nil {
		return nil, fmt.Errorf("加载映射文件失败: %w", err)
	}

	idx, err := index.Read(env.IndexPath())
	if err != nil {
		log.Warnf("读取 index.json 失败，按旧规则判断翻译方向: %v", err)
		idx = nil
	}
	tasks := collectTasks(store, env.Config.SourceLanguage.Value, env.Config.LocalLanguage.Value, opts.OnlyIDs, originsFrom(idx))
	if len(tasks) == 0 {
		return &TranslateResult{TotalTasks: 0}, nil
	}

	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = env.Config.Provider.BatchSize.Value
	}
	if batchSize <= 0 {
		batchSize = 10
	}
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = env.Config.Provider.Concurrency.Value
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	log.Infof("发现 %d 条待翻译注释，开始批量翻译 (BatchSize=%d, Concurrency=%d)...", len(tasks), batchSize, concurrency)

	failures, err := loadFailures(env)
	if err != nil {
		log.Warnf("读取失败记录出错，将重新记录: %v", err)
		failures = map[string]Failure{}
	}

	units := dedupe(tasks)
	res := &TranslateResult{TotalTasks: len(tasks)}
	// 文档注释：受保护片段换成占位符后再发给翻译器；全是受保护片段的注释不需要翻译。
	units, direct := prepareDocs(env, idx, units)
	for _, u := range direct {
		for _, t := range u.tasks {
			store.Set(t.id, t.toLang, u.text)
			res.SuccessCount++
		}
	}
	res.Requests = len(units)
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)
	sem := make(chan struct{}, concurrency)
	processed := 0
	sv := startSaver(store, opts.SaveEvery, opts.SaveInterval, log)

dispatch:
	for i := 0; i < len(units); i += batchSize {
		batch := units[i:min(i+batchSize, len(units))]
		select {
		case <-ctx.Done():
			break dispatch
		case sem <- struct{}{}:
		}
		if ctx.Err() != nil {
			<-sem
			break
		}
		wg.Add(1)
		go func(batch []*unit) {
			defer wg.Done()
			defer func() { <-sem }()
			results, err := translateBatch(ctx, trans, batch)

			mu.Lock()
			defer mu.Unlock()
			if err != nil && ctx.Err() != nil && errors.Is(err, ctx.Err()) {
				return // 因取消而中断的批次：不计成功也不计失败
			}
			n := 0
			if err != nil {
				msg := env.sanitize(err)
				for _, u := range batch {
					for _, t := range u.tasks {
						res.FailCount++
						failures[failureKey(t)] = Failure{ID: t.id, From: t.fromLang, To: t.toLang, Error: msg}
						n++
					}
				}
			} else {
				// 还原占位符并校验；不一致的单条重试 1 次，仍失败则记为失败、不写入坏译文。
				outs, errs := restoreUnits(ctx, trans, batch, results)
				for i, u := range batch {
					for _, t := range u.tasks {
						n++
						if errs[i] != nil {
							res.FailCount++
							failures[failureKey(t)] = Failure{ID: t.id, From: t.fromLang, To: t.toLang, Error: env.sanitize(errs[i])}
							continue
						}
						store.Set(t.id, t.toLang, outs[i])
						delete(failures, failureKey(t))
						res.SuccessCount++
					}
				}
				sv.batchDone()
			}
			processed += n
			env.Progress.Emit(events.Event{Phase: events.PhaseTranslate, Done: processed, Total: len(tasks)})
		}(batch)
	}
	wg.Wait()

	env.Progress.Emit(events.Event{Phase: events.PhaseSave, Done: 0, Total: 1})
	saves, err := sv.finish()
	res.Saves = saves
	if err != nil {
		return nil, fmt.Errorf("保存映射文件失败: %w", err)
	}
	if err := saveFailures(env, failures); err != nil {
		log.Warnf("保存失败记录出错: %v", err)
	}
	inRun := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		inRun[failureKey(t)] = true
	}
	for k, f := range failures {
		if inRun[k] {
			res.Failed = append(res.Failed, f)
		}
	}
	sort.Slice(res.Failed, func(i, j int) bool { return res.Failed[i].ID < res.Failed[j].ID })

	if err := ctx.Err(); err != nil {
		res.Canceled = true
		return res, err
	}
	env.Progress.Emit(events.Event{Phase: events.PhaseDone, Done: len(tasks), Total: len(tasks)})
	return res, nil
}


// PendingCount 返回需要翻译的条目数（与 TranslateMissing 的任务数一致）。idx 可以为 nil（按旧规则判断）。
func PendingCount(store *mapping.Store, source, local string, idx ...*index.Index) int {
	var origins map[string]origin
	if len(idx) > 0 {
		origins = originsFrom(idx[0])
	}
	return len(collectTasks(store, source, local, nil, origins))
}

// origin 是一条注释原文所在的 mappings 键与识别出的原语言（来自 index.json）。
type origin struct {
	key  string
	lang string
}

func originsFrom(idx *index.Index) map[string]origin {
	if idx == nil {
		return nil
	}
	out := map[string]origin{}
	for _, f := range idx.Files {
		for _, c := range f.Comments {
			out[c.ID] = origin{key: c.SourceKey, lang: c.Lang}
		}
	}
	return out
}

// collectTasks 找出缺失的翻译。
//
// 有 index.json 时：原文在 sourceKey 下、原语言为 lang（unknown 时视为 sourceKey 的语言），
// 目标为 {源语言, 本地语言} 中与原语言不等价、且尚无文本的那些——源语言注释补本地语言，本地语言注释补源语言，
// 第三种语言（如 ja）的注释两者都补。
// 没有 index.json（或该 ID 不在索引中）时沿用旧规则：源语言有、本地语言缺 → 正向；本地语言有、源语言缺 → 反向。
func collectTasks(store *mapping.Store, source, local string, only []string, origins map[string]origin) []task {
	m := store.Snapshot()
	ids := make([]string, 0, len(m.Comments))
	if len(only) > 0 {
		ids = append(ids, only...)
	} else {
		for id := range m.Comments {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids) // 稳定的顺序便于测试与断点续传
	var tasks []task
	for _, id := range ids {
		translations, ok := m.Comments[id]
		if !ok {
			continue
		}
		if o, ok := origins[id]; ok && translations[o.key] != "" {
			from := o.lang
			if from == "" || from == langcode.Unknown {
				from = o.key
			}
			for _, target := range []string{local, source} {
				if langcode.Equivalent(target, from) || langcode.Equivalent(target, o.key) {
					continue
				}
				if k := langcode.FindKey(translations, target); k != "" && translations[k] != "" {
					continue
				}
				tasks = append(tasks, task{id: id, text: translations[o.key], fromLang: from, toLang: target})
			}
			continue
		}
		if enText := translations[source]; enText != "" {
			if translations[local] == "" {
				tasks = append(tasks, task{id: id, text: enText, fromLang: source, toLang: local})
			}
		}
		if zhText := translations[local]; zhText != "" {
			if translations[source] == "" {
				tasks = append(tasks, task{id: id, text: zhText, fromLang: local, toLang: source})
			}
		}
	}
	return tasks
}

// translateBatch 方向一致时整批请求，否则逐条翻译。
func translateBatch(ctx context.Context, trans core.Translator, batch []*unit) ([]string, error) {
	from, to := batch[0].fromLang, batch[0].toLang
	consistent := true
	for _, t := range batch {
		if t.fromLang != from || t.toLang != to {
			consistent = false
			break
		}
	}
	if consistent {
		texts := make([]string, len(batch))
		for i, t := range batch {
			texts[i] = t.payload()
		}
		results, err := trans.TranslateBatch(ctx, texts, from, to)
		if err == nil && len(results) != len(texts) {
			err = fmt.Errorf("翻译结果数量不匹配 (in=%d, out=%d)", len(texts), len(results))
		}
		return results, err
	}
	results := make([]string, len(batch))
	for i, t := range batch {
		r, err := trans.Translate(ctx, t.payload(), t.fromLang, t.toLang)
		if err != nil {
			return nil, err
		}
		results[i] = r
	}
	return results, nil
}

func failureKey(t task) string { return t.id + "\x00" + t.toLang }

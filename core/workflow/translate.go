package workflow

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/CairnPack/codei18n/core"
	"github.com/CairnPack/codei18n/core/events"
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

	tasks := collectTasks(store, env.Config.SourceLanguage.Value, env.Config.LocalLanguage.Value, opts.OnlyIDs)
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

	res := &TranslateResult{TotalTasks: len(tasks)}
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)
	sem := make(chan struct{}, concurrency)
	processed := 0

dispatch:
	for i := 0; i < len(tasks); i += batchSize {
		batch := tasks[i:min(i+batchSize, len(tasks))]
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
		go func(batch []task) {
			defer wg.Done()
			defer func() { <-sem }()
			results, err := translateBatch(ctx, trans, batch)

			mu.Lock()
			defer mu.Unlock()
			if err != nil && ctx.Err() != nil && errors.Is(err, ctx.Err()) {
				return // 因取消而中断的批次：不计成功也不计失败
			}
			if err != nil {
				msg := SanitizeError(err)
				for _, t := range batch {
					res.FailCount++
					failures[failureKey(t)] = Failure{ID: t.id, From: t.fromLang, To: t.toLang, Error: msg}
				}
			} else {
				for i, r := range results {
					t := batch[i]
					store.Set(t.id, t.toLang, r)
					delete(failures, failureKey(t))
					res.SuccessCount++
				}
				if err := store.Save(); err != nil {
					log.Warnf("保存进度失败: %v", err)
				}
			}
			processed += len(batch)
			env.Progress.Emit(events.Event{Phase: events.PhaseTranslate, Done: processed, Total: len(tasks)})
		}(batch)
	}
	wg.Wait()

	env.Progress.Emit(events.Event{Phase: events.PhaseSave, Done: 0, Total: 1})
	if err := store.Save(); err != nil {
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

// PendingCount 返回需要翻译的条目数（与 TranslateMissing 的任务数一致）。
func PendingCount(store *mapping.Store, source, local string) int {
	return len(collectTasks(store, source, local, nil))
}

// collectTasks 找出缺失的翻译：源语言有、本地语言缺 → 正向；本地语言有、源语言缺 → 反向。
func collectTasks(store *mapping.Store, source, local string, only []string) []task {
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
func translateBatch(ctx context.Context, trans core.Translator, batch []task) ([]string, error) {
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
			texts[i] = t.text
		}
		results, err := trans.TranslateBatch(ctx, texts, from, to)
		if err == nil && len(results) != len(texts) {
			err = fmt.Errorf("翻译结果数量不匹配 (in=%d, out=%d)", len(texts), len(results))
		}
		return results, err
	}
	results := make([]string, len(batch))
	for i, t := range batch {
		r, err := trans.Translate(ctx, t.text, t.fromLang, t.toLang)
		if err != nil {
			return nil, err
		}
		results[i] = r
	}
	return results, nil
}

func failureKey(t task) string { return t.id + "\x00" + t.toLang }

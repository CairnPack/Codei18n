package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/briandowns/spinner"

	"github.com/CairnPack/codei18n/adapters/translator"
	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/mapping"
	"github.com/CairnPack/codei18n/internal/log"
)

// TranslateOptions configures the translation workflow
type TranslateOptions struct {
	Concurrency int
	Provider    string
	Model       string
	BatchSize   int
}

// TranslateResult holds the result of translation workflow
type TranslateResult struct {
	SuccessCount int
	FailCount    int
	TotalTasks   int
}

// Translate executes the translation workflow
func Translate(cfg *config.Config, opts TranslateOptions) (*TranslateResult, error) {
	// Apply overrides
	if opts.Provider != "" {
		cfg.TranslationProvider = opts.Provider
	}
	if opts.BatchSize > 0 {
		cfg.BatchSize = opts.BatchSize
	}
	if opts.Model != "" {
		if cfg.TranslationConfig == nil {
			cfg.TranslationConfig = make(map[string]string)
		}
		cfg.TranslationConfig["model"] = opts.Model
	}

	// 2. Init Translator
	trans, err := translator.NewFromConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("初始化翻译引擎失败: %w", err)
	}

	// 3. Load Mapping
	storePath := filepath.Join(".codei18n", "mappings.json")
	store := mapping.NewStore(storePath)
	if err := store.Load(); err != nil {
		return nil, fmt.Errorf("加载映射文件失败: %w", err)
	}

	m := store.GetMapping()

	// 4. Identify missing translations
	type task struct {
		id       string
		text     string
		fromLang string
		toLang   string
	}
	var tasks []task

	for id, translations := range m.Comments {
		// Case 1: EN exists, ZH missing -> Translate EN to ZH
		if enText, hasEn := translations[cfg.SourceLanguage]; hasEn && enText != "" {
			if zhText, hasZh := translations[cfg.LocalLanguage]; !hasZh || zhText == "" {
				tasks = append(tasks, task{
					id:       id,
					text:     enText,
					fromLang: cfg.SourceLanguage,
					toLang:   cfg.LocalLanguage,
				})
			}
		}

		// Case 2: ZH exists, EN missing -> Translate ZH to EN (reverse translation)
		if zhText, hasZh := translations[cfg.LocalLanguage]; hasZh && zhText != "" {
			if enText, hasEn := translations[cfg.SourceLanguage]; !hasEn || enText == "" {
				tasks = append(tasks, task{
					id:       id,
					text:     zhText,
					fromLang: cfg.LocalLanguage,
					toLang:   cfg.SourceLanguage,
				})
			}
		}
	}

	if len(tasks) == 0 {
		return &TranslateResult{TotalTasks: 0}, nil
	}

	log.Info("发现 %d 条待翻译注释，开始批量翻译 (BatchSize=%d, Concurrency=%d)...", len(tasks), cfg.BatchSize, opts.Concurrency)

	// 5. Process with Batching and Concurrency
	s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
	s.Suffix = " 正在翻译..."
	s.Writer = os.Stderr
	s.Start()

	var wg sync.WaitGroup
	sem := make(chan struct{}, opts.Concurrency)
	successCount := 0
	failCount := 0
	var countMu sync.Mutex

	// Split tasks into batches
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 10 // Safe default
	}

	batches := make([][]task, 0, (len(tasks)+batchSize-1)/batchSize)
	for i := 0; i < len(tasks); i += batchSize {
		end := i + batchSize
		if end > len(tasks) {
			end = len(tasks)
		}
		batches = append(batches, tasks[i:end])
	}

	for _, batch := range batches {
		wg.Add(1)
		sem <- struct{}{} // Acquire token

		go func(currentBatch []task) {
			defer wg.Done()
			defer func() { <-sem }() // Release token

			// Prepare batch input
			texts := make([]string, len(currentBatch))
			for i, t := range currentBatch {
				texts[i] = t.text
			}

			// Group by direction
			// Let's assume for typical usage (filling missing), it's usually En->Zh.
			// But mixed is possible.
			from := currentBatch[0].fromLang
			to := currentBatch[0].toLang
			consistent := true
			for _, t := range currentBatch {
				if t.fromLang != from || t.toLang != to {
					consistent = false
					break
				}
			}

			var results []string
			var err error

			if consistent {
				results, err = trans.TranslateBatch(context.Background(), texts, from, to)
			} else {
				// Mixed batch, fallback to sequential loop manually here
				results = make([]string, len(currentBatch))
				for i, t := range currentBatch {
					res, e := trans.Translate(context.Background(), t.text, t.fromLang, t.toLang)
					if e != nil {
						err = e // Capture last error
						break
					}
					results[i] = res
				}
			}

			countMu.Lock()

			if err != nil {
				failCount += len(currentBatch)
			} else {
				// Save results
				for i, res := range results {
					t := currentBatch[i]
					store.Set(t.id, t.toLang, res)
					successCount++
				}
				// Save progress immediately
				if err := store.Save(); err != nil {
					log.Warn("保存进度失败: %v", err)
				}
			}

			countMu.Unlock()

			s.Suffix = fmt.Sprintf(" 正在翻译... (%d/%d 成功, %d 失败)", successCount, len(tasks), failCount)
		}(batch)
	}

	wg.Wait()
	s.Stop()

	// 6. Save
	if err := store.Save(); err != nil {
		return nil, fmt.Errorf("保存映射文件失败: %w", err)
	}

	return &TranslateResult{
		SuccessCount: successCount,
		FailCount:    failCount,
		TotalTasks:   len(tasks),
	}, nil
}

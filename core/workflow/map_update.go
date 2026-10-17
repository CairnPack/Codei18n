package workflow

import (
	"fmt"
	"path/filepath"

	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/mapping"
	"github.com/CairnPack/codei18n/core/scanner"
	"github.com/CairnPack/codei18n/core/utils"
	"github.com/CairnPack/codei18n/internal/log"
)

// MapUpdateResult holds the result of map update operation
type MapUpdateResult struct {
	TotalComments int
	AddedCount    int
	StorePath     string
}

// MapUpdate executes the logic for updating the mapping file
func MapUpdate(cfg *config.Config, scanDir string, dryRun bool) (*MapUpdateResult, error) {
	// 1. Scan for comments
	log.Info("正在扫描目录: %s", scanDir)
	comments, err := scanner.Directory(scanDir, cfg.ExcludePatterns...)
	if err != nil {
		return nil, fmt.Errorf("扫描失败: %w", err)
	}

	// 2. Generate IDs
	for _, c := range comments {
		if c.ID == "" {
			c.ID = utils.GenerateCommentID(c)
		}
	}

	// 3. Load Store
	storePath := filepath.Join(".codei18n", "mappings.json")
	store := mapping.NewStore(storePath)
	if err := store.Load(); err != nil {
		// Just warn if load fails (might be new file), but for robustness we should probably check if file exists
		// logic in original map.go was: log.Fatal("加载映射文件失败: %v", err)
		// but Store.Load() usually returns error if file is malformed, not if missing (if NewStore handles missing correctly)
		// Let's assume we want to fail if existing file is bad
		return nil, fmt.Errorf("加载映射文件失败: %w", err)
	}

	// 4. Update
	m := store.GetMapping()
	m.SourceLanguage = cfg.SourceLanguage
	m.TargetLanguage = cfg.LocalLanguage

	addedCount := 0
	for _, c := range comments {
		// Check if ID exists
		if _, exists := m.Comments[c.ID]; !exists {
			// [MOCK zh-CN->en] // Intelligently detect comment language
			detectedLang := utils.DetectLanguage(c.SourceText)

			if detectedLang == cfg.LocalLanguage {
				// The comment is in the local language, stored as LocalLanguage
				store.Set(c.ID, cfg.LocalLanguage, c.SourceText)
				log.Info("检测到中文注释: ID=%s, Text=%s", c.ID, c.SourceText)
			} else {
				// The comment is in the source language, stored as SourceLanguage
				store.Set(c.ID, cfg.SourceLanguage, c.SourceText)
			}
			addedCount++
		}
	}

	result := &MapUpdateResult{
		TotalComments: len(comments),
		AddedCount:    addedCount,
		StorePath:     storePath,
	}

	if dryRun {
		return result, nil
	}

	// 5. Save
	if err := store.Save(); err != nil {
		return nil, fmt.Errorf("保存映射文件失败: %w", err)
	}

	return result, nil
}

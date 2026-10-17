package scanner

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/CairnPack/codei18n/adapters"
	"github.com/CairnPack/codei18n/core/domain"
	"github.com/CairnPack/codei18n/internal/log"
)

// FromStdin scans code from standard input
func FromStdin(filename string) ([]*domain.Comment, error) {
	// Read stdin
	src, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("读取 stdin 失败: %w", err)
	}

	adapter, err := adapters.GetAdapter(filename)
	if err != nil {
		return nil, err
	}
	return adapter.Parse(filename, src)
}

// SingleFile scans a single file
func SingleFile(filename string) ([]*domain.Comment, error) {
	adapter, err := adapters.GetAdapter(filename)
	if err != nil {
		return nil, err
	}

	// Read file content manually to ensure consistency with directory scan
	// and to debug potential read issues
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read file failed: %w", err)
	}

	return adapter.Parse(filename, content)
}

// Directory recursively scans a directory
func Directory(dir string, excludePatterns ...string) ([]*domain.Comment, error) {
	var comments []*domain.Comment
	var walkErr error

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path for matching
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			relPath = path
		}
		// Normalize path separators to / for glob matching
		relPath = filepath.ToSlash(relPath)

		// Check excludes
		for _, pattern := range excludePatterns {
			matched, _ := doublestar.Match(pattern, relPath)
			if matched {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		if info.IsDir() {
			// Skip .git, vendor, .codei18n
			if info.Name() == ".git" || info.Name() == "vendor" || info.Name() == ".codei18n" {
				return filepath.SkipDir
			}
			return nil
		}

		// Try to get adapter for file
		adapter, err := adapters.GetAdapter(path)
		if err == nil {
			// Supported file

			// Read file content manually to ensure we access the correct file
			content, err := os.ReadFile(path)
			if err != nil {
				log.Warn("读取文件 %s 失败: %v", path, err)
				return nil
			}

			fileComments, err := adapter.Parse(relPath, content)
			if err != nil {
				log.Warn("解析文件 %s 失败: %v", path, err)
				return nil // Continue scanning other files
			}
			comments = append(comments, fileComments...)
		}
		return nil
	})

	if walkErr != nil {
		return nil, walkErr
	}
	return comments, err
}

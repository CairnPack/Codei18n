package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/CairnPack/codei18n"
	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/domain"
	"github.com/CairnPack/codei18n/core/mapping"
	"github.com/CairnPack/codei18n/core/scanner"
	"github.com/CairnPack/codei18n/core/utils"
	"github.com/CairnPack/codei18n/internal/log"
)

var (
	scanFile             string
	scanDir              string
	scanFormat           string
	scanOutput           string
	scanLang             string
	scanStdin            bool
	scanWithTranslations bool
)

// scanCmd represents the scan command
var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "扫描源码并提取注释",
	Long: `扫描指定的源码文件或目录，提取注释信息并生成唯一 ID。
支持多种输出格式（JSON, Table），并支持从 Stdin 读取内容以支持 IDE 集成。`,
	Run: func(cmd *cobra.Command, args []string) {
		runScan()
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)

	scanCmd.Flags().StringVarP(&scanFile, "file", "f", "", "指定扫描的单个文件路径")
	scanCmd.Flags().StringVarP(&scanDir, "dir", "d", ".", "指定扫描的目录路径")
	scanCmd.Flags().StringVar(&scanFormat, "format", "table", "输出格式 (json, table)")
	scanCmd.Flags().StringVarP(&scanOutput, "output", "o", "", "将输出写入指定文件 (默认 stdout)")
	scanCmd.Flags().StringVar(&scanLang, "lang", "", "指定目标语言 (覆盖配置)")
	scanCmd.Flags().BoolVar(&scanStdin, "stdin", false, "从 stdin 读取文件内容 (必须同时指定 --file)")
	scanCmd.Flags().BoolVar(&scanWithTranslations, "with-translations", false, "在 JSON 输出中包含翻译文本")
}

func runScan() {
	// Validation
	if scanStdin && scanFile == "" {
		log.Fatal("--stdin 模式下必须指定 --file 以确定语言和 ID 上下文")
	}

	var comments []*domain.Comment
	var err error

	// Determine strategy
	if scanStdin {
		comments, err = scanner.FromStdin(scanFile)
	} else if scanFile != "" {
		comments, err = scanner.SingleFile(scanFile)
	} else {
		p := openProject(config.Overrides{})
		ctx, cancel := signalContext()
		defer cancel()
		var res *scanner.Result
		res, err = p.Scan(ctx, codei18n.ScanOptions{Dir: scanDir})
		if res != nil {
			comments = res.Comments()
		}
	}

	if err != nil {
		log.Fatal("扫描失败: %v", err)
	}

	// Calculate IDs for all comments
	for _, c := range comments {
		if c.ID == "" {
			c.ID = utils.GenerateCommentID(c)
		}
	}

	// Load mapping if requested
	if scanWithTranslations {
		p := openProject(config.Overrides{})
		cfg := effectiveConfig(p)

		// Load store
		store := mapping.NewStore(filepath.Join(p.DataDir(), mapping.FileName))
		// We ignore error here, if file doesn't exist, we just don't show translations
		// or better: log.Warn
		if err := store.Load(); err != nil {
			log.Warn("加载映射文件失败: %v", err)
		} else {
			// Populate LocalizedText
			targetLang := cfg.LocalLanguage.Value // Default target
			if scanLang != "" {
				targetLang = scanLang
			}
			for _, c := range comments {
				if text, ok := store.Get(c.ID, targetLang); ok {
					c.LocalizedText = text
				}
			}
		}
	}

	// Output
	if err := outputResults(comments); err != nil {
		log.Fatal("输出结果失败: %v", err)
	}
}

func outputResults(comments []*domain.Comment) error {
	// Prepare output data
	type Output struct {
		File     string            `json:"file,omitempty"` // populated if single file? Or just list?
		Comments []*domain.Comment `json:"comments"`
	}

	var data interface{}
	if scanFile != "" || scanStdin {
		// Single file context
		data = Output{
			File:     scanFile,
			Comments: comments,
		}
	} else {
		// Directory context - just return the list
		data = comments
	}

	if scanFormat == "json" {
		jsonData, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return err
		}

		if scanOutput != "" {
			return os.WriteFile(scanOutput, jsonData, 0644)
		}

		log.PrintJSON(jsonData)
		return nil
	}

	// Table format (for humans)
	if scanOutput != "" {
		// Write simple text to file
		f, err := os.Create(scanOutput)
		if err != nil {
			return err
		}
		defer f.Close()
		for _, c := range comments {
			fmt.Fprintf(f, "[%s] %s: %s\n", c.ID[:8], c.Symbol, c.SourceText)
		}
		return nil
	}

	// Print to stdout
	fmt.Printf("扫描完成，共找到 %d 条注释:\n", len(comments))
	for _, c := range comments {
		fmt.Printf("- [%s] %s (%d:%d): %s\n", c.ID[:8], c.Symbol, c.Range.StartLine, c.Range.StartCol, c.SourceText)
	}
	return nil
}

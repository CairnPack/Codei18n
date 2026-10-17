package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/mapping"
	"github.com/CairnPack/codei18n/core/workflow"
	"github.com/CairnPack/codei18n/internal/log"
)

var (
	mapScanDir string
	mapDryRun  bool
	mapLang    string
)

// mapCmd represents the map command
var mapCmd = &cobra.Command{
	Use:   "map",
	Short: "管理多语言映射",
	Long:  `创建、更新或查询注释的多语言映射文件。`,
}

// mapUpdateCmd represents the map update command
var mapUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "更新映射文件",
	Long:  `扫描项目中的注释，并将新发现的注释添加到映射文件中。`,
	Run: func(cmd *cobra.Command, args []string) {
		runMapUpdate()
	},
}

// mapGetCmd represents the map get command
var mapGetCmd = &cobra.Command{
	Use:   "get [commentID]",
	Short: "查询注释翻译",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runMapGet(args[0])
	},
}

func init() {
	rootCmd.AddCommand(mapCmd)
	mapCmd.AddCommand(mapUpdateCmd)
	mapCmd.AddCommand(mapGetCmd)

	mapUpdateCmd.Flags().StringVar(&mapScanDir, "scan-dir", ".", "扫描目录以更新映射")
	mapUpdateCmd.Flags().BoolVar(&mapDryRun, "dry-run", false, "仅显示变更，不写入文件")

	mapGetCmd.Flags().StringVar(&mapLang, "lang", "", "目标语言代码 (默认使用配置中的 LocalLanguage)")
}

func runMapUpdate() {
	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Warn("无法加载配置，使用默认值: %v", err)
		cfg = config.DefaultConfig()
	}

	result, err := workflow.MapUpdate(cfg, mapScanDir, mapDryRun)
	if err != nil {
		log.Fatal("Map update failed: %v", err)
	}

	log.Success("发现 %d 条注释，新增 %d 条映射", result.TotalComments, result.AddedCount)
	if mapDryRun {
		log.Info("Dry run 模式，不保存文件")
	} else {
		log.Success("映射文件已更新: %s", result.StorePath)
	}
}

func runMapGet(commentID string) {
	cfg, err := config.LoadConfig()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	targetLang := mapLang
	if targetLang == "" {
		targetLang = cfg.LocalLanguage
	}

	storePath := filepath.Join(".codei18n", "mappings.json")
	store := mapping.NewStore(storePath)
	if err := store.Load(); err != nil {
		log.Fatal("加载映射文件失败: %v", err)
	}

	if text, ok := store.Get(commentID, targetLang); ok {
		fmt.Println(text)
	} else {
		log.Error("未找到 ID 为 %s 的 %s 翻译", commentID, targetLang)
		os.Exit(1)
	}
}

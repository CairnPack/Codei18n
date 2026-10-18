package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/CairnPack/codei18n"
	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/internal/log"
)

var (
	convertFile   string
	convertDir    string
	convertDryRun bool
	convertTo     string
)

var convertCmd = &cobra.Command{
	Use:   "convert",
	Short: "转换源码中的注释语言",
	Long:  `在源码中原地修改注释，将其替换为目标语言的翻译文本（或还原为源语言）。`,
	Run: func(cmd *cobra.Command, args []string) {
		runConvert()
	},
}

func init() {
	rootCmd.AddCommand(convertCmd)

	convertCmd.Flags().StringVarP(&convertFile, "file", "f", "", "指定文件")
	convertCmd.Flags().StringVarP(&convertDir, "dir", "d", ".", "指定目录")
	convertCmd.Flags().StringVar(&convertTo, "to", "", "目标语言 (en 或 zh-CN)")
	convertCmd.Flags().BoolVar(&convertDryRun, "dry-run", false, "仅显示将要修改的内容")
}

func runConvert() {
	if convertTo == "" {
		log.Fatal("必须指定目标语言: --to <lang>")
	}
	p := openProject(config.Overrides{})
	ctx, cancel := signalContext()
	defer cancel()

	res, err := p.Convert(ctx, codei18n.ConvertOptions{File: convertFile, Dir: convertDir, To: convertTo, DryRun: convertDryRun})
	if err != nil {
		log.Fatal("%v", err)
	}
	if convertDryRun {
		for _, f := range res.Files {
			fmt.Printf("File: %s\n", f)
		}
		return
	}
	if res.Missing > 0 {
		log.Fatal("发现 %d 条缺失翻译，请先运行 'codei18n translate' 完成翻译", res.Missing)
	}
}

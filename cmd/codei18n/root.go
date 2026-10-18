package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/CairnPack/codei18n"
	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/internal/log"
)

var (
	cfgFile string
	verbose bool
	version = "dev" // Override at build time via -ldflags "-X main.version=<version>"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "codei18n",
	Short: "CodeI18n 核心 CLI 工具",
	Long: `CodeI18n 是一个代码注释国际化基础设施工具。
它支持源码注释扫描、ID 生成、多语言映射管理以及自动翻译功能。
主要用于将源码中的英文注释与本地语言注释进行互转。`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("CodeI18n CLI 版本: {{.Version}}\n")

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "配置文件路径 (默认读取 ~/.codei18n/config.json 以及当前目录的 .codei18n/config.json)")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "显示详细日志")
}

// cliLogger 把库日志输出到 stderr（带颜色）。Debug 级别只在 --verbose 时输出。
type cliLogger struct{}

func (cliLogger) Debugf(format string, args ...any) {
	if verbose {
		log.Debug(format, args...)
	}
}
func (cliLogger) Infof(format string, args ...any) { log.Info(format, args...) }
func (cliLogger) Warnf(format string, args ...any) { log.Warn(format, args...) }

// openProject 以当前目录为项目根目录打开项目。
func openProject(ov config.Overrides) *codei18n.Project {
	codei18n.Version = version
	opts := []codei18n.Option{codei18n.WithLogger(cliLogger{}), codei18n.WithOverrides(ov)}
	if cfgFile != "" {
		if _, err := os.Stat(cfgFile); err != nil {
			log.Fatal("读取配置文件失败: %v", err)
		}
		opts = append(opts, codei18n.WithConfigFile(cfgFile))
		if verbose {
			fmt.Fprintf(os.Stderr, "Using config file: %s\n", cfgFile)
		}
	}
	p, err := codei18n.Open(".", opts...)
	if err != nil {
		log.Fatal("打开项目失败: %v", err)
	}
	return p
}

// effectiveConfig 读取合并后的配置；失败时警告并退回默认值（与旧版行为一致）。
func effectiveConfig(p *codei18n.Project) *config.Effective {
	cfg, err := p.Config()
	if err != nil {
		log.Warn("无法加载配置，使用默认值: %v", err)
		return config.Resolve(nil, nil, config.Overrides{})
	}
	return cfg
}

// signalContext 返回在 Ctrl+C 时取消的 context。
func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}

func main() {
	Execute()
}

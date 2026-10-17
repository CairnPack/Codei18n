package codei18n

import (
	"context"
	"os"

	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/gitint"
	"github.com/CairnPack/codei18n/core/scanner"
	"github.com/CairnPack/codei18n/core/workflow"
)

// 结果类型。
type (
	MapUpdateResult = workflow.MapUpdateResult
	TranslateResult = workflow.TranslateResult
	ConvertResult   = workflow.ConvertResult
	Failure         = workflow.Failure
	ScanResult      = scanner.Result
)

// InitOptions 配置项目初始化。nil / 空值表示不写该字段（继承全局或默认值）。
type InitOptions struct {
	SourceLanguage  *string
	LocalLanguage   *string
	ExcludePatterns []string
	Profile         *string
	// Provider 写入旧版 translationProvider 字段（与 CLI --provider 一致）。
	Provider *string
	// Snapshot 非 nil 时把这份扁平配置的全部字段写入项目文件（旧版 CLI init 的行为）。
	Snapshot *config.Config
	// Force 覆盖已存在的配置文件（保留原有 projectId）。
	Force bool
}

// Init 写入项目配置 .codei18n/config.json，并生成 projectId。
func (p *Project) Init(opts InitOptions) error {
	path := config.ProjectPath(p.root)
	if _, err := os.Stat(path); err == nil && !opts.Force {
		return ErrAlreadyInitialized
	}
	prev, err := config.ReadFile(path)
	if err != nil {
		prev = &config.File{}
	}
	f := &config.File{ProjectID: prev.ProjectID}
	if f.ProjectID == "" {
		f.ProjectID = config.NewProjectID()
	}
	if opts.Snapshot != nil {
		f.ApplyLegacy(opts.Snapshot.Sanitize())
	}
	if opts.SourceLanguage != nil {
		f.SourceLanguage = opts.SourceLanguage
	}
	if opts.LocalLanguage != nil {
		f.LocalLanguage = opts.LocalLanguage
	}
	if opts.ExcludePatterns != nil {
		f.ExcludePatterns = append([]string(nil), opts.ExcludePatterns...)
	}
	if opts.Profile != nil {
		f.Profile = opts.Profile
	}
	if opts.Provider != nil {
		f.TranslationProvider = opts.Provider
	}
	return config.WriteFile(path, f)
}

// IsGitRepo 判断项目根目录是否是 Git 仓库。
func (p *Project) IsGitRepo() bool { return gitint.IsRepo(p.root) }

// InstallHooks 安装 pre-commit 与 commit-msg hook。
func (p *Project) InstallHooks() error { return gitint.InstallHooks(p.root) }

// HooksInstalled 判断 hook 是否已安装。
func (p *Project) HooksInstalled() bool { return gitint.HooksInstalled(p.root) }

// EnsureGitignore 确保 .gitignore 中忽略 .codei18n/；返回是否修改了文件。
func (p *Project) EnsureGitignore() (bool, error) { return gitint.EnsureGitignore(p.root) }

// ScanOptions 配置只读扫描。
type ScanOptions struct {
	// Dir 为空时扫描项目根目录。
	Dir      string
	Progress ProgressFunc
}

// Scan 只读地扫描注释，不写任何文件。
func (p *Project) Scan(ctx context.Context, opts ScanOptions) (*ScanResult, error) {
	cfg, err := p.Config()
	if err != nil {
		return nil, err
	}
	dir := opts.Dir
	if dir == "" {
		dir = p.root
	}
	return scanner.Scan(ctx, scanner.Options{Dir: dir, Exclude: cfg.ExcludePatterns.Value, Logger: p.s.logger, Progress: opts.Progress})
}

// MapOptions 配置映射更新。
type MapOptions struct {
	// ScanDir 为空时扫描项目根目录。
	ScanDir  string
	DryRun   bool
	Progress ProgressFunc
}

// UpdateMappings 扫描源码，把新注释加入 mappings.json，并原子写出 index.json。
func (p *Project) UpdateMappings(ctx context.Context, opts MapOptions) (*MapUpdateResult, error) {
	if err := p.lock(); err != nil {
		return nil, err
	}
	defer p.busy.Unlock()
	env, err := p.env(opts.Progress)
	if err != nil {
		return nil, err
	}
	return workflow.UpdateMappings(ctx, env, workflow.MapUpdateOptions{ScanDir: opts.ScanDir, DryRun: opts.DryRun})
}

// TranslateOptions 配置翻译。
type TranslateOptions struct {
	// Concurrency <=0 时使用配置值。
	Concurrency int
	// OnlyIDs 非空时只翻译这些注释（失败重试）。
	OnlyIDs  []string
	Progress ProgressFunc
}

// Translate 翻译缺失的条目。取消后已完成的结果已落盘，返回的错误为 ctx.Err()，结果中 Canceled 为 true。
func (p *Project) Translate(ctx context.Context, opts TranslateOptions) (*TranslateResult, error) {
	if err := p.lock(); err != nil {
		return nil, err
	}
	defer p.busy.Unlock()
	env, err := p.env(opts.Progress)
	if err != nil {
		return nil, err
	}
	return workflow.TranslateMissing(ctx, env, workflow.TranslateOptions{Concurrency: opts.Concurrency, OnlyIDs: opts.OnlyIDs})
}

// TranslateText 用项目配置的后端翻译一段文本；from/to 为空时为源语言 → 本地语言。
func (p *Project) TranslateText(ctx context.Context, text, from, to string) (string, error) {
	env, err := p.env(nil)
	if err != nil {
		return "", err
	}
	return workflow.TranslateTextWith(ctx, env, text, from, to)
}

// ConvertOptions 配置源码改写。
type ConvertOptions struct {
	File     string
	Dir      string
	To       string
	DryRun   bool
	Progress ProgressFunc
}

// Convert 在源码中原地改写注释为目标语言（源码改写逻辑与旧版一致）。
func (p *Project) Convert(ctx context.Context, opts ConvertOptions) (*ConvertResult, error) {
	if err := p.lock(); err != nil {
		return nil, err
	}
	defer p.busy.Unlock()
	env, err := p.env(opts.Progress)
	if err != nil {
		return nil, err
	}
	dir := opts.Dir
	if dir == "" && opts.File == "" {
		dir = p.root
	}
	return workflow.Convert(ctx, env, workflow.ConvertOptions{File: opts.File, Dir: dir, To: opts.To, DryRun: opts.DryRun})
}

// Failures 返回上次翻译留下的失败条目。
func (p *Project) Failures() ([]Failure, error) {
	env, err := p.env(nil)
	if err != nil {
		return nil, err
	}
	return workflow.LoadFailures(env)
}

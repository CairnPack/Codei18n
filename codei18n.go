// Package codei18n 是 CodeI18n 的库入口：以项目根目录为单位打开项目，执行扫描、映射更新、翻译、改写等长任务。
//
// 同一进程中可以同时打开多个 Project，彼此互不影响（没有全局可变状态）。库代码不打印、不调用 os.Exit，
// 日志通过 Logger 注入，进度通过 ProgressFunc 回调发出，所有长任务都接受 context.Context 并可取消。
//
//	p, err := codei18n.Open(root, codei18n.WithLogger(myLogger))
//	res, err := p.UpdateMappings(ctx, codei18n.MapOptions{Progress: onProgress})
package codei18n

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/CairnPack/codei18n/core"
	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/events"
	"github.com/CairnPack/codei18n/core/logging"
	"github.com/CairnPack/codei18n/core/workflow"
)

// Version 是库版本，写入 index.json 的 generator 字段；CLI 构建时可覆盖。
var Version = "dev"

// Logger 是可注入的日志接口。
type Logger = logging.Logger

// Event 是结构化进度事件。
type Event = events.Event

// Phase 是进度阶段。
type Phase = events.Phase

// ProgressFunc 接收进度事件；可能在任意 goroutine 上被调用。
type ProgressFunc = events.Func

// 进度阶段。
const (
	PhaseDiscover  = events.PhaseDiscover
	PhaseScan      = events.PhaseScan
	PhaseMap       = events.PhaseMap
	PhaseIndex     = events.PhaseIndex
	PhaseTranslate = events.PhaseTranslate
	PhaseSave      = events.PhaseSave
	PhaseConvert   = events.PhaseConvert
	PhaseDone      = events.PhaseDone
)

// ErrBusy 表示该项目正在执行另一个会写入映射的长任务。
var ErrBusy = errors.New("项目正在执行其他任务")

// ErrNotInitialized 表示项目尚未初始化（没有 .codei18n/config.json）。
var ErrNotInitialized = errors.New("项目尚未初始化")

// ErrAlreadyInitialized 表示项目已初始化且未要求覆盖。
var ErrAlreadyInitialized = errors.New("配置文件已存在")

// Option 配置 Open。
type Option func(*settings)

type settings struct {
	logger     Logger
	globalDir  string
	configFile string
	overrides  config.Overrides
	translator core.Translator
	secrets    SecretStore
	getenv     func(string) string
}

// WithLogger 注入日志实现；默认丢弃日志。
func WithLogger(l Logger) Option { return func(s *settings) { s.logger = l } }

// WithGlobalDir 指定全局配置目录（默认 ~/.codei18n）。
func WithGlobalDir(dir string) Option { return func(s *settings) { s.globalDir = dir } }

// WithConfigFile 只使用指定的配置文件（作为项目层），不读取全局与项目默认位置的配置。
// 对应 CLI 的 --config。
func WithConfigFile(path string) Option { return func(s *settings) { s.configFile = path } }

// WithOverrides 设置调用方覆盖（优先级最高，不写入文件）。
func WithOverrides(ov config.Overrides) Option { return func(s *settings) { s.overrides = ov } }

// WithTranslator 注入翻译后端（测试或自定义后端），忽略配置中的 provider。
func WithTranslator(t core.Translator) Option { return func(s *settings) { s.translator = t } }

// Project 是一个已打开的项目。方法可以从多个 goroutine 调用；写映射的长任务在同一项目上互斥（返回 ErrBusy）。
type Project struct {
	root string
	s    settings
	busy sync.Mutex
}

// Open 打开以 root 为根目录的项目。root 必须是已存在的目录；项目不必已初始化。
func Open(root string, opts ...Option) (*Project, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("%s 不是目录", abs)
	}
	p := &Project{root: abs}
	for _, o := range opts {
		o(&p.s)
	}
	if p.s.getenv == nil {
		p.s.getenv = os.Getenv
	}
	if p.s.globalDir == "" {
		if dir, err := config.GlobalDir(); err == nil {
			p.s.globalDir = dir
		}
	}
	return p, nil
}

// Root 返回项目根目录（绝对路径）。
func (p *Project) Root() string { return p.root }

// DataDir 返回 <root>/.codei18n。
func (p *Project) DataDir() string { return filepath.Join(p.root, config.DirName) }

// ConfigPath 返回项目配置文件路径。
func (p *Project) ConfigPath() string {
	if p.s.configFile != "" {
		return p.s.configFile
	}
	return config.ProjectPath(p.root)
}

// GlobalConfigPath 返回全局配置文件路径；使用 WithConfigFile 时为空。
func (p *Project) GlobalConfigPath() string {
	if p.s.configFile != "" || p.s.globalDir == "" {
		return ""
	}
	return filepath.Join(p.s.globalDir, config.FileName)
}

// Initialized 判断项目是否已初始化（存在项目配置文件）。
func (p *Project) Initialized() bool {
	_, err := os.Stat(config.ProjectPath(p.root))
	return err == nil
}

// Files 返回全局与项目两层配置文件的内容（不存在时为空 File）。
func (p *Project) Files() (global, project *config.File, err error) {
	global = &config.File{}
	if path := p.GlobalConfigPath(); path != "" {
		if global, err = config.ReadFile(path); err != nil {
			return nil, nil, err
		}
	}
	if project, err = config.ReadFile(p.ConfigPath()); err != nil {
		return nil, nil, err
	}
	return global, project, nil
}

// Config 每次重新读取配置文件并返回合并后的值及每个字段的来源层。
func (p *Project) Config() (*config.Effective, error) {
	global, project, err := p.Files()
	if err != nil {
		return nil, err
	}
	return config.Resolve(global, project, p.s.overrides), nil
}

// UpdateConfig 原子地修改项目配置文件（保留未识别字段）。
func (p *Project) UpdateConfig(fn func(*config.File) error) error {
	return config.UpdateFile(p.ConfigPath(), fn)
}

// env 为一次长任务构造运行环境。
func (p *Project) env(progress ProgressFunc) (*workflow.Env, error) {
	cfg, err := p.Config()
	if err != nil {
		return nil, err
	}
	r := &redactor{}
	return &workflow.Env{
		Root:       p.root,
		Config:     cfg,
		Logger:     p.s.logger,
		Progress:   progress,
		Generator:  "codei18n/" + Version,
		Translator: p.s.translator,
		Key:        p.keySource(cfg, r),
		Redact:     r.redact,
	}, nil
}

// lock 尝试获取项目的写锁。
func (p *Project) lock() error {
	if !p.busy.TryLock() {
		return ErrBusy
	}
	return nil
}

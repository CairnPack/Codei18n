package workflow

import (
	"path/filepath"

	"github.com/CairnPack/codei18n/adapters/translator"
	"github.com/CairnPack/codei18n/core"
	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/events"
	"github.com/CairnPack/codei18n/core/index"
	"github.com/CairnPack/codei18n/core/logging"
	"github.com/CairnPack/codei18n/core/mapping"
)

// Env 是长任务的运行环境。同一进程中可以有多个 Env（对应多个项目），彼此互不影响。
type Env struct {
	// Root 是项目根目录（包含 .codei18n 的目录），必须是绝对路径或相对于当前目录可解析的路径。
	Root string
	// Config 是合并后的有效配置。
	Config *config.Effective
	Logger logging.Logger
	// Progress 接收结构化进度事件，可以为 nil。
	Progress events.Func
	// Generator 写入 index.json 的生成工具标识，例如 "codei18n/1.2.0"。
	Generator string
	// Translator 非 nil 时直接使用（测试、GUI 注入）；否则按 Config 构造。
	Translator core.Translator
	// Key 在需要 API 令牌时被调用；nil 时读取环境变量。
	Key translator.KeySource
}

func (e *Env) log() logging.Logger { return logging.OrNop(e.Logger) }

// DataDir 返回 <root>/.codei18n。
func (e *Env) DataDir() string { return filepath.Join(e.Root, config.DirName) }

// MappingsPath 返回 mappings.json 路径。
func (e *Env) MappingsPath() string { return filepath.Join(e.DataDir(), mapping.FileName) }

// IndexPath 返回 index.json 路径。
func (e *Env) IndexPath() string { return filepath.Join(e.DataDir(), index.FileName) }

// OverridesPath 返回 overrides.json 路径。
func (e *Env) OverridesPath() string { return filepath.Join(e.DataDir(), index.OverridesFileName) }

// CacheDir 返回增量扫描等缓存所在目录。
func (e *Env) CacheDir() string { return filepath.Join(e.DataDir(), "cache") }

// translator 返回注入的翻译器，或按配置构造一个。
func (e *Env) translator() (core.Translator, error) {
	if e.Translator != nil {
		return e.Translator, nil
	}
	key := e.Key
	if key == nil {
		key = translator.EnvKey
	}
	return translator.New(translator.SettingsFrom(e.Config, e.Logger), key)
}

// loadStore 读取项目的 mappings.json。
func (e *Env) loadStore() (*mapping.Store, error) {
	store := mapping.NewStore(e.MappingsPath())
	if err := store.Load(); err != nil {
		return nil, err
	}
	return store, nil
}

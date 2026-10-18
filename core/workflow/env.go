package workflow

import (
	"context"
	"errors"
	"fmt"
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
	// Redact 抹去文本中本次任务使用的令牌及其片段；作用于翻译错误、失败记录、进度事件与日志。
	Redact func(string) string
}

func (e *Env) log() logging.Logger {
	return redactingLogger{inner: logging.OrNop(e.Logger), redact: e.redact}
}

// redact 先按模式抹去形似令牌的片段，再抹去本次任务实际使用的令牌。
func (e *Env) redact(s string) string {
	s = RedactSecrets(s)
	if e.Redact != nil {
		s = e.Redact(s)
	}
	return s
}

// sanitize 把错误转为可以安全展示的字符串。
func (e *Env) sanitize(err error) string {
	if err == nil {
		return ""
	}
	return e.redact(err.Error())
}

// redactingLogger 在输出前对格式化后的日志脱敏。
type redactingLogger struct {
	inner  logging.Logger
	redact func(string) string
}

func (l redactingLogger) Debugf(f string, a ...any) {
	l.inner.Debugf("%s", l.redact(fmt.Sprintf(f, a...)))
}
func (l redactingLogger) Infof(f string, a ...any) {
	l.inner.Infof("%s", l.redact(fmt.Sprintf(f, a...)))
}
func (l redactingLogger) Warnf(f string, a ...any) {
	l.inner.Warnf("%s", l.redact(fmt.Sprintf(f, a...)))
}

// redactingTranslator 对翻译后端返回的错误脱敏（后端错误可能回显 Authorization 头或 key 片段）。
type redactingTranslator struct {
	inner  core.Translator
	redact func(string) string
}

type redactedError struct {
	msg   string
	cause error
}

func (e *redactedError) Error() string { return e.msg }

// Is 保留 context.Canceled 等哨兵错误的判断能力，但不暴露原始错误文本。
func (e *redactedError) Is(target error) bool { return errors.Is(e.cause, target) }

func (t redactingTranslator) wrap(err error) error {
	if err == nil {
		return nil
	}
	return &redactedError{msg: t.redact(err.Error()), cause: err}
}

func (t redactingTranslator) Translate(ctx context.Context, text, from, to string) (string, error) {
	r, err := t.inner.Translate(ctx, text, from, to)
	return r, t.wrap(err)
}

func (t redactingTranslator) TranslateBatch(ctx context.Context, texts []string, from, to string) ([]string, error) {
	r, err := t.inner.TranslateBatch(ctx, texts, from, to)
	return r, t.wrap(err)
}

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

// translator 返回注入的翻译器，或按配置构造一个；结果总是包装为对错误脱敏的翻译器。
func (e *Env) translator() (core.Translator, error) {
	t := e.Translator
	if t == nil {
		key := e.Key
		if key == nil {
			key = translator.EnvKey
		}
		var err error
		if t, err = translator.New(translator.SettingsFrom(e.Config, e.log()), key); err != nil {
			return nil, &redactedError{msg: e.sanitize(err), cause: err}
		}
	}
	return redactingTranslator{inner: t, redact: e.redact}, nil
}

// loadStore 读取项目的 mappings.json。
func (e *Env) loadStore() (*mapping.Store, error) {
	store := mapping.NewStore(e.MappingsPath())
	if err := store.Load(); err != nil {
		return nil, err
	}
	return store, nil
}

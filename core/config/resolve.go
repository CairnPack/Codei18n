package config

import (
	"reflect"
	"strings"
)

// Layer 表示配置值来自哪一层。
type Layer string

const (
	LayerDefault  Layer = "default"  // 内置默认
	LayerGlobal   Layer = "global"   // ~/.codei18n/config.json（含其中定义的 profile）
	LayerProject  Layer = "project"  // <root>/.codei18n/config.json
	LayerOverride Layer = "override" // 调用方覆盖（CLI flags / GUI 临时设置）
)

// Value 是合并后的值及其来源层。
type Value[T any] struct {
	Value  T     `json:"value"`
	Source Layer `json:"source"`
}

func (v *Value[T]) set(val T, src Layer) { v.Value, v.Source = val, src }

// Provider 是合并后的翻译后端设置。
type Provider struct {
	Name        Value[string] `json:"provider"`
	BaseURL     Value[string] `json:"baseUrl"`
	Model       Value[string] `json:"model"`
	Endpoint    Value[string] `json:"endpoint"`
	BatchSize   Value[int]    `json:"batchSize"`
	Concurrency Value[int]    `json:"concurrency"`
	// APIKeyRef 为空表示使用按 profile 名推导的默认引用。
	APIKeyRef Value[string] `json:"apiKeyRef"`
}

// Effective 是合并后的有效配置，每个字段都带来源层，供 GUI 显示继承关系。
type Effective struct {
	ProjectID       string          `json:"projectId,omitempty"`
	SourceLanguage  Value[string]   `json:"sourceLanguage"`
	LocalLanguage   Value[string]   `json:"localLanguage"`
	ExcludePatterns Value[[]string] `json:"excludePatterns"`
	DetectLanguages Value[[]string] `json:"detectLanguages"`
	// Profile 是选用的配置档名；为空表示未选用（使用旧版字段或默认值）。
	Profile Value[string] `json:"profile"`
	// ProfileMissing 表示选用的配置档在全局配置中不存在。
	ProfileMissing bool     `json:"profileMissing,omitempty"`
	Provider       Provider `json:"providerSettings"`
}

// Overrides 是调用方的临时覆盖（CLI flags / GUI 临时设置），优先级最高，不写入任何文件。
type Overrides struct {
	SourceLanguage  *string
	LocalLanguage   *string
	ExcludePatterns []string
	Profile         *string
	Provider        ProviderFields
}

// 内置默认值。
const (
	DefaultSourceLanguage = "en"
	DefaultLocalLanguage  = "zh-CN"
	DefaultProvider       = "openai"
	DefaultOpenAIModel    = "gpt-3.5-turbo"
	DefaultOllamaModel    = "llama3"
	DefaultOllamaEndpoint = "http://localhost:11434"
	DefaultBatchSize      = 10
	DefaultConcurrency    = 5
	// DefaultProfileName 是未选用任何配置档时，令牌引用使用的配置档名。
	DefaultProfileName = "default"
)

// DefaultExcludePatterns 返回默认排除规则（.codei18n/** 已覆盖缓存目录）。
func DefaultExcludePatterns() []string {
	return []string{".git/**", "vendor/**", ".codei18n/**"}
}

// DefaultDetectLanguages 返回默认的语种识别候选集合。
func DefaultDetectLanguages() []string {
	return []string{"en", "zh-Hans", "zh-Hant", "ja", "ko", "de", "fr", "es", "ru"}
}

// Resolve 按 内置默认 < 全局 < 项目 < 调用方覆盖 合并配置。global、project 可以为 nil。
//
// provider 设置的合并顺序为：默认 < 全局旧版字段 < 全局中选用的 profile < 项目旧版字段 < 项目 profileOverrides < 调用方覆盖。
// 项目显式选用了 profile 时，项目中的旧版 provider 字段（例如旧 init 生成的快照）被忽略，以免遮住所选 profile。
func Resolve(global, project *File, ov Overrides) *Effective {
	if global == nil {
		global = &File{}
	}
	if project == nil {
		project = &File{}
	}
	e := &Effective{ProjectID: project.ProjectID}
	e.SourceLanguage.set(DefaultSourceLanguage, LayerDefault)
	e.LocalLanguage.set(DefaultLocalLanguage, LayerDefault)
	e.ExcludePatterns.set(DefaultExcludePatterns(), LayerDefault)
	e.DetectLanguages.set(DefaultDetectLanguages(), LayerDefault)
	e.Profile.set("", LayerDefault)
	p := &e.Provider
	p.Name.set(DefaultProvider, LayerDefault)
	p.BatchSize.set(DefaultBatchSize, LayerDefault)
	p.Concurrency.set(DefaultConcurrency, LayerDefault)

	applyCommon := func(f *File, src Layer) {
		setStr(&e.SourceLanguage, f.SourceLanguage, src)
		setStr(&e.LocalLanguage, f.LocalLanguage, src)
		if f.ExcludePatterns != nil {
			e.ExcludePatterns.set(append([]string(nil), f.ExcludePatterns...), src)
		}
		if f.DetectLanguages != nil {
			e.DetectLanguages.set(append([]string(nil), f.DetectLanguages...), src)
		}
		setStr(&e.Profile, f.Profile, src)
	}
	applyCommon(global, LayerGlobal)
	applyCommon(project, LayerProject)
	setStr(&e.Profile, ov.Profile, LayerOverride)

	applyProvider(p, global.legacyProvider(), LayerGlobal)
	if name := e.Profile.Value; name != "" {
		if prof, ok := global.Profiles[name]; ok && prof != nil {
			applyProvider(p, prof, LayerGlobal)
		} else {
			e.ProfileMissing = true
		}
	}
	if project.Profile == nil {
		applyProvider(p, project.legacyProvider(), LayerProject)
	}
	applyProvider(p, project.ProfileOverrides, LayerProject)

	setStr(&e.SourceLanguage, ov.SourceLanguage, LayerOverride)
	setStr(&e.LocalLanguage, ov.LocalLanguage, LayerOverride)
	if ov.ExcludePatterns != nil {
		e.ExcludePatterns.set(append([]string(nil), ov.ExcludePatterns...), LayerOverride)
	}
	applyProvider(p, &ov.Provider, LayerOverride)

	// 与 provider 相关的默认值在最后补齐，来源仍标记为 default。
	if p.Model.Value == "" {
		switch strings.ToLower(p.Name.Value) {
		case "ollama":
			p.Model.set(DefaultOllamaModel, LayerDefault)
		case "openai", "llm", "llm-api", "":
			p.Model.set(DefaultOpenAIModel, LayerDefault)
		}
	}
	if p.Endpoint.Value == "" && strings.EqualFold(p.Name.Value, "ollama") {
		p.Endpoint.set(DefaultOllamaEndpoint, LayerDefault)
	}
	if p.BatchSize.Value <= 0 {
		p.BatchSize.set(DefaultBatchSize, LayerDefault)
	}
	if p.Concurrency.Value <= 0 {
		p.Concurrency.set(DefaultConcurrency, LayerDefault)
	}
	return e
}

// ProfileName 返回令牌引用使用的配置档名：未选用时为 "default"。
func (e *Effective) ProfileName() string {
	if e.Profile.Value == "" {
		return DefaultProfileName
	}
	return e.Profile.Value
}

// legacyProvider 把旧版顶层字段转换为 ProviderFields。
func (f *File) legacyProvider() *ProviderFields {
	pf := &ProviderFields{Provider: f.TranslationProvider, BatchSize: f.BatchSize, Concurrency: f.Concurrency}
	if v, ok := lookupCI(f.TranslationConfig, "model"); ok {
		pf.Model = &v
	}
	if v, ok := lookupCI(f.TranslationConfig, "baseurl", "base_url"); ok {
		pf.BaseURL = &v
	}
	if v, ok := lookupCI(f.TranslationConfig, "endpoint"); ok {
		pf.Endpoint = &v
	}
	return pf
}

// lookupCI 不区分大小写地查找第一个非空值。
func lookupCI(m map[string]string, keys ...string) (string, bool) {
	for _, want := range keys {
		for k, v := range m {
			if strings.EqualFold(k, want) && v != "" {
				return v, true
			}
		}
	}
	return "", false
}

func applyProvider(p *Provider, f *ProviderFields, src Layer) {
	if f == nil {
		return
	}
	setStr(&p.Name, f.Provider, src)
	setStr(&p.BaseURL, f.BaseURL, src)
	setStr(&p.Model, f.Model, src)
	setStr(&p.Endpoint, f.Endpoint, src)
	setInt(&p.BatchSize, f.BatchSize, src)
	setInt(&p.Concurrency, f.Concurrency, src)
	setStr(&p.APIKeyRef, f.APIKeyRef, src)
}

func setStr(v *Value[string], s *string, src Layer) {
	if s != nil && strings.TrimSpace(*s) != "" {
		v.set(strings.TrimSpace(*s), src)
	}
}

func setInt(v *Value[int], n *int, src Layer) {
	if n != nil && *n > 0 {
		v.set(*n, src)
	}
}

// tagNames 返回结构体所有字段的 json 名称。
func tagNames(v any) []string {
	t := reflect.TypeOf(v)
	var names []string
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		names = append(names, strings.Split(tag, ",")[0])
	}
	return names
}

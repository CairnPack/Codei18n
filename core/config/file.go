package config

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/CairnPack/codei18n/internal/atomicfile"
)

// ProviderFields 是翻译后端的一组可选设置。nil 表示"本层未设置，继承上一层"。
// 用于全局 profile 定义、项目对 profile 的单字段覆盖，以及调用方临时覆盖。
type ProviderFields struct {
	Provider    *string `json:"provider,omitempty"`
	BaseURL     *string `json:"baseUrl,omitempty"`
	Model       *string `json:"model,omitempty"`
	Endpoint    *string `json:"endpoint,omitempty"`
	BatchSize   *int    `json:"batchSize,omitempty"`
	Concurrency *int    `json:"concurrency,omitempty"`
	// APIKeyRef 是令牌在系统凭据库中的引用名（account），不是令牌本身。
	APIKeyRef *string `json:"apiKeyRef,omitempty"`

	extra map[string]json.RawMessage
}

// IsZero 判断是否没有设置任何字段。
func (p *ProviderFields) IsZero() bool {
	return p == nil || (p.Provider == nil && p.BaseURL == nil && p.Model == nil && p.Endpoint == nil &&
		p.BatchSize == nil && p.Concurrency == nil && p.APIKeyRef == nil && len(p.extra) == 0)
}

// Extra 返回未识别的字段（只读副本）。
func (p *ProviderFields) Extra() map[string]json.RawMessage { return copyRaw(p.extra) }

// DeleteExtra 删除一个未识别字段，例如迁移旧的明文令牌字段。
func (p *ProviderFields) DeleteExtra(key string) { delete(p.extra, key) }

type providerFieldsJSON ProviderFields

func (p *ProviderFields) UnmarshalJSON(data []byte) error {
	var known providerFieldsJSON
	extra, err := splitUnknown(data, &known, providerKnownKeys)
	if err != nil {
		return err
	}
	*p = ProviderFields(known)
	p.extra = extra
	return nil
}

func (p ProviderFields) MarshalJSON() ([]byte, error) {
	return joinUnknown(providerFieldsJSON(p), p.extra)
}

var providerKnownKeys = jsonKeys(providerFieldsJSON{})

// File 是单个配置文件（全局 ~/.codei18n/config.json 或项目 .codei18n/config.json）的内容。
// 指针 / nil 切片表示本层未设置。读写时保留未识别的字段，避免重写配置时丢失数据。
type File struct {
	// ProjectID 是 init 时生成的 UUID，用于项目级令牌的命名（不是秘密，可以入库）。仅项目配置使用。
	ProjectID string `json:"projectId,omitempty"`

	SourceLanguage  *string  `json:"sourceLanguage,omitempty"`
	LocalLanguage   *string  `json:"localLanguage,omitempty"`
	ExcludePatterns []string `json:"excludePatterns,omitempty"`
	// DetectLanguages 是语种识别的候选语言集合（BCP 47）。
	DetectLanguages []string `json:"detectLanguages,omitempty"`

	// 旧版字段：仍然支持读取，含义等同于本层对 provider 设置的覆盖。
	TranslationProvider *string           `json:"translationProvider,omitempty"`
	TranslationConfig   map[string]string `json:"translationConfig,omitempty"`
	BatchSize           *int              `json:"batchSize,omitempty"`
	Concurrency         *int              `json:"concurrency,omitempty"`

	// Profile 选用的 provider 配置档名称。
	Profile *string `json:"profile,omitempty"`
	// Profiles 定义命名配置档（只在全局配置中生效）。
	Profiles map[string]*ProviderFields `json:"profiles,omitempty"`
	// ProfileOverrides 项目对所选配置档的单字段覆盖（只在项目配置中生效）。
	ProfileOverrides *ProviderFields `json:"profileOverrides,omitempty"`

	extra map[string]json.RawMessage
}

type fileJSON File

var fileKnownKeys = jsonKeys(fileJSON{})

func (f *File) UnmarshalJSON(data []byte) error {
	var known fileJSON
	extra, err := splitUnknown(data, &known, fileKnownKeys)
	if err != nil {
		return err
	}
	*f = File(known)
	f.extra = extra
	return nil
}

func (f File) MarshalJSON() ([]byte, error) {
	if f.ProfileOverrides.IsZero() {
		f.ProfileOverrides = nil
	}
	return joinUnknown(fileJSON(f), f.extra)
}

// Extra 返回未识别的字段（只读副本）。
func (f *File) Extra() map[string]json.RawMessage { return copyRaw(f.extra) }

// DeleteExtra 删除一个未识别字段。
func (f *File) DeleteExtra(key string) { delete(f.extra, key) }

// ReadFile 读取配置文件；文件不存在时返回空的 File 与 nil。
func ReadFile(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &File{}, nil
		}
		return nil, err
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if len(bytes.TrimSpace(data)) == 0 {
		return &File{}, nil
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
	}
	return &f, nil
}

// WriteFile 原子写出配置文件。
func WriteFile(path string, f *File) error {
	return atomicfile.WriteJSON(path, f)
}

// UpdateFile 读取 path、调用 fn 修改后原子写回。fn 返回错误时不写入。
func UpdateFile(path string, fn func(*File) error) error {
	f, err := ReadFile(path)
	if err != nil {
		return err
	}
	if err := fn(f); err != nil {
		return err
	}
	return WriteFile(path, f)
}

// NewProjectID 生成随机 UUID（版本 4）。
func NewProjectID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand 失败意味着系统熵源不可用
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// splitUnknown 把 data 解码到 known，并返回 known 不认识的顶层字段。
func splitUnknown(data []byte, known any, knownKeys map[string]bool) (map[string]json.RawMessage, error) {
	if err := json.Unmarshal(data, known); err != nil {
		return nil, err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, err
	}
	var extra map[string]json.RawMessage
	for k, v := range all {
		if knownKeys[k] {
			continue
		}
		if extra == nil {
			extra = map[string]json.RawMessage{}
		}
		extra[k] = v
	}
	return extra, nil
}

// joinUnknown 先按结构体字段顺序编码 known，再按键名顺序追加未识别字段。
func joinUnknown(known any, extra map[string]json.RawMessage) ([]byte, error) {
	data, err := json.Marshal(known)
	if err != nil || len(extra) == 0 {
		return data, err
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	buf.Write(data[:len(data)-1]) // 去掉结尾的 '}'
	first := len(bytes.TrimSpace(data)) == 2
	for _, k := range keys {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		kb, _ := json.Marshal(k)
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(extra[k])
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func jsonKeys(v any) map[string]bool {
	data, _ := json.Marshal(v)
	keys := map[string]bool{}
	var m map[string]json.RawMessage
	_ = json.Unmarshal(data, &m)
	for k := range m {
		keys[k] = true
	}
	// omitempty 字段在零值时不会出现，补齐所有 json 标签。
	for _, k := range tagNames(v) {
		keys[k] = true
	}
	return keys
}

func copyRaw(m map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

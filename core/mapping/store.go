package mapping

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"sync"

	"github.com/CairnPack/codei18n/core/domain"
	"github.com/CairnPack/codei18n/internal/atomicfile"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// FileName 是映射文件名。
const FileName = "mappings.json"

// Store 管理 mappings.json 的读写与并发访问。所有写盘都是原子的（临时文件 + rename）。
type Store struct {
	mu      sync.RWMutex
	mapping *domain.Mapping
	path    string
}

// NewStore 创建映射存储；path 为空时只在内存中使用。
func NewStore(path string) *Store {
	return &Store{
		path: path,
		mapping: &domain.Mapping{
			Version:        "1.0",
			SourceLanguage: "en",
			TargetLanguage: "zh-CN", // 默认值，通常由配置覆盖
			Comments:       make(map[string]map[string]string),
		},
	}
}

// Path 返回映射文件路径。
func (s *Store) Path() string { return s.path }

// Load 从磁盘读取映射文件；文件不存在时保持空映射。
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	data = bytes.TrimPrefix(data, utf8BOM)
	if err := json.Unmarshal(data, s.mapping); err != nil {
		return err
	}
	if s.mapping.Comments == nil {
		s.mapping.Comments = make(map[string]map[string]string)
	}
	return nil
}

// Save 把映射原子写入磁盘。
func (s *Store) Save() error {
	return s.SaveSnapshot(s.Snapshot())
}

// Snapshot 返回当前映射的深拷贝，可在不持有锁的情况下编码写盘。
func (s *Store) Snapshot() *domain.Mapping {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := *s.mapping
	m.Comments = make(map[string]map[string]string, len(s.mapping.Comments))
	for id, langs := range s.mapping.Comments {
		c := make(map[string]string, len(langs))
		for k, v := range langs {
			c[k] = v
		}
		m.Comments[id] = c
	}
	return &m
}

// SaveSnapshot 原子写出一个快照（不持有 Store 的锁）。
func (s *Store) SaveSnapshot(m *domain.Mapping) error {
	return atomicfile.WriteJSON(s.path, m)
}

// Get 读取某条注释某种语言的文本。
func (s *Store) Get(id, lang string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if translations, ok := s.mapping.Comments[id]; ok {
		if text, ok := translations[lang]; ok {
			return text, true
		}
	}
	return "", false
}

// Languages 返回某条注释的所有语言文本副本；不存在时返回 nil。
func (s *Store) Languages(id string) map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	langs, ok := s.mapping.Comments[id]
	if !ok {
		return nil
	}
	out := make(map[string]string, len(langs))
	for k, v := range langs {
		out[k] = v
	}
	return out
}

// Has 判断是否存在某条注释。
func (s *Store) Has(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.mapping.Comments[id]
	return ok
}

// Set 添加或更新一条文本（去掉结尾换行）。
func (s *Store) Set(id, lang, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.mapping.Comments[id]; !ok {
		s.mapping.Comments[id] = make(map[string]string)
	}
	cleanText := strings.TrimRight(text, "\r\n")
	s.mapping.Comments[id][lang] = cleanText
}

// DeleteLang 删除某条注释的某种语言文本。
func (s *Store) DeleteLang(id, lang string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if langs, ok := s.mapping.Comments[id]; ok {
		delete(langs, lang)
	}
}

// Delete 删除一条注释的全部映射。
func (s *Store) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.mapping.Comments, id)
}

// IDs 返回所有注释 ID。
func (s *Store) IDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.mapping.Comments))
	for id := range s.mapping.Comments {
		ids = append(ids, id)
	}
	return ids
}

// SetLanguages 更新映射头部的源语言与目标语言。
func (s *Store) SetLanguages(source, target string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mapping.SourceLanguage = source
	s.mapping.TargetLanguage = target
}

// GetMapping 返回底层映射对象。调用方不得在其他 goroutine 写入 Store 时读写它；
// 并发场景请使用 Snapshot。
func (s *Store) GetMapping() *domain.Mapping {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mapping
}

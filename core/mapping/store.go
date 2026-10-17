package mapping

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/CairnPack/codei18n/core/domain"
)

// Store manages the persistence and concurrent access of mappings
type Store struct {
	mu      sync.RWMutex
	mapping *domain.Mapping
	path    string
}

// NewStore creates a new mapping store
func NewStore(path string) *Store {
	return &Store{
		path: path,
		mapping: &domain.Mapping{
			Version:        "1.0",
			SourceLanguage: "en",
			TargetLanguage: "zh-CN", // Default, should be updated from config
			Comments:       make(map[string]map[string]string),
		},
	}
}

// Load reads the mapping file from disk
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			// If file doesn't exist, we start with empty mapping (initialized in NewStore)
			return nil
		}
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(s.mapping); err != nil {
		return err
	}

	// Ensure map is initialized if file had null
	if s.mapping.Comments == nil {
		s.mapping.Comments = make(map[string]map[string]string)
	}

	return nil
}

// Save writes the mapping to disk
func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	file, err := os.Create(s.path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(s.mapping)
}

// Get retrieves a translation for a given comment ID and language
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

// Set adds or updates a translation
func (s *Store) Set(id, lang, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.mapping.Comments[id]; !ok {
		s.mapping.Comments[id] = make(map[string]string)
	}
	cleanText := strings.TrimRight(text, "\r\n")
	s.mapping.Comments[id][lang] = cleanText
}

// Delete removes a comment mapping by ID
func (s *Store) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.mapping.Comments, id)
}

// GetMapping returns the underlying mapping object (read-only copy recommended for complex ops)
// For now returning pointer for simplicity in MVP
func (s *Store) GetMapping() *domain.Mapping {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mapping
}

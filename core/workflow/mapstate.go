package workflow

import (
	"bytes"
	"crypto/sha1"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/CairnPack/codei18n/core/index"
	"github.com/CairnPack/codei18n/core/langdetect"
	"github.com/CairnPack/codei18n/core/scanner"
	"github.com/CairnPack/codei18n/internal/atomicfile"
)

// 缓存目录中的文件名。
const (
	ScanCacheFileName = "scan.gob"
	mapStateFileName  = "mapstate.json"
	langCacheFileName = "lang.gob"
)

// fileStat 记录文件的 size 与 mtime，用于判断文件是否被外部修改过。
type fileStat struct {
	Exists  bool  `json:"exists"`
	Size    int64 `json:"size"`
	ModTime int64 `json:"modTime"`
}

func statOf(path string) fileStat {
	st, err := os.Stat(path)
	if err != nil {
		return fileStat{}
	}
	return fileStat{Exists: true, Size: st.Size(), ModTime: st.ModTime().UnixNano()}
}

// mapState 记录上一次 map update 的输入与输出指纹。下一次扫描全部命中缓存、且这些指纹都没变时，
// 结果必然相同，可以直接返回而不重算（无改动重扫的快速路径）。
type mapState struct {
	Version     int      `json:"version"`
	Prefix      string   `json:"prefix"`
	ScanDigest  string   `json:"scanDigest"`
	IndexDigest string   `json:"indexDigest"`
	Source      string   `json:"source"`
	Local       string   `json:"local"`
	Detector    string   `json:"detector"`
	Mappings    fileStat `json:"mappings"`
	Index       fileStat `json:"index"`
	Overrides   fileStat `json:"overrides"`
	Total       int      `json:"total"`
	Orphans     int      `json:"orphans"`
}

const mapStateVersion = 1

func readMapState(dir string) *mapState {
	data, err := os.ReadFile(filepath.Join(dir, mapStateFileName))
	if err != nil {
		return nil
	}
	var s mapState
	if json.Unmarshal(data, &s) != nil || s.Version != mapStateVersion {
		return nil
	}
	return &s
}

func writeMapState(dir string, s *mapState) error {
	s.Version = mapStateVersion
	return atomicfile.WriteJSON(filepath.Join(dir, mapStateFileName), s)
}

// scanDigest 对扫描结果中的（路径, 内容 hash）序列取摘要：文件增删改都会改变它。
func scanDigest(prefix string, r *scanner.Result) string {
	h := sha1.New()
	h.Write([]byte(prefix))
	for _, f := range r.Files {
		h.Write([]byte{0})
		h.Write([]byte(f.Path))
		h.Write([]byte{0})
		h.Write([]byte(f.ContentHash))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// indexDigest 对索引的语义内容取摘要（不含 generatedAt / generator），用于判断是否需要重写 index.json。
func indexDigest(idx *index.Index) string {
	h := sha1.New()
	w := func(s string) { h.Write([]byte(s)); h.Write([]byte{0}) }
	w(idx.SourceLanguage)
	w(idx.LocalLanguage)
	paths := make([]string, 0, len(idx.Files))
	for p := range idx.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		f := idx.Files[p]
		w(p)
		w(f.ContentHash)
		w(f.Language)
		for _, c := range f.Comments {
			w(c.ID)
			for _, n := range []int{c.Range.StartLine, c.Range.StartCol, c.Range.EndLine, c.Range.EndCol, c.Parts} {
				w(strconv.Itoa(n))
			}
			w(c.Type)
			w(c.Lang)
			w(c.LangSource)
			w(c.SourceKey)
			w(c.TextHash)
			w(c.Symbol)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// langCache 按归一化文本 hash 缓存语种识别结果。识别器标识变化时整体失效；保存时只保留本次用到的条目。
type langCache struct {
	path  string
	key   string
	old   map[string]langdetect.Result
	used  map[string]langdetect.Result
	added bool
}

type langCacheFile struct {
	Key     string
	Entries map[string]langdetect.Result
}

func openLangCache(dir string, d langdetect.Detector) *langCache {
	k, ok := d.(langdetect.CacheKeyer)
	if !ok {
		return nil
	}
	c := &langCache{path: filepath.Join(dir, langCacheFileName), key: k.CacheKey(), old: map[string]langdetect.Result{}, used: map[string]langdetect.Result{}}
	if data, err := os.ReadFile(c.path); err == nil {
		var f langCacheFile
		if gob.NewDecoder(bytes.NewReader(data)).Decode(&f) == nil && f.Key == c.key && f.Entries != nil {
			c.old = f.Entries
		}
	}
	return c
}

func (c *langCache) detect(d langdetect.Detector, textHash, text string) langdetect.Result {
	if c == nil {
		return d.Detect(text)
	}
	if r, ok := c.used[textHash]; ok {
		return r
	}
	r, ok := c.old[textHash]
	if !ok {
		r = d.Detect(text)
		c.added = true
	}
	c.used[textHash] = r
	return r
}

func (c *langCache) save() error {
	if c == nil || (!c.added && len(c.used) == len(c.old)) {
		return nil
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(langCacheFile{Key: c.key, Entries: c.used}); err != nil {
		return err
	}
	return atomicfile.WriteFile(c.path, buf.Bytes(), 0o644)
}

func detectorKey(d langdetect.Detector) string {
	if k, ok := d.(langdetect.CacheKeyer); ok {
		return k.CacheKey()
	}
	return ""
}

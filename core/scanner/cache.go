package scanner

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/CairnPack/codei18n/core/domain"
	"github.com/CairnPack/codei18n/internal/atomicfile"
)

// ParserVersion 标识解析规则的版本。任何 adapter 或合并规则的变化都必须修改它，使旧缓存失效。
const ParserVersion = "2026-10-a2-2"

const cacheFormat = 3

// Cache 是增量扫描缓存：按文件路径记录 size、mtime、内容 hash 与上次的解析结果。并发安全。
// 缓存只是加速手段，损坏或版本不符时整体丢弃、重新解析。
//
// 为了让"无改动重扫"足够快：打开时只解码每个文件的元数据，注释以紧凑二进制块保存，命中时才在
// 扫描 worker 中解码；没有任何变化时 Save 不写盘。
type Cache struct {
	path string

	mu      sync.Mutex
	loaded  map[string]*cacheEntry
	next    map[string]*cacheEntry
	savedAt time.Time
	hits    int
	dirty   bool
}

type cacheEntry struct {
	// IDPath 是参与 ID 计算的相对路径（相对于扫描目录）。
	IDPath      string
	Size        int64
	ModTime     int64
	ContentHash string
	Language    string
	// Blob 是注释列表的紧凑编码（见 encodeComments）。
	Blob []byte
}

type cacheFile struct {
	Format        int
	ParserVersion string
	SavedAt       int64
	Files         map[string]*cacheEntry
}

// OpenCache 读取 path 处的缓存；不存在、损坏或版本不符时返回空缓存。
func OpenCache(path string) *Cache {
	c := &Cache{path: path, loaded: map[string]*cacheEntry{}, next: map[string]*cacheEntry{}}
	data, err := os.ReadFile(path)
	if err != nil {
		c.dirty = true
		return c
	}
	var f cacheFile
	if gob.NewDecoder(bytes.NewReader(data)).Decode(&f) != nil ||
		f.Format != cacheFormat || f.ParserVersion != ParserVersion || f.Files == nil {
		c.dirty = true
		return c
	}
	c.loaded = f.Files
	c.savedAt = time.Unix(0, f.SavedAt)
	return c
}

// Hits 返回本次扫描中复用缓存的文件数。
func (c *Cache) Hits() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits
}

// lookup 返回 key 的缓存条目，以及 size 与 mtime 是否可信。若文件的 mtime 离上次写缓存太近
// （同一时钟周期内的改动可能被漏掉），则要求调用方重新计算 hash。
func (c *Cache) lookup(key, idPath string, size, mtime int64) (e *cacheEntry, trustStat bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e = c.loaded[key]
	if e == nil || e.IDPath != idPath {
		return nil, false
	}
	sameStat := e.Size == size && e.ModTime == mtime
	racy := !c.savedAt.IsZero() && time.Unix(0, mtime).After(c.savedAt.Add(-2*time.Second))
	return e, sameStat && !racy
}

func (c *Cache) hit(key string, e *cacheEntry, size, mtime int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e.Size != size || e.ModTime != mtime {
		cp := *e
		cp.Size, cp.ModTime = size, mtime
		e = &cp
		c.dirty = true
	}
	c.next[key] = e
	c.hits++
}

func (c *Cache) store(key, idPath string, size, mtime int64, fr *FileResult) {
	e := &cacheEntry{IDPath: idPath, Size: size, ModTime: mtime,
		ContentHash: fr.ContentHash, Language: fr.Language, Blob: encodeComments(fr.Comments)}
	c.mu.Lock()
	c.next[key] = e
	c.dirty = true
	c.mu.Unlock()
}

// Save 原子写出本次扫描见到的文件（被删除的文件随之从缓存中消失）。没有任何变化时不写盘。
func (c *Cache) Save() error {
	if c == nil || c.path == "" {
		return errors.New("缓存路径为空")
	}
	c.mu.Lock()
	if !c.dirty && len(c.next) == len(c.loaded) {
		c.mu.Unlock()
		return nil
	}
	f := cacheFile{Format: cacheFormat, ParserVersion: ParserVersion, SavedAt: time.Now().UnixNano(), Files: c.next}
	c.mu.Unlock()
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(&f); err != nil {
		return err
	}
	return atomicfile.WriteFile(c.path, buf.Bytes(), 0o644)
}

func (e *cacheEntry) result(path string) (*FileResult, error) {
	comments, err := decodeComments(e.Blob, path)
	if err != nil {
		return nil, err
	}
	return &FileResult{Path: path, Language: e.Language, ContentHash: e.ContentHash, Comments: comments}, nil
}

// encodeComments 把注释列表编码为紧凑的二进制块。File 不编码（解码时用条目的 IDPath 填回）；
// 单条注释的 PartSymbols / PartTexts 与 Symbol / SourceText 相同，也不重复编码。
func encodeComments(cs []*domain.Comment) []byte {
	var b []byte
	putStr := func(s string) {
		b = binary.AppendUvarint(b, uint64(len(s)))
		b = append(b, s...)
	}
	putInt := func(n int) { b = binary.AppendVarint(b, int64(n)) }
	putInt(len(cs))
	for _, c := range cs {
		putStr(c.ID)
		putStr(c.Language)
		putStr(c.Symbol)
		putStr(c.SourceText)
		putStr(c.TextHash)
		putStr(string(c.Type))
		for _, n := range []int{c.Range.StartLine, c.Range.StartCol, c.Range.EndLine, c.Range.EndCol,
			c.IndexRange.StartLine, c.IndexRange.StartCol, c.IndexRange.EndLine, c.IndexRange.EndCol, c.Parts} {
			putInt(n)
		}
		if c.PartCount() > 1 {
			putInt(len(c.PartSymbols))
			for _, s := range c.PartSymbols {
				putStr(s)
			}
			putInt(len(c.PartTexts))
			for _, s := range c.PartTexts {
				putStr(s)
			}
		}
	}
	return b
}

var errCorrupt = errors.New("扫描缓存已损坏")

func decodeComments(b []byte, file string) ([]*domain.Comment, error) {
	var err error
	getInt := func() int {
		v, n := binary.Varint(b)
		if n <= 0 {
			err = errCorrupt
			return 0
		}
		b = b[n:]
		return int(v)
	}
	getStr := func() string {
		l, n := binary.Uvarint(b)
		if n <= 0 || uint64(len(b)-n) < l {
			err = errCorrupt
			return ""
		}
		s := string(b[n : n+int(l)])
		b = b[n+int(l):]
		return s
	}
	count := getInt()
	if err != nil || count < 0 || count > len(b) {
		return nil, fmt.Errorf("%w: 条目数", errCorrupt)
	}
	out := make([]*domain.Comment, count)
	for i := range out {
		c := &domain.Comment{File: file}
		c.ID, c.Language, c.Symbol, c.SourceText = getStr(), getStr(), getStr(), getStr()
		c.TextHash = getStr()
		c.Type = domain.CommentType(getStr())
		c.Range = domain.TextRange{StartLine: getInt(), StartCol: getInt(), EndLine: getInt(), EndCol: getInt()}
		c.IndexRange = domain.TextRange{StartLine: getInt(), StartCol: getInt(), EndLine: getInt(), EndCol: getInt()}
		c.Parts = getInt()
		if c.PartCount() > 1 {
			c.PartSymbols = make([]string, max(0, getInt()))
			for k := range c.PartSymbols {
				c.PartSymbols[k] = getStr()
			}
			c.PartTexts = make([]string, max(0, getInt()))
			for k := range c.PartTexts {
				c.PartTexts[k] = getStr()
			}
		} else {
			c.PartSymbols, c.PartTexts = []string{c.Symbol}, []string{c.SourceText}
		}
		if err != nil {
			return nil, err
		}
		out[i] = c
	}
	return out, nil
}

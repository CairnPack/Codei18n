package langdetect

// CacheKeyer 是可选接口：识别器返回一个标识其规则与参数的字符串（例如候选语言集合与模型版本）。
// 实现了它的识别器，其结果可以按"归一化文本 hash"缓存在 .codei18n/cache 中；标识变化时缓存失效。
type CacheKeyer interface {
	CacheKey() string
}

// CacheKey 实现 CacheKeyer。
func (Legacy) CacheKey() string { return "legacy-1" }

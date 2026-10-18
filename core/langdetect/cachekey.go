package langdetect

import (
	"sort"
	"strings"
)

// CacheKeyer 是可选接口：识别器返回一个标识其规则与参数的字符串（例如候选语言集合与模型版本）。
// 实现了它的识别器，其结果可以按"归一化文本 hash"缓存在 .codei18n/cache 中；标识变化时缓存失效。
type CacheKeyer interface {
	CacheKey() string
}

// CacheKey 实现 CacheKeyer。
func (Legacy) CacheKey() string { return "legacy-1" }

// detectorRulesVersion 标识 lingua 识别器的清洗规则、阈值与模型裁剪方式；任何调整都必须修改它，使识别缓存失效。
const detectorRulesVersion = "lingua-1.4.0-trim1-rules1"

// CacheKey 实现 CacheKeyer：规则版本 + 排序后的候选集合。
func (d *detector) CacheKey() string {
	codes := make([]string, 0, len(d.cands))
	for c := range d.cands {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	return detectorRulesVersion + ":" + strings.Join(codes, ",")
}

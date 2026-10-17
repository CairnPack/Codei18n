// Package events 定义长任务发出的结构化进度事件。
package events

// Phase 表示长任务所处的阶段。
type Phase string

const (
	PhaseDiscover  Phase = "discover"  // 枚举待扫描文件
	PhaseScan      Phase = "scan"      // 解析文件、提取注释
	PhaseMap       Phase = "map"       // 合并到 mappings
	PhaseIndex     Phase = "index"     // 写位置索引
	PhaseTranslate Phase = "translate" // 调用翻译后端
	PhaseSave      Phase = "save"      // 落盘
	PhaseConvert   Phase = "convert"   // 改写源码
	PhaseDone      Phase = "done"      // 结束（Done == Total）
)

// Event 是一条进度事件。Err 只包含已脱敏的错误描述，不会包含令牌。
type Event struct {
	Phase Phase  `json:"phase"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
	File  string `json:"file,omitempty"`
	Err   string `json:"error,omitempty"`
}

// Func 接收进度事件。调用方可能在任意 goroutine 上被回调，需要自行切回 UI 线程；
// 回调应尽快返回，不要在其中做阻塞操作。
type Func func(Event)

// Emit 在 f 非 nil 时发出事件。
func (f Func) Emit(e Event) {
	if f != nil {
		f(e)
	}
}

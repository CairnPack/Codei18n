package workflow

import (
	"sync"
	"time"

	"github.com/CairnPack/codei18n/core/logging"
	"github.com/CairnPack/codei18n/core/mapping"
	"github.com/CairnPack/codei18n/core/utils"
)

// unit 是一次实际的翻译请求：方向相同、归一化原文相同的任务合并为一个 unit，只请求一次，
// 结果写回所有对应 ID。这既省钱，也从源头消除了 IDE 端"同文异译"。
type unit struct {
	text     string
	fromLang string
	toLang   string
	tasks    []task
}

// dedupe 按 (from, to, 归一化原文) 合并任务，保持首次出现的顺序。
func dedupe(tasks []task) []*unit {
	byKey := map[string]*unit{}
	var out []*unit
	for _, t := range tasks {
		key := t.fromLang + "\x00" + t.toLang + "\x00" + utils.NormalizeCommentText(t.text)
		u := byKey[key]
		if u == nil {
			u = &unit{text: t.text, fromLang: t.fromLang, toLang: t.toLang}
			byKey[key] = u
			out = append(out, u)
		}
		u.tasks = append(u.tasks, t)
	}
	return out
}

// 默认的保存节流参数。
const (
	DefaultSaveEvery    = 10
	DefaultSaveInterval = 5 * time.Second
)

// saver 在独立 goroutine 中节流保存 mappings.json：每 every 批或距上次保存超过 interval 时保存一次快照。
// 翻译 goroutine 只调用 batchDone（不阻塞），保存期间翻译继续进行。
type saver struct {
	store    *mapping.Store
	every    int
	interval time.Duration
	log      logging.Logger

	mu       sync.Mutex
	pending  int
	last     time.Time
	saves    int
	signal   chan struct{}
	done     chan struct{}
	exited   chan struct{}
	stopOnce sync.Once
}

func startSaver(store *mapping.Store, every int, interval time.Duration, log logging.Logger) *saver {
	if every <= 0 {
		every = DefaultSaveEvery
	}
	if interval <= 0 {
		interval = DefaultSaveInterval
	}
	s := &saver{store: store, every: every, interval: interval, log: log,
		last: time.Now(), signal: make(chan struct{}, 1), done: make(chan struct{}), exited: make(chan struct{})}
	go s.loop()
	return s
}

// batchDone 记录一个已写入 store 的批次，达到阈值时通知保存 goroutine。
func (s *saver) batchDone() {
	s.mu.Lock()
	s.pending++
	due := s.pending >= s.every || time.Since(s.last) >= s.interval
	if due {
		s.pending = 0
		s.last = time.Now()
	}
	s.mu.Unlock()
	if due {
		select {
		case s.signal <- struct{}{}:
		default: // 已有一次保存待执行，它会包含本批结果
		}
	}
}

func (s *saver) loop() {
	defer close(s.exited)
	for {
		select {
		case <-s.signal:
			if err := s.save(); err != nil {
				s.log.Warnf("保存进度失败: %v", err)
			}
		case <-s.done:
			return
		}
	}
}

func (s *saver) save() error {
	if err := s.store.SaveSnapshot(s.store.Snapshot()); err != nil {
		return err
	}
	s.mu.Lock()
	s.saves++
	s.mu.Unlock()
	return nil
}

// finish 停止后台保存并同步做最后一次保存，返回总保存次数。
func (s *saver) finish() (int, error) {
	s.stopOnce.Do(func() { close(s.done) })
	// 等后台 goroutine 退出（它可能正在用较旧的快照保存），保证最后写盘的是最终状态。
	<-s.exited
	err := s.save()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saves, err
}

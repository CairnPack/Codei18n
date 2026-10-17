package atomicfile

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWriteFileCreatesDirAndReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "x.json")
	if err := WriteFile(path, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "two" {
		t.Fatalf("got %q", got)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("临时文件未清理: %v", entries)
	}
}

// 并发读者在写入过程中只能看到完整的旧内容或完整的新内容。
func TestConcurrentReadersNeverSeePartialJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.json")
	big := map[string]string{}
	for i := 0; i < 2000; i++ {
		big[strings.Repeat("k", 5)+string(rune('a'+i%26))+strings.Repeat("v", i%50)] = strings.Repeat("x", 100)
	}
	if err := WriteJSON(path, big); err != nil {
		t.Fatal(err)
	}
	want, _ := os.ReadFile(path)

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			if err := WriteJSON(path, big); err != nil {
				t.Error(err)
			}
		}
		close(stop)
	}()
	for {
		select {
		case <-stop:
			wg.Wait()
			return
		default:
		}
		got, err := os.ReadFile(path)
		if err == nil && len(got) != len(want) {
			t.Fatalf("读到不完整内容: %d != %d", len(got), len(want))
		}
		// 读者持有文件期间 Windows 会拒绝 rename；让出时间片，模拟 IDE 周期性读取。
		time.Sleep(time.Millisecond)
	}
}

func TestMarshalJSONDoesNotEscapeHTML(t *testing.T) {
	data, err := MarshalJSON(map[string]string{"a": "[MOCK en->zh-CN] <b>"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "en->zh-CN] <b>") {
		t.Fatalf("不应转义: %s", data)
	}
}

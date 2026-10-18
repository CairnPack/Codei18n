package index

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// 与 IDEA 插件共享的归一化测试向量。
func TestNormalizeSharedVectors(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "testdata", "text-normalization.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Cases []struct{ Input, Normalized string } `json:"cases"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Cases) == 0 {
		t.Fatal("测试向量为空")
	}
	for _, c := range v.Cases {
		if got := Normalize(c.Input); got != c.Normalized {
			t.Errorf("Normalize(%q) = %q, want %q", c.Input, got, c.Normalized)
		}
	}
}

func TestTextHashIgnoresIndentationOfGroups(t *testing.T) {
	a := TextHash("// a\n// b")
	b := TextHash("// a\n        // b")
	if a != b {
		t.Fatal("缩进不应影响 textHash")
	}
	if len(a) != 40 {
		t.Fatalf("hash 长度错误: %s", a)
	}
}

func TestUTF16Col(t *testing.T) {
	line := []byte("    // 你好😀x")
	// "    // " 7 字节 → 列 8；"你好" 各 3 字节 1 码元；😀 4 字节 2 码元
	cases := []struct{ byteCol, want int }{
		{1, 1}, {5, 5}, {8, 8}, {11, 9}, {14, 10}, {18, 12}, {19, 13}, {100, 13},
	}
	for _, c := range cases {
		if got := UTF16Col(line, c.byteCol); got != c.want {
			t.Errorf("UTF16Col(%d) = %d, want %d", c.byteCol, got, c.want)
		}
	}
}

func TestRangeFromBytesHandlesCRLF(t *testing.T) {
	lines := Lines([]byte("a\r\n  // 你好\r\nb"))
	r := RangeFromBytes(lines, 2, 3, 2, 3+len("// 你好"))
	if r != (Range{StartLine: 2, StartCol: 3, EndLine: 2, EndCol: 8}) {
		t.Fatalf("got %+v", r)
	}
}

func TestReadWriteRoundTripAndVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if idx, err := Read(path); err != nil || idx != nil {
		t.Fatalf("不存在时应返回 nil, nil: %v %v", idx, err)
	}
	idx := New()
	idx.Files["a.go"] = &File{ContentHash: ContentHash([]byte("x")), Language: "go", Comments: []Comment{
		{ID: "2", Range: Range{StartLine: 5, StartCol: 1}, Parts: 1},
		{ID: "1", Range: Range{StartLine: 1, StartCol: 1}, Parts: 1},
	}}
	if err := Write(path, idx); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Files["a.go"].Comments[0].ID != "1" {
		t.Fatal("写出前应按位置排序")
	}
	if p, c := got.Find("2"); p != "a.go" || c == nil {
		t.Fatal("Find 失败")
	}
	os.WriteFile(path, []byte(`{"version":"2","files":{}}`), 0o644)
	if _, err := Read(path); err == nil {
		t.Fatal("不认识的主版本应报错")
	}
}

func TestOverridesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), OverridesFileName)
	o, err := ReadOverrides(path)
	if err != nil || len(o.Languages) != 0 {
		t.Fatal(err)
	}
	o.Languages["id1"] = "ja"
	if err := WriteOverrides(path, o); err != nil {
		t.Fatal(err)
	}
	o2, _ := ReadOverrides(path)
	if o2.Languages["id1"] != "ja" {
		t.Fatal("round trip 失败")
	}
}

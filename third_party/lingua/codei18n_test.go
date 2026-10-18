package lingua

// 本文件由 CodeI18n 新增：上游测试依赖全部 75 种语言的模型，未随裁剪版保留；
// 这里只覆盖本副本的改动点（手写解码器、模型清单、无 decimal 的置信度计算）。

import (
	"encoding/binary"
	"math"
	"reflect"
	"testing"
)

// appendTag 追加 protobuf 字段标签。
func appendTag(b []byte, field, wire uint64) []byte {
	return binary.AppendUvarint(b, field<<3|wire)
}

func appendBytes(b []byte, field uint64, data []byte) []byte {
	b = appendTag(b, field, wireBytes)
	b = binary.AppendUvarint(b, uint64(len(data)))
	return append(b, data...)
}

func TestDecodeLanguageModel(t *testing.T) {
	ngramSet := func(p float64, ngrams ...string) []byte {
		var b []byte
		for _, n := range ngrams {
			b = appendBytes(b, 2, []byte(n))
		}
		// 概率字段故意放在 ngram 之后，验证字段顺序无关。
		b = appendTag(b, 1, wireFixed64)
		return binary.LittleEndian.AppendUint64(b, math.Float64bits(p))
	}
	var data []byte
	data = appendTag(data, 1, wireVarint)
	data = binary.AppendUvarint(data, 17)
	data = appendTag(data, 2, wireVarint)
	data = binary.AppendUvarint(data, 3)
	data = appendTag(data, 3, wireVarint)
	data = binary.AppendUvarint(data, 3)
	data = appendTag(data, 9, wireFixed32) // 未知字段应被跳过
	data = append(data, 1, 2, 3, 4)
	data = appendBytes(data, 4, ngramSet(0.5, "abc", "bcd"))
	data = appendBytes(data, 4, ngramSet(0.25, "xyz"))

	got, err := decodeLanguageModel(data)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"abc": 0.5, "bcd": 0.5, "xyz": 0.25}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("解码结果 %v，期望 %v", got, want)
	}

	if _, err := decodeLanguageModel(data[:len(data)-3]); err == nil {
		t.Fatal("截断的数据应返回错误")
	}
}

func TestAvailableLanguages(t *testing.T) {
	want := []Language{Chinese, English, French, German, Japanese, Korean, Russian, Spanish}
	got := AvailableLanguages()
	if len(got) != len(want) {
		t.Fatalf("内嵌语言 %v，期望 %v", got, want)
	}
	set := map[Language]bool{}
	for _, l := range got {
		set[l] = true
	}
	for _, l := range want {
		if !set[l] {
			t.Errorf("缺少语言 %v", l)
		}
	}
}

func TestEmbeddedModelsLoad(t *testing.T) {
	for _, l := range AvailableLanguages() {
		if m := loadProtobufData(l, 1); m == nil {
			t.Errorf("%v 缺少 unigram 模型", l)
		}
	}
	for _, l := range []Language{English, German, French, Spanish, Russian} {
		for n := 1; n <= 4; n++ {
			if loadProtobufData(l, n) == nil {
				t.Errorf("%v 缺少 %d-gram 模型", l, n)
			}
		}
		if loadProtobufData(l, 5) != nil {
			t.Errorf("%v 的 fivegram 模型应已被裁剪", l)
		}
	}
}

func TestDetectBasic(t *testing.T) {
	detector := NewLanguageDetectorBuilder().
		FromLanguages(AvailableLanguages()...).
		WithPreloadedLanguageModels().
		Build()
	cases := map[string]Language{
		"Returns the number of bytes written to the underlying buffer": English,
		"Gibt die Anzahl der geschriebenen Bytes zurück":               German,
		"Retourne le nombre d'octets écrits dans le tampon":            French,
		"Devuelve el número de elementos que hay en la lista":          Spanish,
		"Возвращает количество записанных байтов в буфер":              Russian,
		"初始化连接池然后处理请求":                                                 Chinese,
		"設定ファイルを読み込む":                                                  Japanese,
		"설정 파일을 읽습니다":                                                  Korean,
	}
	for text, want := range cases {
		got, ok := detector.DetectLanguageOf(text)
		if !ok || got != want {
			t.Errorf("%q：得到 %v（%v），期望 %v", text, got, ok, want)
		}
		values := detector.ComputeLanguageConfidenceValues(text)
		sum := 0.0
		for _, v := range values {
			sum += v.Value()
		}
		if math.Abs(sum-1) > 1e-9 {
			t.Errorf("%q：置信度之和 %v 不为 1", text, sum)
		}
	}
}

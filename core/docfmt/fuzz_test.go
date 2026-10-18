package docfmt

import (
	"strings"
	"testing"
)

// FuzzRoundTrip：任意可识别的注释都不应 panic，且 Restore(Body()) 必须原样还原（不含 \r 的输入）。
func FuzzRoundTrip(f *testing.F) {
	for _, c := range roundTripCases {
		f.Add(c.comment)
	}
	for _, c := range bodyCases {
		f.Add(c.comment)
	}
	formats := []Format{None, Javadoc, KDoc, JSDoc, Dartdoc, CSharpXML}
	f.Fuzz(func(t *testing.T, comment string) {
		for _, ft := range formats {
			p, err := Prepare(ft, comment)
			if err != nil {
				return
			}
			got, err := p.Restore(p.Body())
			if err != nil {
				t.Fatalf("%s: Restore(Body()) 失败：%v（正文 %q）", ft, err, p.Body())
			}
			if !strings.Contains(comment, "\r") && got != comment {
				t.Fatalf("%s: 往返不一致：\n得到 %q\n期望 %q", ft, got, comment)
			}
		}
	})
}

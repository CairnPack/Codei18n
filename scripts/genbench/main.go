// genbench 生成性能基准用的中型样例项目。
//
//	go run ./scripts/genbench -out <dir> [-files 2000] [-seed 1] [-legacy]
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/CairnPack/codei18n/internal/benchgen"
)

func main() {
	out := flag.String("out", "", "输出目录（必须为空或不存在）")
	files := flag.Int("files", 2000, "文件数")
	seed := flag.Int64("seed", 1, "随机种子")
	legacy := flag.Bool("legacy", false, "只生成旧版本也支持的语言（go/java/kt/ts/js/rs），用于与重构前对比")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "必须指定 -out")
		os.Exit(2)
	}
	langs := benchgen.Languages
	if *legacy {
		langs = benchgen.LegacyLanguages
	}
	if err := benchgen.Generate(*out, *files, *seed, langs); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("已生成 %d 个文件到 %s\n", *files, *out)
}

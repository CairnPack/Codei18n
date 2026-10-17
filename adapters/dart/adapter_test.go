package dart

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/CairnPack/codei18n/adapters/dart/grammar"
	"github.com/CairnPack/codei18n/core/domain"
	"github.com/CairnPack/codei18n/core/utils"
)

// sample 是一段覆盖 Dart 3 常见语法的非平凡代码（sealed/base/final class、record、
// pattern、switch 表达式、extension type、async*、级联、空安全等）。
const sample = `library app.models;

import 'dart:async';
import 'dart:math' as math show max;

part 'models.g.dart';

/// 形状的基类。
sealed class Shape {
  const Shape();
  double get area;
}

final class Circle extends Shape {
  final double radius;
  const Circle(this.radius);
  @override
  double get area => math.pi * radius * radius;
}

base class Rect extends Shape {
  final double w, h;
  Rect(this.w, this.h) : assert(w >= 0);
  Rect.square(double s) : this(s, s);
  @override
  double get area => w * h;
}

String describe(Shape s) => switch (s) {
      Circle(radius: var r) when r > 10 => 'big circle $r',
      Circle c => 'circle ${c.radius}',
      Rect(:final w, :final h) => 'rect ${w}x$h',
    };

(int, String) pair() => (1, 'one');

extension type const Meters(double value) implements Object {
  Meters operator +(Meters o) => Meters(value + o.value);
}

mixin Logger on Object {
  final List<String> _log = <String>[];
  void log(String msg) => _log..add(msg)..add('\n');
}

extension NumX<T extends num> on List<T> {
  T? get firstOrNull => isEmpty ? null : first;
}

enum Status implements Comparable<Status> {
  active('A'),
  inactive('I');

  const Status(this.code);
  final String code;

  @override
  int compareTo(Status other) => code.compareTo(other.code);
}

typedef Mapper<T> = T Function(T value);

Stream<int> counter(int n) async* {
  for (var i = 0; i < n; i++) {
    await Future<void>.delayed(const Duration(milliseconds: 1));
    yield i;
  }
}

Future<void> main(List<String> args) async {
  final shapes = <Shape>[const Circle(1), Rect(2, 3)];
  var (a, b) = pair();
  final map = {for (final s in shapes) s: s.area};
  late final String name;
  name = args.isNotEmpty ? args.first : 'world';
  if (map case {Circle(): final double x}) {
    print('$a $b $x');
  }
  try {
    await for (final v in counter(3)) {
      print(v);
    }
  } on StateError catch (e, st) {
    print('$e $st');
  } finally {
    print(name?.length ?? 0);
  }
}
`

func parse(t *testing.T, src string) []*domain.Comment {
	t.Helper()
	cs, err := NewAdapter().Parse("lib/a.dart", []byte(src))
	require.NoError(t, err)
	return cs
}

func texts(cs []*domain.Comment) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.SourceText
	}
	return out
}

func TestGrammarParsesWithoutErrors(t *testing.T) {
	p := sitter.NewParser()
	p.SetLanguage(grammar.GetLanguage())
	tree, err := p.ParseCtx(context.Background(), nil, []byte(sample))
	require.NoError(t, err)
	defer tree.Close()
	root := tree.RootNode()
	assert.Equal(t, "program", root.Type())
	if root.HasError() {
		t.Fatalf("语法树含 ERROR 节点: %s", root.String())
	}
	assert.Greater(t, int(root.NamedChildCount()), 10)
}

func TestLanguage(t *testing.T) {
	assert.Equal(t, "dart", NewAdapter().Language())
}

func TestCommentTypes(t *testing.T) {
	src := "// 行注释\nvoid a() {}\n\n/* 块注释 */\nvoid b() {}\n\n/** 文档块 */\nvoid c() {}\n\n/// 文档行\nvoid d() {}\n"
	cs := parse(t, src)
	require.Len(t, cs, 4)
	want := []domain.CommentType{domain.CommentTypeLine, domain.CommentTypeBlock, domain.CommentTypeDoc, domain.CommentTypeDoc}
	for i, c := range cs {
		assert.Equal(t, want[i], c.Type, c.SourceText)
		assert.Equal(t, "dart", c.Language)
		assert.Equal(t, "lib/a.dart", c.File)
		assert.Empty(t, c.ID, "adapter 不设置 ID")
		assert.Equal(t, 1, c.Parts)
	}
	assert.Equal(t, []string{"a", "b", "c", "d"}, []string{cs[0].Symbol, cs[1].Symbol, cs[2].Symbol, cs[3].Symbol})
}

// 语法把 "////" 与 "/**/" 报告为 documentation_comment，类型必须按原文判定。
func TestGrammarDocQuirksUseTextRules(t *testing.T) {
	cs := parse(t, "//// 分隔线\nvoid a() {}\n/**/ int x = 1;\n")
	require.Len(t, cs, 2)
	assert.Equal(t, domain.CommentTypeLine, cs[0].Type)
	assert.Equal(t, domain.CommentTypeBlock, cs[1].Type)
}

func TestDocLineGroupMerged(t *testing.T) {
	src := "class A {\n  /// 第一行\n  ///\n  /// 第三行\n  void m() {}\n}\n"
	cs := parse(t, src)
	require.Len(t, cs, 1)
	c := cs[0]
	assert.Equal(t, domain.CommentTypeDoc, c.Type)
	assert.Equal(t, 3, c.Parts)
	assert.Equal(t, "/// 第一行\n///\n/// 第三行", c.SourceText)
	assert.Equal(t, []string{"/// 第一行", "///", "/// 第三行"}, c.PartTexts)
	assert.Equal(t, "A#m", c.Symbol)
	// 列为 1 起始字节偏移，EndCol 左闭右开
	assert.Equal(t, domain.TextRange{StartLine: 2, StartCol: 3, EndLine: 4, EndCol: 3 + len("/// 第三行")}, c.Range)
}

func TestDocBlock(t *testing.T) {
	src := "/**\n * 返回两数之和。\n * [a] 第一个数\n */\nint add(int a, int b) => a + b;\n"
	cs := parse(t, src)
	require.Len(t, cs, 1)
	assert.Equal(t, domain.CommentTypeDoc, cs[0].Type)
	assert.Equal(t, 1, cs[0].Parts)
	assert.Equal(t, "add", cs[0].Symbol)
	assert.Equal(t, domain.TextRange{StartLine: 1, StartCol: 1, EndLine: 4, EndCol: 4}, cs[0].Range)
}

func TestNestedBlockComment(t *testing.T) {
	cs := parse(t, "/* 外 /* 内 */ 外 */\nvoid f() {}\n")
	require.Len(t, cs, 1)
	assert.Equal(t, "/* 外 /* 内 */ 外 */", cs[0].SourceText)
	assert.Equal(t, domain.CommentTypeBlock, cs[0].Type)
}

func TestConsecutiveLineCommentsMerged(t *testing.T) {
	src := "void f() {\n  // 第一行\n  // 第二行\n  var x = 1;\n}\n"
	cs := parse(t, src)
	require.Len(t, cs, 1)
	assert.Equal(t, 2, cs[0].Parts)
	assert.Equal(t, domain.CommentTypeLine, cs[0].Type)
	assert.Equal(t, "// 第一行\n// 第二行", cs[0].SourceText)
	assert.Equal(t, "f", cs[0].Symbol)
}

func TestTrailingCommentsNotMerged(t *testing.T) {
	src := "void f() {\n  var a = 1; // 尾注释一\n  var b = 2; // 尾注释二\n  // 独占一行\n  // 第二行\n}\n"
	cs := parse(t, src)
	require.Len(t, cs, 3)
	assert.Equal(t, []string{"// 尾注释一", "// 尾注释二", "// 独占一行\n// 第二行"}, texts(cs))
	assert.Equal(t, []int{1, 1, 2}, []int{cs[0].Parts, cs[1].Parts, cs[2].Parts})
	// 中文按 UTF-8 字节计列
	assert.Equal(t, domain.TextRange{StartLine: 2, StartCol: 14, EndLine: 2, EndCol: 14 + len("// 尾注释一")}, cs[0].Range)
}

func TestCommentLikeTextInStringsIgnored(t *testing.T) {
	src := `void f(int x) {
  var a = '// 单引号 /* 不是注释 */';
  var b = "// 双引号 /// 也不是";
  var c = r'// raw /* raw */';
  var d = r"/** raw doc */";
  var e = '''
  // 三引号里的行
  /* 三引号里的块 */
  ''';
  var g = """
  /// 三双引号
  """;
  var h = 'x $x // 插值后的文本';
  var i = "\${'// 插值内的字符串'} /* 外面 */ ${x}";
  var k = "${'// 插值内的字符串'} ${"/* 嵌套字符串 */"}";
  var j = r'''
  // 原始三引号
  ''';
  // 真正的注释
}
`
	cs := parse(t, src)
	assert.Equal(t, []string{"// 真正的注释"}, texts(cs))
}

func TestCommentInsideInterpolationIsRealComment(t *testing.T) {
	// ${ } 内部是 Dart 代码，其中的注释是真正的注释
	cs := parse(t, "void f(int x) {\n  var s = '${x /* 真注释 */} // 字符串';\n}\n")
	assert.Equal(t, []string{"/* 真注释 */"}, texts(cs))
	assert.Equal(t, "f", cs[0].Symbol)
}

func TestSymbols(t *testing.T) {
	// 每条注释的文本就是期望的符号
	src := `library my.app;

import 'dart:async';

// my.app.Foo
abstract class Foo<T> extends Object implements Comparable<Foo<T>> {
  // my.app.Foo#count
  @Deprecated('x')
  static const int count = 1;
  // my.app.Foo#name
  final String name;
  // my.app.Foo#a
  late int a, b;
  // my.app.Foo#Foo
  Foo(this.name);
  // my.app.Foo#Foo.named
  Foo.named() : name = 'n';
  // my.app.Foo#Foo.create
  factory Foo.create() => _Impl();
  // my.app.Foo#run
  @override
  Future<void> run() async {
    // my.app.Foo#run
    await null;
  }
  // my.app.Foo#size
  int get size => 0;
  // my.app.Foo#size
  set size(int v) {}
  // my.app.Foo#operator+
  Foo<T> operator +(Foo<T> o) => this;
  // my.app.Foo#abstractMethod
  void abstractMethod();
  // my.app.Foo
}

// my.app._Impl
class _Impl<T> extends Foo<T> {
  _Impl() : super('i');
}

// my.app.Logging
mixin Logging on Object {
  // my.app.Logging#log
  void log(String m) {}
}

// my.app.StringX
extension StringX on String {
  // my.app.StringX#shout
  String shout() => toUpperCase();
  // my.app.StringX#isBlank
  bool get isBlank => trim().isEmpty;
}

// my.app.Id
extension type Id(int raw) {
  // my.app.Id#next
  Id next() => Id(raw + 1);
}

// my.app.Color
enum Color {
  // my.app.Color#red
  red,
  // my.app.Color#green
  green;

  // my.app.Color#hex
  String hex() => '';
}

// my.app.Callback
typedef Callback = void Function(int);
// my.app.Legacy
typedef int Legacy(int x);

// my.app.topLevel
int topLevel(int x) {
  // my.app.topLevel
  int inner() => 1;
  return inner() + x; // my.app.topLevel
}

// my.app.version
const version = '1.0';
// my.app.counter
int counter = 0;
// my.app.lazy
late final String lazy;
// my.app.current
String get current => '';

// my.app.main
void main() {}
`
	cs := parse(t, src)
	require.NotEmpty(t, cs)
	for _, c := range cs {
		want := strings.TrimPrefix(c.SourceText, "// ")
		assert.Equal(t, want, c.Symbol, "第 %d 行", c.Range.StartLine)
	}
	assert.Len(t, cs, strings.Count(src, "// my.app"))
}

func TestSymbolsWithoutLibraryAndUnnamedExtension(t *testing.T) {
	src := "// A\nclass A {\n  // A#m\n  void m() {}\n}\n\nextension on int {\n  // twice\n  int twice() => this * 2;\n}\n\n// 文件末尾\n"
	cs := parse(t, src)
	require.Len(t, cs, 4)
	assert.Equal(t, []string{"A", "A#m", "twice", ""}, []string{cs[0].Symbol, cs[1].Symbol, cs[2].Symbol, cs[3].Symbol})
}

func TestCRLF(t *testing.T) {
	src := "void a() {}\r\n// 第一行\r\n// 第二行\r\nvoid b() {}\r\n"
	cs := parse(t, src)
	require.Len(t, cs, 1)
	assert.Equal(t, "// 第一行\n// 第二行", cs[0].SourceText)
	assert.Equal(t, domain.TextRange{StartLine: 2, StartCol: 1, EndLine: 3, EndCol: 1 + len("// 第二行")}, cs[0].Range)
	assert.Equal(t, "b", cs[0].Symbol)
}

func TestIDAndSymbolStableWhenInsertingLinesAbove(t *testing.T) {
	src := "library x;\n\nclass A {\n  /// 方法文档\n  void m() {}\n  // 字段\n  int f = 0;\n}\n"
	shifted := strings.Replace(src, "class A {\n", "// 新增的说明\nvoid other() {}\n\nclass A {\n\n", 1)
	before := parse(t, src)
	after := parse(t, shifted)
	require.Len(t, before, 2)
	require.Len(t, after, 3)
	after = after[1:]
	for i := range before {
		assert.Equal(t, before[i].Symbol, after[i].Symbol)
		assert.Equal(t, utils.GenerateCommentID(before[i]), utils.GenerateCommentID(after[i]))
		assert.NotEqual(t, before[i].Range.StartLine, after[i].Range.StartLine)
	}
	assert.Equal(t, "x.A#m", before[0].Symbol)
	assert.Equal(t, "x.A#f", before[1].Symbol)
}

func TestParseReadsFileWhenSrcNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.dart")
	require.NoError(t, os.WriteFile(path, []byte("// 你好\nvoid main() {}\n"), 0o644))
	cs, err := NewAdapter().Parse(path, nil)
	require.NoError(t, err)
	require.Len(t, cs, 1)
	assert.Equal(t, "main", cs[0].Symbol)

	_, err = NewAdapter().Parse(filepath.Join(dir, "missing.dart"), nil)
	assert.Error(t, err)
}

func TestSyntaxErrorsStillYieldComments(t *testing.T) {
	cs := parse(t, "class A {\n  // 仍能提取\n  void m( {\n}\n")
	assert.Contains(t, texts(cs), "// 仍能提取")
}

func TestConcurrentAdapters(t *testing.T) {
	want := parse(t, sample+"\n// 结尾\n/// 文档\nvoid z() {}\n")
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			a := NewAdapter()
			shared := sharedAdapter
			for i := 0; i < 8; i++ {
				for _, ad := range []*Adapter{a, shared} {
					cs, err := ad.Parse(fmt.Sprintf("f%d.dart", g), []byte(sample+"\n// 结尾\n/// 文档\nvoid z() {}\n"))
					if err != nil {
						errs <- err
						return
					}
					if len(cs) != len(want) || cs[len(cs)-1].Symbol != "app.models.z" {
						errs <- fmt.Errorf("goroutine %d: 结果不一致: %d 条", g, len(cs))
						return
					}
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// sharedAdapter 用于验证同一实例被并发调用时也安全（内部有互斥锁）。
var sharedAdapter = NewAdapter()

package csharp

import (
	"strings"
	"unicode"

	sitter "github.com/smacker/go-tree-sitter"
)

// 符号格式：Namespace.Outer.Inner#Member。
// 符号只由声明名称组成，不含任何位置，因此在上方插入空行或代码不会改变符号（也就不会改变 ID）。

// declKind 表示节点在符号路径中的角色。
type declKind int

const (
	kindNone      declKind = iota // 非声明节点（在祖先遍历中透明）
	kindNamespace                 // 命名空间（含文件级 namespace X;）
	kindType                      // 类型：class / struct / interface / record / enum / delegate
	kindMember                    // 成员：方法、构造器、析构器、属性、索引器、运算符、字段、事件、枚举成员
)

// classify 判断节点是否为声明，并返回其名称。
func classify(n *sitter.Node, src []byte) (declKind, string) {
	switch n.Type() {
	case "namespace_declaration", "file_scoped_namespace_declaration":
		return kindNamespace, compact(field(n, "name", src))
	case "class_declaration", "struct_declaration", "interface_declaration", "record_declaration",
		"record_struct_declaration", "enum_declaration", "delegate_declaration":
		return kindType, field(n, "name", src)
	case "method_declaration", "constructor_declaration", "property_declaration",
		"event_declaration", "enum_member_declaration":
		return kindMember, field(n, "name", src)
	case "destructor_declaration":
		return kindMember, "~" + field(n, "name", src)
	case "indexer_declaration":
		return kindMember, "this"
	case "operator_declaration":
		return kindMember, "operator" + compact(field(n, "operator", src))
	case "conversion_operator_declaration":
		return kindMember, "operator " + compact(field(n, "type", src))
	case "field_declaration", "event_field_declaration":
		return kindMember, variableName(n, src)
	}
	return kindNone, ""
}

// skippable 判断查找"下一个声明"时是否越过该兄弟节点：其他注释、特性、修饰符、区域等预处理指令。
func skippable(n *sitter.Node) bool {
	switch n.Type() {
	case "comment", "attribute_list", "modifier",
		"preproc_region", "preproc_endregion", "preproc_pragma", "preproc_nullable",
		"preproc_define", "preproc_undef", "preproc_line", "preproc_error", "preproc_warning":
		return true
	case "ERROR":
		return errorDecl(n) == nil
	}
	return false
}

// errorDecl 返回 ERROR 节点中的第一个声明子节点（若有）。
func errorDecl(n *sitter.Node) *sitter.Node {
	if n.Type() != "ERROR" {
		return nil
	}
	for i := 0; i < int(n.NamedChildCount()); i++ {
		c := n.NamedChild(i)
		if k, _ := classify(c, nil); k != kindNone {
			return c
		}
	}
	return nil
}

// resolveSymbol 解析物理注释的符号：
//  1. 越过注释、特性、修饰符后的下一个命名兄弟若是声明，取该声明的符号；
//  2. 否则取包含该注释的最近声明的符号；
//  3. 都没有时为文件级命名空间（若注释位于 namespace X; 之后），否则为 ""。
func resolveSymbol(comment *sitter.Node, src []byte, fileNS *sitter.Node) string {
	if next := nextDeclaration(comment, src); next != nil {
		return symbolOf(next, src, fileNS)
	}
	for p := comment.Parent(); p != nil; p = p.Parent() {
		if k, _ := classify(p, src); k != kindNone {
			return symbolOf(p, src, fileNS)
		}
	}
	return symbolOf(comment, src, fileNS)
}

// nextDeclaration 查找注释之后紧邻的声明节点。
// 若注释及其后的可越过节点是父节点的末尾部分（被语法吸收进前一个节点），上升到父节点继续查找。
func nextDeclaration(comment *sitter.Node, src []byte) *sitter.Node {
	cur := comment
	for cur != nil {
		last := cur
		next := cur.NextNamedSibling()
		for next != nil && skippable(next) {
			last = next
			next = next.NextNamedSibling()
		}
		if next != nil {
			if d := errorDecl(next); d != nil {
				return d
			}
			if k, _ := classify(next, src); k != kindNone {
				return next
			}
			return nil
		}
		parent := cur.Parent()
		if parent == nil || parent.Parent() == nil || parent.EndByte() != last.EndByte() {
			return nil
		}
		if k, _ := classify(parent, src); k != kindNone {
			// 被吸收进声明末尾的注释仍归属该声明
			return nil
		}
		cur = parent
	}
	return nil
}

// symbolOf 由节点及其祖先构建符号：命名空间与类型名以 "." 连接；
// 自外向内遇到的第一个成员以 "#" 接在类型路径之后，更内层的声明（局部函数等）忽略。
// 该语法把文件级命名空间 namespace X; 解析为与类型并列的兄弟节点，
// 因此位于它之后、又不在它子树中的节点需要补上它的名称。
func symbolOf(n *sitter.Node, src []byte, fileNS *sitter.Node) string {
	var chain []*sitter.Node
	insideFileNS := false
	for cur := n; cur != nil; cur = cur.Parent() {
		chain = append(chain, cur)
		if fileNS != nil && cur.Equal(fileNS) {
			insideFileNS = true
		}
	}
	var parts []string
	if fileNS != nil && !insideFileNS && n.StartByte() >= fileNS.StartByte() {
		if _, name := classify(fileNS, src); name != "" {
			parts = append(parts, name)
		}
	}
	member := ""
	for i := len(chain) - 1; i >= 0; i-- {
		k, name := classify(chain[i], src)
		if name == "" {
			continue
		}
		if k == kindNamespace || k == kindType {
			parts = append(parts, name)
		} else if k == kindMember {
			member = name
			break
		}
	}
	path := strings.Join(parts, ".")
	if member == "" {
		return path
	}
	if path == "" {
		return member
	}
	return path + "#" + member
}

// variableName 返回字段 / 事件字段声明中第一个变量的名称（int a, b; 取 "a"）。
func variableName(n *sitter.Node, src []byte) string {
	for i := 0; i < int(n.NamedChildCount()); i++ {
		c := n.NamedChild(i)
		if c.Type() != "variable_declaration" {
			continue
		}
		for j := 0; j < int(c.NamedChildCount()); j++ {
			if d := c.NamedChild(j); d.Type() == "variable_declarator" {
				if name := field(d, "name", src); name != "" {
					return name
				}
				// 兼容没有 name 字段的语法版本：取第一个 identifier 子节点
				for k := 0; k < int(d.NamedChildCount()); k++ {
					if id := d.NamedChild(k); id.Type() == "identifier" {
						if src == nil {
							return "identifier"
						}
						return id.Content(src)
					}
				}
			}
		}
	}
	return ""
}

// field 返回指定字段子节点的文本；src 为 nil 时只判断存在性（返回字段名）。
func field(n *sitter.Node, name string, src []byte) string {
	c := n.ChildByFieldName(name)
	if c == nil {
		return ""
	}
	if src == nil {
		return name
	}
	return c.Content(src)
}

// compact 去掉名称中的所有空白，例如 "Foo . Bar" → "Foo.Bar"。
func compact(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

// fileScopedNamespace 返回文件级命名空间声明 namespace X;（C# 规定一个文件至多一个）。
func fileScopedNamespace(root *sitter.Node) *sitter.Node {
	for i := 0; i < int(root.NamedChildCount()); i++ {
		if c := root.NamedChild(i); c.Type() == "file_scoped_namespace_declaration" {
			return c
		}
	}
	return nil
}

package kotlin

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// 符号格式与 Java adapter 一致：package.Outer.Inner#member。
// 符号只由声明名称组成，不含任何位置，因此在上方插入空行或代码不会改变符号（也就不会改变 ID）。

// declKind 表示节点在符号路径中的角色。
type declKind int

const (
	kindNone   declKind = iota // 非声明节点（在祖先遍历中透明）
	kindType                   // 类型：class / interface / enum class / object / companion object / typealias
	kindMember                 // 成员：函数、属性、次构造器、主构造器参数、枚举项
)

// classify 判断节点是否为声明，并返回其名称。
func classify(n *sitter.Node, src []byte) (declKind, string) {
	switch n.Type() {
	case "class_declaration", "object_declaration", "type_alias":
		return kindType, childContent(n, "type_identifier", src)
	case "companion_object":
		if name := childContent(n, "type_identifier", src); name != "" {
			return kindType, name
		}
		return kindType, "Companion"
	case "function_declaration":
		return kindMember, childContent(n, "simple_identifier", src)
	case "property_declaration":
		return kindMember, propertyName(n, src)
	case "secondary_constructor":
		return kindMember, "constructor"
	case "class_parameter", "enum_entry":
		return kindMember, childContent(n, "simple_identifier", src)
	}
	return kindNone, ""
}

// skippable 判断查找"下一个声明"时是否越过该兄弟节点：其他注释、注解、修饰符。
// 顶层 "@Ann(...)\nclass X" 会被该语法版本解析成以 annotation 开头的 prefix_expression，也一并越过。
func skippable(n *sitter.Node) bool {
	switch n.Type() {
	case "line_comment", "multiline_comment", "annotation", "file_annotation", "modifiers", "shebang_line":
		return true
	case "prefix_expression":
		first := n.NamedChild(0)
		return first != nil && first.Type() == "annotation"
	case "ERROR":
		// 语法错误节点中不含声明时越过
		return errorDecl(n) == nil
	}
	return false
}

// errorDecl 返回 ERROR 节点中的第一个声明子节点（若有）。
// 该语法版本不支持无函数体的接口方法（interface Api { fun f() }），会把它包进 ERROR 节点。
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
//  1. 越过注释、注解、修饰符后的下一个命名兄弟若是声明，取该声明的符号；
//  2. 否则取包含该注释的最近声明的符号；
//  3. 都没有时为 ""（文件级）。
func resolveSymbol(comment *sitter.Node, src []byte, pkg string) string {
	if next := nextDeclaration(comment, src); next != nil {
		return symbolOf(next, src, pkg)
	}
	for p := comment.Parent(); p != nil; p = p.Parent() {
		if k, _ := classify(p, src); k != kindNone {
			return symbolOf(p, src, pkg)
		}
	}
	return ""
}

// nextDeclaration 查找注释之后紧邻的声明节点。
// 该语法会把位于末尾的注释吸收进前一个节点（例如 import 列表之后的注释挂在 import_header 下），
// 此时注释是父节点的最后一部分，需要上升到父节点继续找它的下一个兄弟。
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

// symbolOf 由声明节点及其祖先构建符号：类型名以 "." 连接；
// 自外向内遇到的第一个成员以 "#" 接在类型路径之后，更内层的声明（局部函数、局部类）忽略，
// 即局部声明归属于外层成员。
func symbolOf(n *sitter.Node, src []byte, pkg string) string {
	var chain []*sitter.Node
	for cur := n; cur != nil; cur = cur.Parent() {
		chain = append(chain, cur)
	}
	var types []string
	member := ""
	for i := len(chain) - 1; i >= 0; i-- {
		k, name := classify(chain[i], src)
		if k == kindType && name != "" {
			types = append(types, name)
		} else if k == kindMember && name != "" {
			member = name
			break
		}
	}
	path := strings.Join(types, ".")
	if pkg != "" {
		if path != "" {
			path = pkg + "." + path
		} else {
			path = pkg
		}
	}
	if member == "" {
		return path
	}
	if path == "" {
		return member
	}
	return path + "#" + member
}

// propertyName 返回属性名；解构声明 val (a, b) 返回 "a,b"。
func propertyName(n *sitter.Node, src []byte) string {
	for i := 0; i < int(n.NamedChildCount()); i++ {
		c := n.NamedChild(i)
		switch c.Type() {
		case "variable_declaration":
			return childContent(c, "simple_identifier", src)
		case "multi_variable_declaration":
			var names []string
			for j := 0; j < int(c.NamedChildCount()); j++ {
				if v := c.NamedChild(j); v.Type() == "variable_declaration" {
					names = append(names, childContent(v, "simple_identifier", src))
				}
			}
			return strings.Join(names, ",")
		}
	}
	return ""
}

// childContent 返回第一个指定类型的直接命名子节点的文本；src 为 nil 时只判断存在性（返回类型名）。
func childContent(n *sitter.Node, typ string, src []byte) string {
	for i := 0; i < int(n.NamedChildCount()); i++ {
		c := n.NamedChild(i)
		if c.Type() == typ {
			if src == nil {
				return typ
			}
			return c.Content(src)
		}
	}
	return ""
}

// packageName 从 package_header 中提取包名（各段以 "." 连接，忽略空白与注释）。
func packageName(root *sitter.Node, src []byte) string {
	for i := 0; i < int(root.NamedChildCount()); i++ {
		c := root.NamedChild(i)
		if c.Type() != "package_header" {
			continue
		}
		for j := 0; j < int(c.NamedChildCount()); j++ {
			id := c.NamedChild(j)
			if id.Type() != "identifier" {
				continue
			}
			var parts []string
			for k := 0; k < int(id.NamedChildCount()); k++ {
				if s := id.NamedChild(k); s.Type() == "simple_identifier" {
					parts = append(parts, s.Content(src))
				}
			}
			return strings.Join(parts, ".")
		}
	}
	return ""
}

package dart

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// 符号路径格式（与 adapters/java/symbol.go 风格一致）：
//   - 类型：         [library.]Class
//   - 类型成员：     [library.]Class#member（构造函数为 Class#Class 或 Class#Class.named）
//   - 顶层函数/变量：[library.]name
//
// library 前缀取自文件开头的 `library a.b;` 指令；没有该指令（或为匿名 `library;`）时省略。
//
// 注意 tree-sitter-dart 的语法把函数签名与函数体、修饰符/类型与变量列表都作为**兄弟节点**
// 平铺在 program / class_body 中（而不是包在一个声明节点里），所以这里按兄弟序列查找声明。

// typeDecls 是可作为符号容器的类型声明节点。
var typeDecls = map[string]bool{
	"class_definition":           true,
	"mixin_declaration":          true,
	"extension_declaration":      true,
	"extension_type_declaration": true,
	"enum_declaration":           true,
}

// memberDecls 是能直接给出成员（或顶层函数/变量）名字的节点。
var memberDecls = map[string]bool{
	"function_signature":                        true,
	"getter_signature":                          true,
	"setter_signature":                          true,
	"operator_signature":                        true,
	"constructor_signature":                     true,
	"constant_constructor_signature":            true,
	"factory_constructor_signature":             true,
	"redirecting_factory_constructor_signature": true,
	"initialized_identifier_list":               true,
	"static_final_declaration_list":             true,
	"enum_constant":                             true,
	// 以下两个是包装节点，名字取自其子节点
	"declaration":      true,
	"method_signature": true,
}

// isDeclaration 判断节点是否是可以作为注释符号的声明。
func isDeclaration(n *sitter.Node) bool {
	t := n.Type()
	return typeDecls[t] || memberDecls[t] || t == "type_alias"
}

// skippable 是寻找“下一个声明”时跳过的节点：其他注释与注解（metadata）。
func skippable(n *sitter.Node) bool {
	switch n.Type() {
	case "comment", "documentation_comment", "annotation":
		return true
	}
	return false
}

// isTypeOrModifier 判断节点是否是声明前部的修饰符或类型
// （顶层变量声明中它们与变量列表平铺为兄弟节点，例如 final_builtin、type_identifier）。
func isTypeOrModifier(n *sitter.Node) bool {
	switch t := n.Type(); t {
	case "final_builtin", "const_builtin", "inferred_type":
		return true
	default:
		return strings.Contains(t, "type")
	}
}

// resolveSymbol 解析注释的符号：优先取其后紧邻的声明，否则取外层声明，都没有时为空。
func resolveSymbol(comment *sitter.Node, src []byte, lib string) string {
	if d := nextDeclaration(comment); d != nil {
		return symbolPath(d, src, lib)
	}
	if d := enclosingDeclaration(comment); d != nil {
		return symbolPath(d, src, lib)
	}
	return ""
}

// nextDeclaration 沿兄弟节点向后查找注释所修饰的声明。跳过注释、注解以及声明前部的修饰符/类型；
// 遇到其他命名节点或 ';'、'{'、'}' 时停止（说明注释后面不是声明）。
func nextDeclaration(n *sitter.Node) *sitter.Node {
	for s := n.NextSibling(); s != nil; s = s.NextSibling() {
		if !s.IsNamed() {
			switch s.Type() {
			case ";", "{", "}":
				return nil
			}
			continue // late、external、static 等匿名关键字
		}
		switch {
		case skippable(s):
			continue
		case isDeclaration(s):
			if isLocal(s) {
				return nil
			}
			return s
		case isTypeOrModifier(s):
			continue
		default:
			return nil
		}
	}
	return nil
}

// enclosingDeclaration 向上查找包含注释的声明。函数体（function_body）与其签名是兄弟节点，
// 因此遇到 function_body 时取它前面的签名。局部函数与闭包不作为符号。
func enclosingDeclaration(n *sitter.Node) *sitter.Node {
	for p := n.Parent(); p != nil; p = p.Parent() {
		if p.Type() == "function_body" {
			if sig := prevSignature(p); sig != nil && !isLocal(sig) {
				return sig
			}
			continue
		}
		if isDeclaration(p) && !isLocal(p) {
			return p
		}
	}
	return nil
}

// prevSignature 返回 function_body 前面的签名节点（跳过注释与注解）。
func prevSignature(body *sitter.Node) *sitter.Node {
	for s := body.PrevNamedSibling(); s != nil; s = s.PrevNamedSibling() {
		if skippable(s) {
			continue
		}
		if memberDecls[s.Type()] {
			return s
		}
		return nil
	}
	return nil
}

// isLocal 判断声明是否位于函数体内（局部函数、闭包等），这类声明不作为符号。
func isLocal(n *sitter.Node) bool {
	for p := n.Parent(); p != nil; p = p.Parent() {
		switch p.Type() {
		case "lambda_expression", "local_function_declaration", "function_expression", "block", "function_body":
			return true
		case "program", "class_body", "extension_body", "enum_body":
			return false
		}
	}
	return false
}

// symbolPath 构建声明的符号路径。
func symbolPath(decl *sitter.Node, src []byte, lib string) string {
	var segs []string
	if lib != "" {
		segs = append(segs, lib)
	}
	if typeDecls[decl.Type()] || decl.Type() == "type_alias" {
		if name := typeName(decl, src); name != "" {
			segs = append(segs, name)
		}
		return strings.Join(segs, ".")
	}

	member := memberName(decl, src)
	container := ""
	for p := decl.Parent(); p != nil; p = p.Parent() {
		if typeDecls[p.Type()] {
			container = typeName(p, src)
			break
		}
	}
	switch {
	case container != "" && member != "":
		segs = append(segs, container+"#"+member)
	case container != "":
		segs = append(segs, container)
	case member != "":
		segs = append(segs, member)
	}
	return strings.Join(segs, ".")
}

// typeName 返回类型声明的名字。匿名 extension（`extension on T`）返回空字符串。
func typeName(n *sitter.Node, src []byte) string {
	if name := n.ChildByFieldName("name"); name != nil {
		return name.Content(src)
	}
	switch n.Type() {
	case "class_definition":
		// class A = B with M; 的名字在 mixin_application_class 中
		if app := firstNamedChildOfType(n, "mixin_application_class"); app != nil {
			if id := firstNamedChildOfType(app, "identifier"); id != nil {
				return id.Content(src)
			}
		}
	case "mixin_declaration":
		if id := firstNamedChildOfType(n, "identifier"); id != nil {
			return id.Content(src)
		}
	case "type_alias":
		return typeAliasName(n, src)
	}
	return ""
}

// typeAliasName 处理两种 typedef 形式：
//   - typedef Name<T> = Type;         名字是第一个 type_identifier；
//   - typedef RetType Name(params);  名字是参数列表前的最后一个 type_identifier。
func typeAliasName(n *sitter.Node, src []byte) string {
	hasEq := false
	for i := 0; i < int(n.ChildCount()); i++ {
		if c := n.Child(i); !c.IsNamed() && c.Type() == "=" {
			hasEq = true
			break
		}
	}
	name := ""
	for i := 0; i < int(n.NamedChildCount()); i++ {
		c := n.NamedChild(i)
		if c.Type() == "formal_parameter_list" {
			break
		}
		if c.Type() == "type_identifier" {
			name = c.Content(src)
			if hasEq {
				break
			}
		}
	}
	return name
}

// memberName 返回成员（或顶层函数/变量）的名字。
func memberName(n *sitter.Node, src []byte) string {
	switch n.Type() {
	case "function_signature", "getter_signature", "setter_signature", "enum_constant":
		if name := n.ChildByFieldName("name"); name != nil {
			return name.Content(src)
		}
	case "constructor_signature", "constant_constructor_signature",
		"factory_constructor_signature", "redirecting_factory_constructor_signature":
		// 名字是参数列表前以 '.' 连接的若干 identifier，例如 Foo、Foo.named
		var ids []string
		for i := 0; i < int(n.NamedChildCount()); i++ {
			c := n.NamedChild(i)
			if c.Type() == "formal_parameter_list" {
				break
			}
			if c.Type() == "identifier" {
				ids = append(ids, c.Content(src))
			}
		}
		return strings.Join(ids, ".")
	case "operator_signature":
		for i := 0; i+1 < int(n.ChildCount()); i++ {
			if c := n.Child(i); !c.IsNamed() && c.Type() == "operator" {
				return "operator" + n.Child(i+1).Content(src)
			}
		}
	case "initialized_identifier_list", "static_final_declaration_list":
		// 多个变量（int a, b;）取第一个
		if first := n.NamedChild(0); first != nil {
			if id := firstNamedChildOfType(first, "identifier"); id != nil {
				return id.Content(src)
			}
		}
	case "declaration", "method_signature":
		for i := 0; i < int(n.NamedChildCount()); i++ {
			c := n.NamedChild(i)
			if memberDecls[c.Type()] {
				if name := memberName(c, src); name != "" {
					return name
				}
			}
		}
	}
	return ""
}

// libraryName 返回 `library a.b;` 指令中的库名，没有时返回空字符串。
func libraryName(root *sitter.Node, src []byte) string {
	lib := firstNamedChildOfType(root, "library_name")
	if lib == nil {
		return ""
	}
	if ids := firstNamedChildOfType(lib, "dotted_identifier_list"); ids != nil {
		return strings.TrimSpace(ids.Content(src))
	}
	return ""
}

func firstNamedChildOfType(n *sitter.Node, typ string) *sitter.Node {
	for i := 0; i < int(n.NamedChildCount()); i++ {
		if c := n.NamedChild(i); c.Type() == typ {
			return c
		}
	}
	return nil
}

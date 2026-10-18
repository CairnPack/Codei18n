package dart

// dartCommentQuery 匹配 Dart 的两种注释节点：
//   - comment：// 行注释、/* */ 块注释（支持嵌套）；
//   - documentation_comment：/// 行注释、/** */ 块注释。
//
// 注意语法把 "////" 与 "/**/" 也归为 documentation_comment，
// 所以注释类型一律交给 commentgroup 按原文判定，不看节点类型。
const dartCommentQuery = `
(comment) @comment
(documentation_comment) @comment
`

# 变更：API 令牌存入系统凭据库（A7）

## 为什么

桌面端保存的令牌，CLI 与 git hook（`codei18n hook` 在提交时调用翻译）也必须能直接使用；同时令牌不得出现在配置文件、日志、错误信息、进度事件或测试快照中。旧版只读 `OPENAI_API_KEY` 环境变量，配置文件里的明文 `api_key` 既不安全也从未被真正使用。

## 变更内容

- 新增 `core/secrets`：`Store` 接口、Windows 凭据管理器实现（`github.com/danieljoos/wincred`，纯 Go）、内存实现；非 Windows 平台返回 `errors.ErrUnsupported` 并提示改用环境变量。
  - generic credential，TargetName `codei18n/<account>`，UserName `<account>`，Persist 与 mygo `Secrets` 一致（keytar 兼容）；固定 service 名 `codei18n`。
  - account：全局 profile `profile/<name>`；项目覆盖 `project/<projectId>/<name>`（projectId 为 init 生成的 UUID）。
  - 单个令牌上限 2560 字节。
- `Secret` 类型：`String`/`Format`/`MarshalJSON` 只输出脱敏形式（`••••abcd`），库 API 不出现令牌明文字符串参数。
- 读取优先级：`CODEI18N_API_KEY` > `OPENAI_API_KEY` > 项目级覆盖令牌 > 所选 profile 的全局令牌（profile 的 `apiKeyRef` 可指定其他 account）> 旧版明文字段（照常使用并警告）> 返回说明设置方式的错误。
- `Project.TokenStatus()` 返回生效来源层与末 4 位，不返回明文。
- 迁移 API：`Project.MigratePlaintextTokens()`、`Global.MigratePlaintextTokens()`（先写凭据库，再原子删除文件中的字段）。
- `Global`：profile 新建 / 复制（不复制令牌）/ 重命名（迁移全局与项目令牌）/ 删除（一并删除令牌）、保存 / 删除 / 查询令牌。
- CLI：`codei18n auth set|status|delete [--profile <name>] [--project]`、`codei18n auth migrate`；`set` 只从标准输入或隐藏输入读取。
- 防泄漏：翻译后端错误、失败记录、进度事件、日志统一脱敏（Authorization 头、`sk-` 形式密钥、"provided:" 回显、实际使用令牌的长前缀/后缀）。

## 影响

- 受影响规范：token-store（新增）
- 受影响代码：`core/secrets`（新增）、`auth.go`、`global.go`、`core/config/plaintext.go`、`core/workflow/env.go`、`cmd/codei18n/auth.go`
- 假设（brief 未覆盖，取保守方案）：
  - 旧版明文字段的优先级排在凭据库之后、报错之前。
  - 复制 profile 不复制令牌，避免在凭据库中悄悄产生副本。
  - 项目配置中的明文迁入该项目当前生效 profile 的项目级令牌；全局顶层明文迁入 `profile/default`。

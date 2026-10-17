# 任务列表：令牌存储

- [x] 1. `core/secrets`：Store 接口、Windows 凭据管理器、内存实现、非 Windows 不支持提示、2560 字节上限
- [x] 2. `Secret` 类型脱敏输出与 `Redact`
- [x] 3. 令牌解析优先级与 `TokenStatus`
- [x] 4. 旧版明文字段检测、照常使用并警告、迁移 API
- [x] 5. `Global` 配置档管理与令牌联动
- [x] 6. 翻译错误 / 失败记录 / 进度 / 日志脱敏
- [x] 7. CLI `auth set|status|delete|migrate`
- [x] 8. 测试：内存 Store 覆盖优先级、迁移、防泄漏（模拟后端回显 Authorization）；Windows 凭据管理器集成测试；core 保存 → CLI translate 与 git hook 直接可用的端到端测试

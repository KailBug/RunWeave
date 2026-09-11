# 契约样例

状态：现行。日期：2026-09-10。适用范围：P0-02 / 业务契约 v1。维护责任：当前工作项开发者。

字段解释见[业务契约](../../参考/业务请求与结果.md)，实现见[contract](../../../internal/contract/request.go)。全部数据为脱敏的虚构样例，不表示资源或产物实际存在。

`cases.json` 列出请求/结果 JSON 与期望错误码；省略 code 表示合法。`request-*` 覆盖三种操作和非法字段/操作/路径/资源引用，`result-*` 覆盖三种结果、执行失败与非法返回。测试自动读取该清单，文档不复制另一组样例。

`list-canonical.json` 是人工列出的 request-list 规范 JSON，文件结尾换行不属于摘要。`list-digest.txt` 由 .NET SHA256 对 UTF-8 的 `runweave-request-v1\n` 加该规范 JSON（去除文件尾 CR/LF）计算，独立于 Go 实现，测试将两者一起作为固定向量。

fixtures 验证命令：仓库根目录执行 `go test ./internal/contract -run TestFixtures -v`。样例文件只是数据，不会发起远程操作；完整测试与限制见[验证记录](../../验证/2026-09-10-P0业务契约验证.md)。

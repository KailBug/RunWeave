# MCP 客户端契约与真实调用证据

状态：现行。日期：2026-09-11。适用范围：P0 / MCP 工具契约 v1。维护责任：当前工作项开发者。

- [tools.json](tools.json)：六个工具的完整 inputSchema/outputSchema，由 Go `Tools()` 导出；测试核对无漂移。
- [客户端验证提示](客户端验证提示.txt)：实际传入 Codex 的 13 步调用输入。
- [codex-calls.jsonl](codex-calls.jsonl)：真实 CLI JSONL 中的 13 个 MCP 完成事件，保留参数、结果和状态，移除其他会话/模型事件。
- [server-audit.jsonl](server-audit.jsonl)：同次进程的 initialize、tools/list 和 tools/call 记录；只有 MCP 协议数据，所有业务值均为内存合成样例，无身份凭据或真实文件数据。

自动核验位于[证据测试](../../../internal/contractprobe/evidence_test.go)，检查客户端与服务端一一对应、实际发现 schema、请求摘要、输出校验、取消和错误语义。不能将重新编码的预期结果作为真实日志；当前证据摘取自 `.cache/p0-real-client-20260911` 的实际输出。

运行方法见[指南](../../指南/P0真实客户端验证.md)，版本、测试结果与限制见[验证记录](../../验证/2026-09-11-P0真实客户端契约验证.md)。这些资料只证明真实客户端与内存契约探针兼容，不证明生产 Node 可用。

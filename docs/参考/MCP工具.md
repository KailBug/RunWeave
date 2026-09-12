# MCP 工具契约 v1

状态：现行，P0 已冻结并经真实客户端验证。日期：2026-09-11。维护责任：当前工作项开发者。适用范围：客户端契约；正式业务服务尚未实现。

## 实现与边界

六个工具的字段由 [Go 类型](../../internal/mcpcontract/types.go)与 [schema 生成器](../../internal/mcpcontract/schema.go)定义，完整 [inputSchema/outputSchema](../资源/MCP契约/tools.json)随代码维护。采用官方 MCP SDK stdio、structuredContent 和同内容 JSON 文本，不依赖 MCP Tasks。决定见 [ADR 0004](../决策/0004-MCP客户端契约验证.md)，实测见[验证记录](../验证/2026-09-11-P0真实客户端契约验证.md)。

当前唯一入口是独立[内存探针](../../cmd/mcp-contract-probe/main.go)。它没有业务身份、生产授权、数据库、Server–Node 连接或真实执行能力，不能用于操作用户文件。正式服务需在 P1–P5 实现下列授权和持久化语义后另行验收。未经定义的字段拒绝；可选字段省略，不能传 null；所有对象大小和语义还需严格业务校验，schema 本身不是安全边界。

## 输入和输出

统一成功输出为 `{"data":具体结果}`，调用错误为 `{"error":{"code":"错误码","message":"说明"}}`，二者恰好一个。调用错误同时设置 MCP `isError=true`。已产生 execution 的执行失败保存在 `data.outcome.result.error`，不能等同调用错误；查询成功仍 `isError=false`。错误分类、字节限额、资源/路径和 Request 摘要见[业务契约](业务请求与结果.md)。未知工具属于 MCP 协议错误，未知或非法参数属于工具错误。

| 工具 | 输入 | data 形态与约束 |
| --- | --- | --- |
| `runweave.list_nodes` | 可选 `limit`、`cursor` | `nodes` 数组；每项 node_id、os、capabilities、online、schedulable、config_version；还有可选 next_cursor |
| `runweave.list_resources` | 可选 `node_id`、`limit`、`cursor` | `resources` 数组，每项 resource 与 locations；位置含 node_id、available、return_content 和资源 evidence；还有可选 next_cursor，不输出物理根目录 |
| `runweave.execute` | 完整 Request：request_id、requirements、operation，可选 correlation_id、timeout_ms | ExecutionView；同调用方 request_id 与同规范化摘要返回同 ID，不同摘要返回 IDEMPOTENCY_CONFLICT；正式服务必须持久提交后返回 |
| `runweave.get_execution` | `execution_id`、`request_id` 恰好一个 | ExecutionView；按当前调用方权限查询，request_id 在调用方范围内解析；缺失返回 EXECUTION_NOT_FOUND |
| `runweave.cancel_execution` | 必填 execution_id | ExecutionView；只表示持久取消意图，不能凭响应或 cancel_requested 判断已停止；继续查询证据，终态重复取消不更改结果 |
| `runweave.read_artifact` | 必填 artifact_id；可选 offset、limit | artifact_id、offset、encoding=`base64`、data、bytes_returned、eof；offset 默认为 0，limit 默认/最大 65536，最小 1；缺失/不可用/过期按相应错误返回 |

列表 limit 默认/最大 64、最小 1；cursor 是 1–256 字符的不透明字符串，返回 next_cursor 才有下一页。列表只返回授权范围内容，不能凭 node_id 或 artifact_id 绕过当前授权。所有 ID 使用业务契约的 1–128 ASCII 标识符规则。时间使用 RFC3339；非负整数上限为 9007199254740991，版本号至少 1。产物 EOF 后读取返回空 base64、0 字节、eof=true，保留请求 offset。

输入示例：`get_execution {"request_id":"my-request"}` 合法；同时给两个查询键、空对象、未知字段或 null 均非法。`read_artifact {"artifact_id":"opaque-id","offset":0,"limit":4}` 合法；limit=0、绝对文件路径作为 artifact_id 均非法。三种 execute 操作及 literal/path 联合参数沿用已有[业务样例](../资源/契约/README.md)；真实调用参数见[客户端提示](../资源/MCP契约/客户端验证提示.txt)和[原始调用事件摘录](../资源/MCP契约/codex-calls.jsonl)。

## ExecutionView

必填：execution_id、request_id、request_digest、state、cancel_requested、created_at、updated_at、poll_after_ms。可选：correlation_id、node_id、started_at、finished_at、approval、placement、outcome。

- request_digest 是 `runweave-request-v1` 摘要；Node/调用方关联和持久原子幂等由后续服务负责。
- created_at ≤ started_at（存在时）≤ finished_at（存在时）≤ updated_at；无 started_at 不表示必然没有启动，必须结合状态与证据。
- 非终态 poll_after_ms 为 100–30000，终态为 0。WAITING_APPROVAL 必须带 approval；approval 含 approval_id、expires_at、policy_version、mapping_version，MCP 不提供批准工具。
- DISPATCHING、WAITING_APPROVAL、RUNNING、UNKNOWN 占槽并包含 node_id；RUNNING 带 started_at。终态必须同时有 finished_at 与 outcome，outcome.state 匹配 state，quiescent=true；started_at 与 outcome.start 一致。UNKNOWN 不得换键重试以绕过隔离。
- placement 可选；提供时含 rule_version、selected_node_id、至多 64 个 candidates（node_id、eligible、reason_code）。候选唯一且选中 Node 为可用候选，并匹配 node_id。真实候选排序/授权尚待 P3 实现。

状态含义和 Outcome 证明要求统一见[执行状态与证据](../设计/执行状态与证据.md)。跨字段、结果错误码分类、UTF-8 字节限制、资源声明、摘要及状态证据由 Go 业务校验负责；JSON Schema 主要负责客户端结构和可表达的字段限制，不能替代这些校验。

## 探针限定行为

只提供一个虚构 Linux Node `p0-simulated-node`、资源 `repo://p0-fixture`、内存 `sample.txt` 与 `p0-artifact`。列表仅一页，非空 cursor 拒绝；不支持版本匹配。fs.list 根路径只能是 `.`，fs.read 只能是 `sample.txt`；process.exec 仅接受示例 python/cwd，返回模拟审批等待，绝不创建子进程。内容为 UTF-8 `RunWeave P0 内存样例\n`，文件/产物分块返回 base64。

execute 仅在当前探针进程的内存中保留 ID/摘要。cancel 返回等待状态和意图，下一次 get 生成模拟未启动取消确认。产物固定驻留内存；expires_at 只为结构样例，不验证真实留存或删除。无并发调度、审批 UI、重启恢复、真实取消竞争或权限边界保证。探针的这些简化不是正式 Server 的实现决策。

## 兼容性维护

当前客户端输入不接受未知业务字段，因此未来增加字段需同步客户端 schema 与版本兼容测试，不能假定旧客户端容忍。`Tools()` 导出与 tools.json 由测试核对；真实客户端的发现 schema、调用、错误、结构化/文本结果另有固定证据回归。新客户端或生产链路必须重新实测，不能沿用 P0 模拟成功结论。

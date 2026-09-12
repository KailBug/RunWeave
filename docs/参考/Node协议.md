# Node 协议

状态：现行，Codec 与匹配校验已验证。日期：2026-09-11。适用范围：Node protocol_version=1；业务与结果摘要版本分别为 runweave-request-v1、runweave-outcome-v1。维护责任：当前工作项开发者。

依据：[ADR 0003](../决策/0003-执行证据与Node协议.md)、[状态与证据](../设计/执行状态与证据.md)。协议只支持声明的 v1，不按软件版本猜测兼容；MCP 协议协商与此无关。

## envelope 与连接

每条 WebSocket text 消息是一个 JSON 对象，原始与重新编码后的完整消息均不超过 1 MiB。不传原始 stdout 流。所有字段沿用业务 JSON 的精确大小写、无重复键/null/非法 Unicode、整数和深度限制。

`{protocol_version,type,message_id,connection_epoch,execution_id?,payload,extensions?}`：前五项（除 execution_id）和 payload 必填；message_id、非空 epoch、execution_id 均为[业务标识符](业务请求与结果.md)。execution_id 在 execution.*、result.receipt、artifact.* 必填，在注册和心跳禁止出现。不能用空值替代省略。

extensions 为可省略对象，最多 16 个标识符键，每项 JSON 编码最多 4096 字节；值不允许 null，其他有界合法 JSON 可忽略。不认识的 envelope/payload 字段仍拒绝；需要对方理解才安全的扩展不能放入此处。

连接外层先认证绑定 Node 身份，非 loopback 使用正常证书验证的 TLS；token 不出现在消息里。首次 register 的 epoch 必须空，registered 的 epoch 是 Server 为这次已认证连接生成的新标识。之后每帧必须匹配保存的 epoch，旧会话失效；epoch 不是可由调用方自报的身份凭据。解码上下文的 sender、node_id、epoch 必须由真实会话构造。

每连接一个受控写循环；写控制帧 deadline 为 5 秒，心跳间隔 10 秒、离线阈值 30 秒，以 Server 接收时间判断。取消/心跳优先于 artifact；每个产物请求最多 64 KiB，一次至多一个在途产物请求，以保证有界队列与控制消息进展。超时/满队列关闭会话并保留持久事实；不将连接错误视为执行失败。本阶段只定义规则，网络循环由 P1 实现。

## 类型与 payload

N→S 为 Node 发往 Server；S→N 相反。表中所有列出字段必填，`?` 表示可省略。

| type / 方向 | payload | 说明 |
| --- | --- | --- |
| register / N→S | node_id, software_version, journal_instance_id, config_version, capabilities, resources | node_id 必须等于认证身份；完整快照，不含物理路径 |
| registered / S→N | node_id, heartbeat_ms, offline_after_ms, reconcile_required | node_id 与身份一致；v1 固定 10000/30000/true；注册完成仍不可调度 |
| heartbeat / N→S | config_version | 仅表示存活，不能解除 UNKNOWN 或释放槽位 |
| execution.dispatch / S→N | request_digest, request | request 用 P0-02 严格校验，摘要重算必须一致 |
| execution.accepted / N→S | request_digest, evidence | evidence.phase=RECEIVED，接收防重已持久化 |
| execution.approval_required / N→S | request_digest, evidence | phase=WAITING_APPROVAL，审批提示见下文 |
| execution.started / N→S | request_digest, evidence | phase=RUNNING，实际启动已确认 |
| execution.result / N→S | request_digest, evidence | phase=FINISHED，带 Outcome |
| execution.cancel / S→N | request_digest | 保存取消意图，不是已取消确认 |
| execution.query / S→N | request_digest | 查询这一个 execution 的当前 journal |
| execution.snapshot / N→S | query_message_id, request_digest, evidence | 必须关联尚在等待的 query；允许 ABSENT 或 START_COMMITTED |
| result.receipt / S→N | request_digest, result_digest | Server 已提交完全相同结果后确认 |
| artifact.request / S→N | artifact_id, principal_id, offset, limit | principal_id 由 Server 认证上下文传递；Node 再做当前内容授权 |
| artifact.response / N→S | request_message_id, artifact_id, offset, data?, bytes_returned?, eof?, error? | 成功 data 为标准 Base64，最多 64 KiB；错误不能带成功字段 |

protocol 错误通过本地 Decode/Encode 返回错误，不伪造 execution 的 Result。本阶段没有约定可恢复的 protocol.error 帧；运行时拒绝非法/旧 epoch/反向帧并关闭该会话，在脱敏日志记录原因。单 execution 请求摘要与已存记录冲突也不能重执行。

## 注册与证据

config_version、evidence.revision、policy_version、mapping_version 均为正 JSON 安全整数，ABSENT 的 revision 唯一允许 0。software_version 为最多 64 字节的标识符；journal_instance_id 是持久状态目录实例标识，不是一次启动的随机值。重启保留它；更换 journal 实例必须由 Server 隔离处理，不能自动清空旧槽位。

capabilities 最多 32 个唯一标识符；resources 最多 64 项，每项 `{available,return_content,evidence}`，evidence 使用 P0-02 ResourceEvidence，resource 不重复。return_content 是能力快照提示，不替代当前授权；config_version 单调性由 Server 与既有快照比较，Codec 只检查范围。更新配置通过新连接重新发送完整 register，不在心跳里偷换快照。

evidence 为 `{revision,phase,approval?,outcome?,start?}`，phase 与状态映射见[状态与证据](../设计/执行状态与证据.md)。WAITING_APPROVAL 必须带 approval=`{approval_id,expires_at,policy_version,mapping_version}`；expires_at 为最多 64 字节 RFC3339 时间。其他阶段禁止 approval，FINISHED 必须带 outcome，其他阶段禁止 outcome。只有 UNRESOLVED 必须带 start=started/uncertain，其他阶段禁止该字段。UNRESOLVED 经 snapshot 报告，不能假作 started/result。Node 的真实审批记录还必须绑定 execution、Node、请求摘要、完整 program/argv/cwd、资源与策略版本；远端提示不授予批准权。

ABSENT 只能作为匹配 query 的 snapshot，不得作为 accepted 或 result。未确认结果即使已收到 receipt，也仍在防重窗口保留结果/终态 tombstone，不能让迟到 dispatch 被当作新任务启动。Server 重连应查询全部未解决执行；Node 的未确认结果主动逐条补报，不用无界全量快照塞入一帧。

## 产物与匹配

artifact_id/principal_id/request_message_id/query_message_id 使用标识符。offset 非负安全整数，limit=1–65536，offset+limit 不溢出安全整数。response 成功三个字段 data/bytes_returned/eof 均必填，bytes_returned 等于 Base64 解码长度，offset+bytes_returned 仍在安全范围；错误只允许 ARTIFACT_NOT_FOUND/ARTIFACT_UNAVAILABLE/ARTIFACT_EXPIRED/FORBIDDEN/IO_ERROR，message 非空最多 1024 字节且无 NUL。

接收 snapshot/receipt/产物响应必须校验原请求 message_id、execution、当前 Node/epoch、摘要或 artifact_id、offset/limit；孤立的合法 JSON 不能推进状态。`MatchReply` 只匹配传入的请求/响应，真实在途请求表和一次消费由后续会话服务实现。迟到查询响应若不再在途则不得设置 reconciled=true。

## 验证范围

合法/非法帧、错误方向/epoch、版本、摘要、回执/查询匹配、扩展/帧限额由[协议测试](../../internal/protocol/protocol_test.go)验证；实际结果见[本项验证](../验证/2026-09-11-P0状态与协议验证.md)。生产认证、单写队列、事务确认、deadline、调度 readiness、重装隔离与产物授权尚未实现。

# Node 协议样例

状态：现行。日期：2026-09-11。适用范围：P0-03 / Node v1。维护责任：当前工作项开发者。

[消息样例](消息样例.json)由[协议测试](../../../internal/protocol/protocol_test.go)逐条读取，包含 14 类合法消息和 3 个非法例。sender/epoch 是测试的外层可信上下文，认证 Node 固定为 node-a；valid 是期望结果。数据虚构，不代表这些 Node/execution 实际存在。

receipt 样例中的全 a 摘要仅展示格式，不能确认 result 样例；MatchReply 测试会验证它被拒，并使用实际 OutcomeDigest 构造匹配的回执。不能把语法合法等同已关联或已提交。

字段见[协议参考](../../参考/Node协议.md)，结果见[验证记录](../../验证/2026-09-11-P0状态与协议验证.md)。

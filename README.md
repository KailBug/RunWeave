# RunWeave

使用 Go 开发的 MCP 执行后端。先通过 Codex、nanobot、loopx 等外部 Harness 验证，再开发自有 Harness。

P0 技术与契约阶段已完成：已有独立 MCP stdio 探针、SQLite/WebSocket/YAML/Linux 进程组实验、业务请求/结果及 Node 状态契约，并完成真实 Codex 的六工具契约验证。下一阶段为 P1；正式 Server、Node、远程文件操作、进程执行和持久化服务尚未实现。

## 构建与检查

需要 Go 1.27.0。依赖版本由 `go.mod` / `go.sum` 固定。

```powershell
go build -o bin/mcp-probe.exe ./cmd/mcp-probe
./bin/mcp-probe.exe check
go vet ./...
go test ./... -count=1
```

`check` 启动真实子进程，验证 MCP 工具发现、输入/输出 schema、UTF-8 结构化返回、缺失参数拒绝和拒绝后的会话可用性；不调用模型。`serve` 通过 stdio 提供一个仅回显 nonce 的 `runweave_probe` 工具，stdout 专用于 MCP 协议。

Linux 可使用 `go build -o bin/mcp-probe ./cmd/mcp-probe` 后执行 `./bin/mcp-probe check`。Windows 交叉编译及 WSL 验证记录见 [P0 技术验证](docs/验证/2026-09-10-P0-MCP探针验证.md)。

基础依赖、Linux 进程组与 race 检查的复现步骤及结果见 [P0 基础依赖与进程验证](docs/验证/2026-09-10-P0基础依赖与进程验证.md)。这些测试使用临时数据库、loopback 连接和测试专用子进程，不开放业务执行入口。

业务字段、错误与幂等规则见[业务请求与结果](docs/参考/业务请求与结果.md)，JSON 样例、固定摘要及 Windows/WSL 验证见[P0 业务契约验证](docs/验证/2026-09-10-P0业务契约验证.md)。`internal/contract` 仅作纯校验，不创建 execution 或执行操作。

状态转换与 Node v1 消息契约已实现，见[状态与协议验证](docs/验证/2026-09-11-P0状态与协议验证.md)。六工具/schema 的真实客户端收尾已通过，见[验证记录](docs/验证/2026-09-11-P0真实客户端契约验证.md)；这是内存样例契约验证，尚未实现 Server–Node 业务链路。

六工具内存探针可用 `go build -o bin/mcp-contract-probe.exe ./cmd/mcp-contract-probe` 构建，再执行 `./bin/mcp-contract-probe.exe check`。真实 Codex 复验的准备、临时配置和证据核对见[指南](docs/指南/P0真实客户端验证.md)。

## 外部 Harness 验证

Codex 的临时启动方式（PowerShell；路径按实际工作目录调整）：

```powershell
codex -c 'mcp_servers.runweave_probe.command="D:/RunWeave/bin/mcp-probe.exe"' -c 'mcp_servers.runweave_probe.args=["serve"]'
```

验证提示：`调用 runweave_probe 工具，nonce 使用 rw-codex-p0-001，报告实际返回的 nonce 和 stage。`

期望 `nonce` 原样返回、`stage` 为 `p0-probe`，并且客户端存在实际工具调用记录。只看到模型回复相同文本不算验证通过。此工具不等于规划中的 `runweave.execute`。

配置依据：[Codex 官方 MCP 文档](https://developers.openai.com/codex/mcp)。nanobot、loopx 的具体项目、版本及接入方式待确认，不预设其配置与 Codex 相同。

## 开发文档

后续开发文档统一保存到 `docs/`，使用简体中文，并随代码同步维护。

- [开发文档索引](docs/README.md)
- [开发文档维护规范](docs/开发文档维护规范.md)
- [当前开发进度](docs/进度/开发进度.md)

## 总体规划资料

根目录只维护两份总体规划，不列入开发文档：

- [RunWeave 项目规划书](RunWeave项目规划书.md)：长期方向、产品范围与核心边界。
- [RunWeave 3 个月开发规划书](RunWeave3个月开发规划书.md)：MCP 首版的 13 周目标、工作包和验收。

# RunWeave

使用 Go 开发的 MCP 执行后端。先通过 Codex、nanobot、loopx 等外部 Harness 验证，再开发自有 Harness。

当前处于 P0：仅实现独立的 MCP stdio 协议探针。Server、Node、远程文件操作、进程执行和持久化尚未实现。

## 构建与检查

需要 Go 1.27.0。依赖版本由 `go.mod` / `go.sum` 固定。

```powershell
go build -o bin/mcp-probe.exe ./cmd/mcp-probe
./bin/mcp-probe.exe check
go vet ./...
```

`check` 启动真实子进程，验证 MCP 工具发现、输入/输出 schema、UTF-8 结构化返回、缺失参数拒绝和拒绝后的会话可用性；不调用模型。`serve` 通过 stdio 提供一个仅回显 nonce 的 `runweave_probe` 工具，stdout 专用于 MCP 协议。

Linux 可使用 `go build -o bin/mcp-probe ./cmd/mcp-probe` 后执行 `./bin/mcp-probe check`。Windows 交叉编译及 WSL 验证记录见 [P0 技术验证](RunWeave-P0-技术验证记录.md)。

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

根目录现有文档属于项目总体规划，不列入开发文档。

- [最小实现任务清单](RunWeave-v0.0.1-最小实现任务清单.md)
- [实施规划](RunWeave-v0.0.1-实施规划.md)
- [产品范围](RunWeave-v0.0.3-项目规划书.md)

# RunWeave P0 技术验证记录

日期：2026-09-10。P0-01 进行中；本记录不代表 P0 或 v0.0.1 已验收。

## 已确认的开发顺序

用户确认先用 Go 开发 MCP 执行后端，以 Codex、nanobot、loopx 等外部 Harness 验证。MCP 功能打通以后再开发自有 Harness，其语言本阶段不锁定。

第一条业务链路仍为 MCP → Go Server → Linux Node → 授权目录列表 → 持久结果查询。当前探针仅验证 SDK 与 stdio，不代替该链路。

## 环境与锁定项

| 项目 | 实际观测 |
| --- | --- |
| 构建工具链 | Go 1.27.0，windows/amd64 |
| MCP SDK | 官方 `github.com/modelcontextprotocol/go-sdk v1.7.0`，固定在 go.mod/go.sum |
| SDK 自测协议 | `2026-07-28`；不能据此推断所有客户端的协商版本 |
| Codex CLI | 0.153.4 |
| Linux 执行环境 | Ubuntu WSL2，Linux 6.6.87.1-microsoft-standard-WSL2，x86_64 |
| WSL Go | 未安装；当前使用 Windows 交叉编译的 Linux 二进制 |
| Docker | CLI 存在，daemon 未可用；未启动 Docker Desktop |
| nanobot / loopx | PATH 未发现对应命令，具体项目与版本待确认 |
| SQLite / WebSocket / YAML | 尚未选择并验证具体版本 |

以上是本机实际版本记录，不是完整工具链安全审计或多版本兼容承诺。

## 当前代码

`cmd/mcp-probe/main.go` 是独立 P0 探针，提供：

- `serve`：官方 SDK 的 stdio server；唯一工具 `runweave_probe` 只回显 nonce 和固定 stage，无文件、网络或命令执行行为。
- `check`：官方 SDK client 启动真实 server 子进程并使用标准输入/输出管道调用。
- 必填字符串 nonce、输入/输出 schema、结构化结果和文本兼容内容。该探针 schema 不作为业务请求最终契约。

工具标记只读是对实际行为的描述，不代替未来 Node 授权。

## 已通过检查

| 检查 | 结果 |
| --- | --- |
| Windows 构建 | 成功 |
| Windows `mcp-probe.exe check` | 通过，实际子进程 stdio |
| Linux amd64 / CGO_ENABLED=0 交叉编译 | 成功 |
| Ubuntu WSL 执行 Linux `check` | 通过，实际运行而非仅交叉编译 |
| `go vet ./...` | 通过 |

`check` 验证：恰好发现预期工具、包含输入和输出 schema、UTF-8 nonce 原样返回、stage 正确、存在非空文本内容、缺失 nonce 被拒且会话仍可用。检查有 20 秒总 deadline，退出时关闭子进程会话。尚未增加业务单元测试；这些检查不能覆盖未来执行后端的安全与故障语义。

复现命令（PowerShell，仓库根目录）：

```powershell
# 以下缓存重定向用于受限工作区，普通本机开发可以省略。
$env:GOPATH = "$PWD/.cache/gopath"
$env:GOMODCACHE = "$PWD/.cache/gomod"
$env:GOCACHE = "$PWD/.cache/gobuild"
go build -o bin/mcp-probe.exe ./cmd/mcp-probe
./bin/mcp-probe.exe check
go vet ./...

$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
go build -o bin/mcp-probe-linux-amd64 ./cmd/mcp-probe
wsl -d Ubuntu --exec /mnt/d/RunWeave/bin/mcp-probe-linux-amd64 check
# 在单独的 PowerShell 会话执行交叉编译，避免目标变量影响后续本机构建。
```

## 外部客户端验证

Codex 使用进程级 `-c mcp_servers...` 配置探针，`--ephemeral --ignore-user-config -s read-only`，未写入全局 MCP 配置。官方配置字段参见 [Codex MCP 文档](https://developers.openai.com/codex/mcp)。

初次沙箱运行遇到证书 `UnknownIssuer`，已终止重试；正常系统权限下可继续模型调用。随后实际工具调用因非交互审批策略被拒，未返回工具结果。后续只对该无副作用探针设置进程级工具许可，并验证工具发现行为。最终结果见下面的更新记录。

不得把配置解析成功、工具出现或模型复述期望值视为真实调用成功。nanobot、loopx 尚未验证。

### 最终更新：Codex 真实调用通过

2026-09-10，Codex CLI 0.153.4 返回实际 `mcp_tool_call` 完成事件：server/tool 均为 `runweave_probe`，状态 `completed`，error 为 null。输入 `nonce=rw-codex-p0-001`，输出同时包含文本和 structured_content：

```json
{"nonce":"rw-codex-p0-001","stage":"p0-probe"}
```

验证配置在普通命令基础上增加：

```text
-c 'mcp_servers.runweave_probe.required=true'
-c 'mcp_servers.runweave_probe.tools.runweave_probe.approval_mode="approve"'
```

以上工具许可仅适用于此无副作用探针和本次进程，不用于未来 process.exec。提示允许客户端先发现延迟加载的工具，再调用；一次先前尝试仅报告不可见，最终尝试取得了真实调用结果。CLI 输出没有记录协商协议版本，该项未推断。此次通过的是探针调用，不是完整 A02 业务验收。

## P0 剩余出口条件

1. Codex 探针调用已通过；补充实际协议协商记录，后续业务工具需重新进行完整客户端验收。
2. 验证并锁定 SQLite、WebSocket、YAML 依赖；检查迁移、事务、帧限制及断线处理。
3. 验证 Linux 进程组取消、超时、后代回收；当前 SDK 子进程关闭不等于 process.exec 生命周期验证。
4. 冻结业务 JSON 契约、错误码、状态转换和 Server–Node 协议。
5. 两个真实 Linux Node 的发布验收后续单独安排；当前只有一个 WSL 环境的探针结果。

SDK 实现依据：[官方 Go SDK quick start](https://github.com/modelcontextprotocol/go-sdk/blob/v1.7.0/docs/quick_start.md)。本地使用锁定版本源码核实接口，没有自写 JSON-RPC 协议栈。

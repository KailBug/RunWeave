# P1 身份接入与 Server 运行

状态：现行。更新日期：2026-09-14。适用范围：v0.0.1-dev / P1 第二批。维护责任：当前工作项开发者。

## 目标与前提

启动真实 Server，创建独立 principal/Node 身份，通过 CLI 认证，再验证在线撤销。此时 Node 尚未注册或可调度，不能执行 fs.list，MCP 配置只用于 principal 认证检查。完整定义见[身份与 HTTP 接入](../参考/身份与HTTP接入.md)。

Windows PowerShell，仓库根目录 `D:\RunWeave`，需要 Go 1.27.0 及已下载依赖。状态和原始 token 放在被 Git 忽略的受保护目录，示例只监听 `127.0.0.1:7788`。首次使用下面的身份 ID，已有身份/文件时不要覆盖，应复用仍有效的文件或有意识地选择新 ID。

## 构建和初始化

```powershell
go build -o bin/runweave.exe ./cmd/runweave
./bin/runweave.exe state init --config docs/资源/P1配置/server.yaml
./bin/runweave.exe identity create --config docs/资源/P1配置/server.yaml --role principal --id principal-local --token-file secrets/principal-local.token
./bin/runweave.exe identity create --config docs/资源/P1配置/server.yaml --role node --id node-local-1 --token-file secrets/node-local-1.token
```

逐条检查 `$LASTEXITCODE`。Server schema 返回 2；创建返回 role/id，原始 token 只写指定文件。当前 CLI 路径与[现有 MCP/Node 配置样例](../参考/配置与状态存储.md)中的引用相匹配，无需手工复制 token 到 YAML。

## 启动、认证和撤销

在终端 A 启动，保持运行：

```powershell
./bin/runweave.exe server serve --config docs/资源/P1配置/server.yaml
```

启动日志在 stderr，stdout 为空。使用 Ctrl+C 正常停止；服务完成 HTTP 关闭后释放数据库和锁。重复启动相同状态目录的第二个 daemon 会失败。

在终端 B（同一仓库目录）执行：

```powershell
Invoke-RestMethod http://127.0.0.1:7788/healthz
./bin/runweave.exe auth check --config docs/资源/P1配置/mcp.yaml
./bin/runweave.exe auth check --config docs/资源/P1配置/node.yaml
./bin/runweave.exe identity revoke --config docs/资源/P1配置/server.yaml --role node --id node-local-1
./bin/runweave.exe auth check --config docs/资源/P1配置/node.yaml
```

前两次认证分别返回 principal-local 与 node-local-1。撤销后最后一条预期退出 1，凭据文件仍在，但不再有效。无需重启 Server。撤销不可恢复且旧 ID 不可重建；该演示会消耗这个 Node 身份，需要继续接入时创建新 ID/token，并更新自己的 Node 配置。不要修改公共样例来冒充已完成 Node 注册。

`/readyz` 当前固定 503，原因 `execution_service_unavailable`；健康成功、认证成功与业务可接单是三个不同条件。

## 可重复的产品二进制检查

```powershell
./docs/资源/P1身份/验证Server身份.ps1
```

[脚本](../资源/P1身份/验证Server身份.ps1)在独立的 `state/p1-identity-smoke-<随机值>` 中生成测试配置/凭据，隐藏窗口启动刚构建的产品二进制，使用临时 loopback 端口。检查两类身份、在线撤销、第二 daemon 拒绝和 stdout 隔离；finally 撤销测试身份并只终止自身创建的进程，随后确认状态可重新打开。它验证产品进程强制退出后的锁释放；优雅 context 关闭另由 Go 子进程测试验证，不将两者混同。

脚本保留独立测试目录供排查，不输出原始 token、不覆盖既有状态。清理前核实该目录由本次脚本创建且无运行进程；不能按名称批量杀 Server 或删除活跃锁文件。

## TLS 与 Linux

非 loopback 配置必须使用 TLS。在 Server 配置加入成对 `tls_cert_file`/`tls_key_file`，客户端 `server_url` 使用证书覆盖的 HTTPS 主机名；内部 CA 可用 `tls_ca_file` 指定 PEM 根。配置路径相对于配置文件，TLS 私钥和 token 必须属于运行账号且限权，不能通过 insecure 参数绕过校验。

Linux 文件用 `chmod 600` 且确保 owner 是运行账号；Windows 需要受保护的合适 ACL，`identity create` 会为 token 自动创建当前用户专用 ACL。外部提供的私钥需按[权限参考](../参考/身份与HTTP接入.md)检查，程序不自动改变已有私钥权限。

Linux 命令同名，构建二进制不带 `.exe`，正常停止使用 Ctrl+C/SIGTERM。本次 WSL 实际运行七个新增/受影响包的交叉编译测试，覆盖真实 HTTP/TLS、凭据权限和 CLI 子进程；没有以编译通过代替 Linux 运行。复现命令见[第二批验证](../验证/2026-09-14-P1身份与Server验证.md)。真实远程机器证书部署和 Windows 控制台 Ctrl+C 未单独实测；TLS 正负例使用本机临时证书和实际加密连接。

## 排障与恢复

- 配置预检成功但启动失败：预检不读凭据/证书，检查实际文件、owner/ACL、证书与私钥配对和端口占用。
- 身份管理提示初始化/版本问题：停止其他管理者，运行新版本 `state init` 升级到 Server v2；管理命令不自动迁移。
- 创建失败且文件保留：这是跨文件/数据库提交边界，不要覆盖或在日志输出文件内容；使用 auth check 核实，需要消除不确定有效性时显式撤销对应身份。
- auth check 失败：核对角色、Node ID、撤销事实、凭据文件和 TLS 信任/主机名；系统 HTTP 代理和重定向不会被使用。
- 停机备份恢复：沿用[状态目录指南](P1启动前检查与状态目录.md)，同时停止身份管理命令。恢复撤销前备份会带回旧有效凭据状态，开放服务前重新撤销；不要同时运行原实例和副本。

# 身份与 HTTP 接入

状态：现行。更新日期：2026-09-14。适用范围：P1 第二批 / v0.0.1-dev / Server schema v2。维护责任：当前工作项开发者。

## 身份和权限边界

依据 [ADR 0006](../决策/0006-P1身份与Server生命周期.md)。实现：[identity](../../internal/identity/identity.go)、[身份存储](../../internal/store/identity.go)、[HTTP Server](../../internal/server/server.go)、[控制客户端](../../internal/controlclient/client.go)。

`principal` 与 `node` 是两个独立命名空间，标识符复用 P0 `ValidIdentifier`。相同 ID 可以分别属于两种角色，但 token 不通用。Server 从 token 摘要记录得出身份，不信任 query、body 或 `X-Principal-ID` 等自报值。Node 认证检查路径中的 ID 必须与认证结果相同。

每个身份创建一个 token：32 字节密码学随机数，Raw URL Base64 编码，前缀分别为 `rw_p_`、`rw_n_`，总长 48 字节；严格校验格式与角色。Server 只持久化完整 token 的 SHA-256 摘要。格式前缀不能代替数据库中的角色/状态检查；不存在、已撤销或角色不符均认证失败。

创建不覆盖已有身份。撤销为不可逆操作，身份与关联 token 在一个事务中失效；重复撤销成功，不存在的身份报错。撤销后禁止用相同角色/ID 重新创建。本批没有 token 轮换、恢复撤销或远程管理接口；需要新凭据时先明确影响，再使用新身份。未来恢复旧 execution/Node journal 不能靠重建身份绕过证据要求。

本批身份检查只证明认证，尚不授予文件、program 或 execution 权限。Node 尚未注册、未对账，也不可调度。

## 本地身份命令

所有命令由 `runweave` 执行。使用 `--config FILE` 指向 Server 配置，本机 OS 账号必须有该状态目录的管理权限。命令结果输出一个 JSON 对象，诊断只到 stderr；参数名严格检查，允许调整参数顺序，拒绝重复或多余参数，不回显错误参数内容。

| 命令 | 参数与行为 | 成功 stdout |
| --- | --- | --- |
| `identity create` | `--config FILE --role principal或node --id ID --token-file FILE`；独占创建原始 token 文件，再提交身份事务；不覆盖文件 | `{"role":"principal","id":"local-user"}` |
| `identity revoke` | `--config FILE --role principal或node --id ID`；撤销身份与 token，可在 daemon 运行时调用 | `{"role":"principal","id":"local-user","revoked":true}` |
| `auth check` | `--config FILE` 使用 Node/MCP 配置；读取对应凭据，通过 HTTP 检查实际身份 | `{"role":"node","id":"node-local-1"}` 或 principal |
| `server serve` | `--config FILE` 使用 Server 配置；启动长期持锁 HTTP daemon，收到中断后停止 | 不写 stdout，启动地址/停止事件在 stderr |

成功退出码 0，失败 1。管理和认证检查上下文上限 10 秒；`server serve` 只对启动存储检查限时，运行生命周期持续到停止。

CLI `--token-file` 的相对路径以**当前工作目录**为基准；YAML 内的 `token_file` 则以**配置文件目录**为基准。两者应指向同一文件。初次使用先 `state init` 或启动 Server；身份管理只访问已初始化的最新 Server schema，不执行迁移、不创建库、不持 daemon 锁。升级/备份时仍必须停止全部管理命令。

创建跨越文件与 SQLite 两种介质：文件成功写入并 Sync 后才提交数据库；如果后续数据库失败，退出 1 并保留私有文件供核对。不能把保留文件当作身份已创建，也不要自动重试覆盖它。可用匹配的 Node/MCP 配置执行 `auth check` 核对；若需消除不确定有效性，可对相应身份执行撤销。现有身份/文件不自动替换。

## 凭据与 TLS 文件

[secretfile](../../internal/secretfile/secretfile.go) 创建时拒绝已有路径，权限在打开文件时设置，不先公开创建再 chmod。

- Linux：普通文件、owner 等于当前有效 UID、mode 无 group/other 位；`O_NOFOLLOW` 打开后从同一文件句柄检查，拒绝最终符号链接和非普通文件。
- Windows：创建时使用当前用户独占的受保护 DACL；读取时核对 owner 为当前用户、DACL 非空，常规允许 ACE 的 SID 只接受当前用户、SYSTEM 或 Administrators。其他允许项、未知 ACE、最终 reparse point/目录均拒绝。不会把 POSIX `0600` 当作 ACL 验证。已有文件不自动改权限。

父目录必须由可信 OS 账号控制，不保护同账号恶意进程。跨机器复制 token/私钥后，应让运行账号取得所有权并设置上述权限。不能把含明文 token 的文件放进仓库、URL 或日志。Linux 权限/链接、Windows 宽 ACL 拒绝已有实测；Windows reparse point 和不同 owner 的负例未单独实测。

token 文件读取上限 256 字节，仅容许 48 字节 token 后带一个 LF 或 CRLF，不接受 BOM、空白包裹、多行或空文件。创建命令写 LF。私钥同样要求限权文件，上限 64 KiB；公钥证书、可选 CA 文件上限 128 KiB。

Server 非 loopback 必须 TLS；成对证书/私钥在监听和状态打开前读取、匹配。TLS 最低 1.2。客户端校验信任链和主机名，`node.tls_ca_file` / `mcp.tls_ca_file` 可选，用指定 PEM CA 扩充系统根；不提供跳过验证开关。HTTP 仅允许 loopback IP 字面量。控制客户端不读取环境 HTTP 代理、不跟随重定向，避免 bearer 被转发。

## HTTP 参考

本批只支持 GET。业务身份端点通过单个 `Authorization: Bearer <token>` 头认证；拒绝多个 Authorization 头、query 和非空 body，不从 query/body/cookie 取凭据。不会返回原始 token 或底层存储错误细节，也不开放 CORS 授权头。

| 路径 | 认证 | 成功/预期响应 |
| --- | --- | --- |
| `/healthz` | 无 | 200，`{"status":"ok"}`，只说明 HTTP 存活 |
| `/readyz` | 无 | **503**，`{"ready":false,"reason":"execution_service_unavailable"}`；P1 执行服务尚未接入 |
| `/v1/principal` | principal | 200，`{"role":"principal","id":"local-user"}` |
| `/v1/nodes/{node_id}/identity` | node，路径必须匹配身份 | 200，`{"role":"node","id":"node-local-1"}` |

错误对象为 `{"error":"CODE"}`：400 `INVALID_REQUEST`（query/body）、401 `UNAUTHORIZED`（缺失/错误/未知/撤销/角色混用凭据）、403 `FORBIDDEN`（有效 Node 冒用其他 ID）、405 `METHOD_NOT_ALLOWED`、503 `STORAGE_UNAVAILABLE`（认证存储查询失败）。未知路由由标准 HTTP mux 返回 404；上表端点之外不构成公开业务 API。

每次认证查询已提交的身份/token 状态；撤销提交后新发起的认证拒绝，不缓存“曾有效”的 token。已经通过认证的在途请求不承诺撤回；未来 WebSocket 的长连接撤销与旧 epoch 隔离须另行实现，不能从本批 HTTP 检查外推。

Server read-header 5 秒、read/write 10 秒、idle 30 秒、MaxHeaderBytes 16 KiB；关闭有 5 秒宽限。客户端总超时 10 秒、TLS 握手与响应头各 5 秒，成功响应正文最多 4096 字节；结果沿用严格 JSON 解码，拒绝未知/重复字段和身份不匹配。错误正文不透传给 CLI，连接错误不输出含密 URL/头。

## 数据与恢复

新增迁移：[Server 002](../../internal/store/migrations/server/002_identities.sql)。Server v1→v2 保留 `store_identity.instance_id`，Node 仍 v1；旧程序对 v2 拒绝打开，不删除历史或自动降级。

| 表 | 字段与约束 |
| --- | --- |
| `principals` | `id` 主键、`created_at`、可空 `revoked_at` |
| `node_identities` | 同上，独立命名空间；不代表 runtime Node 已注册 |
| `token_records` | `role`、可空 `principal_id`/`node_id` 外键、32 字节 `digest` 主键、`created_at`、可空 `revoked_at`；CHECK 保证角色与恰好一个身份外键一致；外键唯一，限制每身份一个 token |

创建身份/token 与撤销身份/token 各自为短事务；注入 token 更新失败时已验证身份撤销回滚，不出现半撤销。身份管理连接不迁移，daemon 仍独占整个运行生命周期。

仅停机完整复制状态目录：停止 daemon 和身份管理，保留配套 SQLite 文件。备份中可能包含有效或撤销的身份摘要；还原到撤销前备份会恢复当时有效状态，重新开放监听前须重新撤销相关身份，不能将旧备份当作撤销事实的最新副本。原始 token 文件独立保护/备份，不在数据库中；丢失后本批不能重新显示。实际兼容和故障证据见[第二批验证](../验证/2026-09-14-P1身份与Server验证.md)。

# ADR 0006：P1 身份与 Server 生命周期

状态：已接受。日期：2026-09-14。适用范围：P1-01/03 第二批。维护责任：当前工作项开发者。

## 背景与决定

沿用 [ADR 0005](0005-P1配置与状态存储基础.md) 的配置/锁/迁移基础，本批实现长期持锁的 Server HTTP daemon、principal 与 Node 独立身份、创建/撤销、凭据限权读写和真实 TLS 认证检查。Node 注册、epoch 与执行仍由 P1-04 之后实现。

- Server schema 追加 v2，不改 v1 SQL；Node 保持 v1。Server 保存 `principals`、`node_identities`、`token_records`。身份由 `(role,id)` 区分，每个身份初次创建一个高熵 token；原文只写本机限权文件，库内只保存 SHA-256 摘要。原始 token 不进 stdout、日志、URL 或 YAML。
- 首版身份创建不覆盖已有 ID；撤销身份与其 token 同事务完成，重复撤销幂等，撤销后不能重用 ID。轮换/恢复身份不在本批开放，避免丢失 journal 的 Node 偷换凭据后被当作原实例。需换身份时创建新 ID。
- 身份管理是具有 Server 状态目录访问权限的本地管理操作，不开放远程管理 API。`identity create/revoke` 通过独立短连接访问已初始化的 Server v2 库，不持 daemon 锁、不迁移；只提供身份事务，不提供执行/调度能力。管理员可以在 daemon 运行时撤销；每次 HTTP 认证重新查询已提交事实，不缓存 token。撤销提交后开始的请求必定拒绝；已认证在途请求不承诺回滚。后续长连接必须另行接入撤销隔离。
- 创建顺序为生成 token → 独占创建限权文件并 Sync → 提交身份/token 事务。已有目标文件不覆盖。文件成功但数据库失败时保留文件并返回错误，避免不确定提交后删掉唯一凭据；调用者核对身份后再清理，不假定跨数据库/文件系统原子性。
- Linux 凭据文件要求当前 UID 所有、无组/其他权限、普通文件，使用 no-follow 打开并从同一文件句柄复核；Windows 创建时赋予当前用户专用受保护 DACL，读取时从句柄核对 owner 与 ACL，允许当前用户、SYSTEM、Administrators 的常规允许项，其他允许项/未知 ACE/空 DACL 拒绝。最终符号链接/reparse point 拒绝。父目录由专用 OS 账号保护，同账号恶意文件替换不在隔离承诺内。
- 仅新增 `GET /healthz`、`GET /readyz`、`GET /v1/principal` 和 `GET /v1/nodes/{node_id}/identity`。认证只取 Authorization Bearer，身份从数据库得出；Node 路径 ID 必须等于认证 Node ID。错误 token、角色混用、撤销均返回统一 401；有效 Node 冒用另一 ID 返回 403。这些接口不构成注册、资源授权或执行权限。
- readiness 与 liveness 分离：本批 `/readyz` 固定 503，说明 execution 服务尚未接入，不能误称可接单；`/healthz` 只证明 HTTP 服务存活。
- Server 非 loopback 强制 TLS，证书/私钥在监听前读取验证；私钥使用限权读取。客户端正常校验证书与主机名，可显式配置 `tls_ca_file` 扩充系统根，不提供 insecure 开关。客户端禁止重定向、禁用环境代理以免凭据被意外转发；请求/响应有界且有超时。
- daemon 持有 Store 到 HTTP 停止，收到 context/信号后有界 Shutdown，超时则 Close，再关数据库和锁。启动失败清理所有已取得的资源。公共日志不记录请求头/完整 URL/凭据，stdout 不写 daemon 日志。

## 备选与后果

停机后才能撤销会拉长凭据有效窗口，故选择本地短事务；本地 socket 管理接口会新增平台 IPC 与管理认证，本批不引入。该例外仅允许身份管理，不能绕过 daemon 独占运行约束。

高熵随机 token 不采用面向人类密码的慢哈希：256 bit 随机性满足本用途；摘要和明文格式严格校验。每身份单 token 简化本批撤销语义，代价是暂不提供无停机轮换。

## 验收与限制

验证 v1→v2 保留实例 ID/失败回滚、角色隔离/摘要存储/事务撤销/重开仍撤销、daemon 在运行时管理、第二 daemon 拒绝与停止释放、凭据权限负例、不覆盖文件、真实 HTTP/TLS 正负例（错误信任链/主机名/重定向）、客户端输出不泄漏 token。生产 Node WebSocket 撤销、真实 fs.list 和 MCP 仍未实现，不能标 A02/A12 全量通过。

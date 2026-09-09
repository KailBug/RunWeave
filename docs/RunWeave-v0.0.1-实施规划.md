# RunWeave v0.0.1 实施规划

日期：2026-09-09。状态：开工前方案；没有业务代码实现，也没有运行验证结果。

配套材料：[可执行性与架构审计](./RunWeave-v0.0.1-可执行性与架构审计.md)、[原产品规划](./RunWeave-v0.0.2-项目规划书.md)。

用户已确认首版选择 **MCP 执行层、Go、Linux Node 优先**。本文将这个选择落实为建议的范围、边界、契约与验收条件。原文件名中的 v0.0.2 视为历史文档标识；本文的 v0.0.1 指软件首发版本，不覆盖原文的长期愿景。

## 1. 首版目标与边界

**让一个现有 Agent 通过 MCP，在两台用户管理的 Linux Node 之间，按明确的能力和资源需求执行操作，并得到可查询、可取消、可解释的结果。**

系统执行的是一次明确操作。自然语言目标拆解、生成命令、选择下一步、持续推进任务由外部 Harness 负责。RunWeave 可以在调用方退出后继续已提交的 execution，但不会在调用方停止后自动产生下一步任务。

首版使用边界为单一可信所有者、少量自管开发节点、非生产环境。多个人共享同一管理员身份不等于团队权限系统。默认部署在自管机器或私有网络；不要求控制平面在公有云。

| 首版交付 | 具体约束 |
| --- | --- |
| Server、Node、CLI、MCP 接入 | 一个 Go module，一个 `runweave` 二进制；不同子命令扮演不同角色 |
| 两个真实 Linux Node | Node 主动建立 WebSocket；不支持节点间直连 |
| 手工资源配置 | 首批 `repo://`、`dataset://`；本地物理路径留在 Node 配置 |
| 能力与资源 placement | 精确匹配、无队列、每 Node 最多一个执行槽 |
| 三种操作 | `fs.list`、`fs.read`、`process.exec`；不另做写文件、Git、Docker 专用工具 |
| 最小权限模型 | Server 身份校验；Node 的 ALLOW / ASK / DENY；本地 CLI 审批 |
| 长任务管理 | 提交、查询、显式取消、超时、重连对账、未知结果 |
| 结果与产物 | 有界结构化结果；Node 本地日志；按授权读取片段 |
| 可观察性 | 查询执行记录、placement 原因、错误代码、持久状态事件 |
| 最小持久化 | Server SQLite；Node SQLite 接收/审批/结果记录；文件保存输出 |

首版暂缓自有 Agent Loop、Web Chat/Dashboard、移动通知、完整 Task 状态机、DAG、自动重试、代码同步、自动依赖安装、PTY、进程脱离后台运行、插件平台、容器编排、强沙箱、多租户、远程 OAuth MCP 服务、多 Server HA。

`process.exec` 经授权可以修改文件，因此“不做 write_file 工具”不等于“首版只读”。这项能力必须按代码执行授权和描述。

## 2. 部署与技术栈

### 2.1 进程形态

Server 独占控制平面状态，处理 HTTP 管理 API 和 Node WebSocket。每个 Node 管理本地资源、策略、执行槽、子进程和产物。CLI 通过 Server API 查询和提交；Node 本地审批命令只操作该 Node 的本地审批记录。

支持 stdio 的第三方 Harness 启动 `runweave mcp` 薄代理，该进程通过认证 HTTP API 访问 Server。代理只转换 MCP 请求/结果，不维护队列、执行数据库或调度状态。这样先兼容一个实际客户端，后续可在 Server 内增加 MCP 网络入口，共用同一执行服务。

MCP stdio 的 stdout 仅用于协议，日志输出到 stderr。Server 不因 bridge 退出而退出。

首版正式验收 Server 与 Node 的 Linux 运行；Windows CLI/bridge 可作为便利目标，但只有完成构建及真实客户端测试后才标支持。Windows 原生 Node 后移，WSL2 测试不能代替原生 Windows 验收。

### 2.2 依赖选择

| 用途 | 建议 | 约束 |
| --- | --- | --- |
| 语言与基础服务 | Go、`net/http`、`context`、`log/slog`、`os/exec` | 选实施当时受支持且已打补丁的 Go 版本，精确记录工具链 |
| MCP | 官方 Go SDK | P0 用真实客户端验证；锁定 SDK 版本，不自写 JSON-RPC 栈 |
| 存储 | `database/sql` + 一个 SQLite 驱动 | 倾向纯 Go 的 `modernc.org/sqlite`；先验证构建、体积、SQL 迁移 |
| WebSocket | 一款维护中的 Go 库 | P0 验证帧限制、取消、写并发要求和断线检测；不自写协议栈 |
| 配置 | YAML 文件 + 环境变量凭据 | 配置 schema 有版本，未知字段报错，秘密不放命令行参数 |
| 测试 | Go testing、临时 SQLite、真实子进程、模拟连接故障 | 不引入工作流测试平台 |

官方 MCP Go SDK 已提供服务端/客户端与传输能力；具体兼容组合以锁定版本实测为准。[官方仓库](https://github.com/modelcontextprotocol/go-sdk)

`modernc.org/sqlite` 提供不依赖 CGO 的 SQLite 驱动，这是此处减少分发依赖的理由；最终选择仍以 P0 结果为准。[驱动文档](https://pkg.go.dev/modernc.org/sqlite)

## 3. 最小代码边界

以下是职责落点建议，不要求开工第一天创建全部目录。没有实现的模块不创建空壳。

| 路径 | 职责 | 依赖限制 |
| --- | --- | --- |
| `cmd/runweave` | 子命令、配置装配、进程生命周期 | 不写业务规则 |
| `internal/contract` | Request、Result、Operation、错误码 | 无数据库、网络和 MCP 依赖 |
| `internal/protocol` | Server–Node 消息、版本与校验 | 可引用 contract，不包含业务执行 |
| `internal/server` | 执行服务、placement、状态转换、连接管理 | placement 使用输入快照，不能直接读 socket |
| `internal/node` | 本地资源、授权、执行器、进程控制与输出 | 不调用 Server 的 placement；不引入 LLM |
| `internal/store` | 显式 SQL、事务、迁移；分 Server/Node store 类型 | 不依赖接入适配层，不决定调度 |
| `internal/api` | 认证 HTTP handler 与 API client | handler 调用 Server 执行服务，不复制规则 |
| `internal/mcp` | MCP 工具定义与 HTTP client 适配 | 不访问数据库、不直接派发 Node |
| `tests/integration` | 两节点、进程生命周期与故障验证 | 复用公共用例，不复制实现算法 |

Node 的策略、资源和进程实现可先用同包不同文件；只有复杂度或复用需求出现后再拆包。不要创建通用 `Manager`、`BaseService`、DI 容器、插件注册平台、CQRS 或 Repository 基类。

Server 的 Submit、Get、Cancel 是唯一业务入口。状态更新集中在执行服务；SQL 通过带当前状态/版本条件的更新阻止非法跳转。必要的小接口放在使用方，例如节点发送、时钟、进程控制；没有第二种实现也不需要隔离测试的对象，优先用具体类型。

## 4. 请求、资源与结果契约

### 4.1 执行请求

概念上包括 `request_id`、可选 `correlation_id`、`requirements`、`operation`、`timeout_ms`。`request_id` 是调用方在重试中保持稳定的幂等键，作用域为已认证 principal；correlation_id 只分组，不授予权限、不代表持久 Agent Task。

以下是契约样例，不是实现代码；revision 是示意值，实际校验必须使用真实版本证据。

```json
{
  "request_id": "demo-001-test",
  "correlation_id": "demo-001",
  "requirements": {
    "capabilities": ["process.exec", "python"],
    "resources": [{"uri": "repo://demo", "revision": "git:<full-commit>", "clean": true}],
    "os": "linux"
  },
  "operation": {
    "kind": "process.exec",
    "program": "python",
    "args": ["-m", "pytest", "-q"],
    "cwd": {"resource": "repo://demo", "relative_path": "."}
  },
  "timeout_ms": 120000
}
```

`program` 是 Node 配置中的程序标识，由 Node 映射到可执行文件绝对路径；不是由 Server 下发任意物理路径。`args` 普通值直接作为 argv，不进行 shell 插值。需要跨资源路径时可用明确的 `{resource, relative_path}` 参数对象，由 Node 解析成单个 argv 项；禁止在任意字符串内隐式替换 URI。

fs 操作只接受资源 URI 和相对路径。Server 从 operation 推导实际引用的资源并验证全部在 requirements 中，防止“调度声明 A、实际使用 B”。Node 再独立验证。通过字符串参数刻意访问其他路径仍属于进程执行权限问题，不能由 URI 规则宣称阻止。

首版不接受调用方任意注入环境变量；使用 Node 配置中明确允许的基础环境，不继承 Node 连接 token。程序标识为 Python 也不代表只运行安全脚本。

### 4.2 资源位置与版本

逻辑 Resource 表达 URI、种类、描述；Location 表达 node_id、可用性、revision 证据、是否允许内容返回。物理 root 和程序路径只存在 Node 本地。

Node 注册发送完整资源快照及单调配置版本；断线或重连对账未完成时不能用于 placement。同一 URI 在不同 Node 上是两个独立 location，不能合并为“内容已同步”。

仓库需要固定版本时，使用完整 commit，并检查跟踪文件与未跟踪文件状态；首版复现示例排除子模块、LFS 和依赖缓存差异。数据集 revision 由所有者在本地清单维护，可使用清单摘要；不自动扫描、上传或计算整库内容哈希。结果标明证据来源，不能将用户标签当成已经验证的数据一致性。

未指定 revision 的调用允许访问当前内容，但结果必须附实际版本或 `unknown`，不宣称可复现。要求精确 revision 而缺少证据时拒绝。外部进程仍可在检查后改工作树；首版测试环境需避免并发编辑，强快照隔离后移。

### 4.3 结果

ExecutionView 包含 ID、状态、node_id、placement 原因、时间、审批提示、结果或错误。Result 包含 exit_code、简短机器生成摘要、observations、artifact_refs、输出截断标记、资源版本证据、mutation coverage。

基础 observation 只承诺客观信息：退出码、耗时、字节计数、限制是否触发。没有测试报告解析器就不能自动声称“3 个测试失败”。首版不额外调用 LLM 压缩日志。

进程操作的 `mutations.coverage` 默认 `unknown`；若增加 Git 前后状态比较则标 `best_effort`，不覆盖仓库外文件、修改后还原的文件及其他进程的变更。不得用空数组表示已证明没有副作用。

## 5. Placement 规则

1. 身份验证后确定 principal 可访问的 Node 和资源；请求内提供的所有者字段没有授权效力。
2. 用同一时刻的节点快照过滤：协议兼容、认证连接有效、已完成对账、在线、能力和 OS 匹配。
3. 过滤资源：一次 execution 引用的所有资源必须在同一个 Node 上可用，并符合所需版本。
4. 过滤 Server 已知的访问禁令和 Node 发布的静态能力限制；最终动态策略仍在 Node 判定。
5. 仅选择有空闲执行槽的候选；允许的候选中优先 preferred_node，然后按 node_id 字典序。
6. 在 Server 本地事务中再次检查并预留执行槽，写入派发意图；冲突则对当前合法候选重新选择，绝不双重预留。

若全部忙，立即返回 `NODE_BUSY`；无能力或资源组合则返回 `NO_MATCHING_NODE` 和可公开的排除原因；不维护等待队列、不自动复制资源。preferred_node 是软偏好；诊断时可使用单独的硬约束 node_id，不能绕过其他条件。

Heartbeat 只衡量连接是否及时响应，不作为性能评分。默认心跳 10 秒、30 秒未收到视为离线；数值集中配置并在故障测试调整。时间使用 Server 接收时间，不能用 Node 自报时间推断网络延迟。

Trace 保存请求约束、候选快照版本、排除原因、选中节点及规则版本；保存所用证据才能解释当时决定。

## 6. 执行生命周期和故障语义

### 6.1 状态

| 状态 | 含义 | 允许的主要后续状态 |
| --- | --- | --- |
| CREATED | 请求已持久化，尚未记录派发意图 | DISPATCHING、REJECTED、CANCELLED |
| DISPATCHING | 已占槽并记录派发意图；可能已到达 Node | WAITING_APPROVAL、RUNNING、已知终态、UNKNOWN |
| WAITING_APPROVAL | Node 已记录请求，尚未启动 | RUNNING、REJECTED、CANCELLED、EXPIRED、UNKNOWN |
| RUNNING | Node 已确认启动 | SUCCEEDED、FAILED、CANCELLED、TIMED_OUT、UNKNOWN |
| UNKNOWN | 无法确认是否启动、结束或仍运行 | 经 Node 持久证据恢复状态；不能凭心跳猜测终态 |
| 已知终态 | SUCCEEDED / FAILED / REJECTED / CANCELLED / TIMED_OUT / EXPIRED | 不再自动变化；冲突证据记事件供排查 |

`cancel_requested` 是独立事实，不是已经取消。`UNKNOWN` 不是终态，也不是允许自动重试的提示。协议消息可重复、晚到或省略中间通知，因此经可信终态证据可以跳过显示层中间状态。

Node 实际启动前检查请求是否取消/过期、策略是否仍允许、资源映射和版本是否仍匹配。审批等待不计入进程运行 timeout，有独立 TTL。终态代表已观察到的执行事实，不保证副作用已回滚。

### 6.2 持久化与重复请求

Server 使用唯一键 `(principal_id, request_id)`，保存规范化请求摘要。同键同请求返回已有 execution；同键不同请求返回 `IDEMPOTENCY_CONFLICT`。状态记录清理后仍在配置的幂等保留窗口内保留 tombstone；首版默认窗口 30 天，不承诺窗口以外或人为删除数据库后防重。

Node 在启动进程前持久化 execution_id、请求摘要与已接收状态；启动前写“启动已承诺”标记，崩溃后遇到此标记不能盲目再启动。即使写标记后实际上还没启动，也宁可报未知。数据库提交和 OS 进程启动无法靠 SQLite 组成一个原子事务。

同一 Node 状态目录只允许一个 daemon 持有 OS 级独占锁，Server 状态目录也只允许一个 Server；第二个进程明确启动失败。本地审批 CLI 可以通过短事务访问 Node store，但不能成为第二个执行 daemon。不能只依赖“用户一般不会启动两次”。

同一 execution 的重复消息仅返回已有记录，不再次启动。已知结果保存并重复发送，直到 Server 在状态与事件事务提交后返回 receipt。Node 结果及防重记录至少保留约定窗口；若重装丢失状态，应撤销旧身份并重新登记，不能冒充保留了旧执行历史。

### 6.3 故障处理矩阵

| 场景 | 必须行为 |
| --- | --- |
| 提交响应丢失 | 调用方带原 request_id 重试，取回同一 execution |
| 派发或启动确认丢失 | 标 UNKNOWN，重连 query/reconcile；不另建 execution 自动重跑 |
| Node 连接中断但 daemon 存活 | 本地进程可继续至自身 deadline；结果本地保存；Server 不释放未知槽 |
| Server 重启 | 节点均待重新认证与对账；恢复持久记录，可能派发的请求先核实 |
| Node 重启 | 依据本地 journal 上报；对“可能启动过”的作业不重跑；残留进程未核实前隔离节点 |
| 取消时 Node 在线 | 发显式 cancel，Node 终止作业并回报；仅确认停止后标 CANCELLED |
| 取消时 Node 离线 | 持久化 cancel_requested；保持 UNKNOWN；重连先处理取消再开放执行槽 |
| 完成与取消竞争 | Node 串行确定结果；若已完成则报告真实完成，不强改为 CANCELLED |
| 磁盘满或 journal 写入失败 | 新操作 fail closed，不启动；运行中的日志受限处理，无法持久化结果则保持不确定 |
| 永久失联 | 保持结果未知；操作者确认残留进程并撤销旧 Node 后才使用新身份接入；不自动补偿 |

每个 Node 的一槽限制也覆盖审批等待和未解决的未知 execution。通过牺牲部分可用性避免首版实现队列、工作窃取和复杂租约恢复。

### 6.4 进程生命周期

Linux 使用独立进程组，取消/超时先发送终止信号，宽限期后强制结束并回收管道、进程和句柄。`context` 用于传递取消，但不能只依赖 `CommandContext` 默认行为。[Go os/exec](https://pkg.go.dev/os/exec)

首版只支持受管理的前台作业，不支持主动 setsid、daemonize 或逃离进程组的代码。Node 崩溃不保证所有后代进程已经退出；启动恢复应检查本地记录，并在无法确认清理时保持不可调度，不能只凭 PID 存在/不存在判断，PID 可能复用。

本地 runtime timeout 建议默认 120 秒、上限 1 小时；Node 可设置更低上限。审批 TTL 建议 10 分钟。排队功能不存在。断线不会关闭本地 timeout。

## 7. Node 协议和身份

消息 envelope 至少含 protocol_version、type、message_id、connection_epoch、execution_id（执行相关消息）、payload。未知操作和缺失必填字段应拒绝；可选字段允许忽略。协议版本与软件版本独立，首版只承诺同一 protocol major 的显式兼容范围，不宣称任意版本互通。

最小消息集为 register/registered、heartbeat、execution.dispatch、accepted/approval_required/started/result、execution.cancel、execution.query、execution.snapshot、result.receipt，以及按授权获取 artifact 片段的 request/response。

每次认证连接由 Server 产生新 epoch；旧连接不能更新新会话状态。结果补报通过新会话发送，但仍使用原 execution_id 和请求摘要。Node 仅允许一个有效 Server 控制会话；同一身份的旧连接被替换后不接受新派发。

控制消息有大小上限与超时；建议单帧上限 1 MiB，产物分块不超过 64 KiB。每条连接一个受控写入循环，避免并发写破坏传输；心跳和取消不得被大量输出阻塞。首版不持续推送原始 stdout，按需拉取降低复杂度。

首版由管理员预先创建独立 Node token 并绑定 node_id；无需先实现短期 enrollment token 兑换协议。Server 仅保存高熵 token 的安全摘要，Node 在受限权限文件保存原 token；支持列出身份和撤销。调用方使用独立 principal token，Node token 不能访问管理 API。

派发携带由 Server 认证上下文生成的 principal_id，Node 可据此进一步限制操作；忽略或拒绝 MCP 参数中试图伪造 principal 的字段。首版 Node 信任已配置 Server 对调用方身份的证明，但执行仍需满足本地策略。

非 loopback 通信强制 TLS 并验证服务端证书；默认拒绝跳过证书校验。不需要 mTLS/SSO，但不能省略身份绑定、凭据撤销和日志脱敏。

## 8. 授权、审批与信息返回

### 8.1 默认策略

| 操作 | 默认 | 说明 |
| --- | --- | --- |
| 发现 Node/Resource 元信息 | 已认证且已授权后 ALLOW | 只发布手工配置的逻辑标识及必要元数据 |
| fs.list / fs.read | 未配置资源 DENY；显式允许返回内容的资源可 ALLOW | roots 仅控制文件 API；读结果离开 Node 也是授权的一部分 |
| process.exec | ASK | 本地所有者可为可信项目显式 ALLOW；不得内置“pytest 天然安全”规则 |
| 未识别程序、能力或资源 | DENY | 不按未知字段兜底执行 |
| 原始日志与 artifact 内容获取 | 未显式授权则 DENY | 可只允许元数据；每次读取复核当前策略 |

Node 以普通专用账号运行；任意受批代码仍可使用该账号能访问的文件和网络。首版无网络出口隔离或完整秘密检测；不承诺日志脱敏能防止恶意外传。授权进程执行就是授权它在此权限范围内运行。

fs API 使用 `os.Root` 等适合目标平台的目录句柄访问方式，并测试 traversal、symlink 与竞态。使用受支持、已修补工具链；不能把“采用某 API”写成永远安全的保证。[Go 目录访问边界](https://go.dev/blog/osroot)

### 8.2 本地审批

Node 将请求规范化，记录摘要、程序、argv、cwd、资源映射版本、策略版本和 TTL。`runweave node approvals` 显示本机等待项；`runweave node approve <execution-id>` 展示完整拟执行操作后由本地操作者批准；deny 命令拒绝。

首版无需审批 Web 服务：本地 CLI 通过 Node store 的事务方法写决策，daemon 在启动检查中读取并原子消费决策。SQLite 文件限该 OS 用户访问；CLI 不直接启动作业。只有 daemon 是启动进程的执行方。

DENY 不能通过 approve 绕过。参数、Node、资源映射或策略改变使审批失效；审批不是对未来任意命令的凭证。同一 execution 不因重复 approve 再次启动。第三方 MCP 工具中不提供批准接口。

CLI 与 daemon 处于同一 OS 账号信任边界；这套机制防止远程调用方自行放行，不抵御恶意本地同权限进程。需要该级隔离时必须另做权限分离。

### 8.3 输出和产物

默认只自动返回状态、退出码、耗时等小结果；stdout/stderr 本地落盘。资源和 Node 配置可允许有界内容返回。`fs.read` 的用途就是返回文件内容，因此不能同时把它描述成“源码永不离开节点”。

建议首版限额：单次读取/结果正文至多 64 KiB；单 execution stdout+stderr 合计 10 MiB；Node 产物总预算默认 1 GiB。日志达到上限后继续排空并丢弃超额字节，记录 truncated 和计数；不能停止读管道导致子进程阻塞。

Artifact ID 不含任意物理路径。元数据含所属 execution、Node、媒体类型、大小、过期时间与截断情况；读取以 ID、offset、limit 为参数，Node 根据其内部索引读取且复核授权。完整日志可在 Node 本地查看，不需要上传对象存储。

已完成日志默认留存 7 天；清理不删除运行中的文件。空间不足先清理可回收旧产物，仍不足则拒绝新执行。未确认结果的结构化 journal 独立保留，不能被日志清理顺带删除。Node 离线时返回 `ARTIFACT_UNAVAILABLE`，过期返回 `ARTIFACT_EXPIRED`。

## 9. SQLite 与可观测性

Server 首版表：nodes、resource_locations、executions、events、principals/token_records、schema_migrations。Node 首版表：received_executions（含状态、摘要与结果）、approvals、artifact_index、schema_migrations。不建 Agent、TaskGraph、InteractionNode 表。

Server execution 更新、执行槽预留/释放和事件插入在同一事务内。Node 接收防重、审批消费和启动意图也需要明确事务。SQL 迁移文件进版本管理，外键显式启用，事务内不进行网络或进程等待。

数据库放本机磁盘，采用 WAL 与合理 busy_timeout；每个进程限制数据库写入并发，Node CLI 与 daemon 的写竞争用短事务处理。不把数据库放共享网络盘，不在首版启动多个 Server 写同一运行状态。WAL 仍只有一个 writer，不代表可无限并行写入。[SQLite WAL 文档](https://www.sqlite.org/wal.html)

持久 events 是排障事实记录，包含 execution_id、事件类型、序号、Server 接收时间及必要详情；不是通过回放重建所有状态的事件溯源系统。终态结果与事件只在事务提交后确认给 Node。首版查询数据库即可，无需内存 EventBus。

提供执行列表、单次详情和 placement 原因查询。日志以 execution_id/node_id/request_id 关联；不记录 token、完整文件内容和完整环境。健康检查区分存活与可接单，例如数据库不可写时不能显示 ready。

备份先采用有文档的停机一致性备份；不能运行中只复制主数据库文件而忽略 WAL。先在临时状态目录验证迁移和恢复；旧 schema 无法安全升级时清楚报错，不自动破坏数据。

## 10. MCP 与 CLI 最小交互

| MCP 工具 | 语义 |
| --- | --- |
| runweave.list_nodes | 返回授权范围内的在线/可调度状态和能力 |
| runweave.list_resources | 返回资源 URI、位置及版本证据，不返回物理路径 |
| runweave.execute | 校验并持久化明确操作，快速返回 execution_id 和当前状态 |
| runweave.get_execution | 查询状态、审批位置、结果和建议轮询间隔 |
| runweave.cancel_execution | 记录取消意图，返回是否已确认停止 |
| runweave.read_artifact | 按 Node 输出策略读取小片段，结果可能被拒、离线或过期 |

execute 不等待远程进程结束，若同一幂等请求已经结束则可返回已保存终态。目标是正常负载下在 2 秒内完成提交确认，P1 实测后校准，不将此数字当成已经达到的性能指标。

MCP 工具使用 `inputSchema`、`outputSchema` 和 `structuredContent`，并给兼容客户端提供精简文本表示。已接收后作业失败属于 ExecutionResult；参数/授权等调用错误按 SDK/工具错误规范映射。hint 与描述均不是安全决策依据。[MCP Tools 规范](https://modelcontextprotocol.io/specification/2025-11-25/server/tools)

首版不依赖 Tasks 扩展或某个客户端的长连接行为；以后可以把扩展映射到同一 execution ID，不重写引擎。[MCP Tasks](https://modelcontextprotocol.io/extensions/tasks/overview)

CLI 至少覆盖 server、mcp、node connect/list、node-token create/revoke、resources list、exec、execution list/get/cancel、node approvals/approve/deny、artifact read。exec 与 MCP 走同一 Server API，避免“只能靠 LLM 才能排障”。凭据通过受限配置文件或环境传入，不打印到常规日志。

## 11. 开发顺序与估算

以下是单名熟悉 Go 的开发者的初步工程估算，不是承诺日期；假设已有两台 Linux 环境、一个可用 MCP 客户端，不含额外企业安全功能。以验收门槛推进，不按模块写完数量推进。

| 阶段 | 交付与顺序 | 出口条件 | 粗估有效人日 |
| --- | --- | --- | --- |
| P0 契约与技术验证 | 冻结本文关键语义；锁 Go/SDK/驱动/WebSocket；小规模客户端和进程验证 | 真客户端连接官方 SDK 示例；Linux 子进程回收验证；依赖构建成功 | 3–5 |
| P1 第一条纵向链路 | Server、单 Node、基础身份、手配资源、fs.list、持久提交/结果、MCP/CLI | 外部客户端发现 Node 并读到授权目录列表；重复提交同 ID | 5–7 |
| P2 受控进程执行 | process.exec、Node journal、本地 ASK、超时取消、有限输出 | 未批准不启动；终止测试进程及子进程；超量日志不阻塞 | 7–10 |
| P3 两节点 placement | 完整资源/能力过滤、版本复核、槽位与原因记录 | 自动选择正确 Node；无共址、版本错、节点忙均明确失败 | 4–6 |
| P4 故障闭环 | 确认丢失、重连对账、Server/Node 重启、取消竞争 | 故障矩阵通过；未知不重跑；旧会话不能更新状态 | 7–10 |
| P5 输出与发布 | fs.read、artifact 分页和授权、留存、诊断、文档与打包 | 干净环境安装；完整两节点 Demo；真实客户端轮询/取消 | 5–8 |

合计约 31–46 有效人日；按每周 5 个有效开发日约 6–9 周，另留 25%–35% 联调与未知问题缓冲，日历安排宜按约 8–13 周考虑。兼职应按实际有效时间换算；P0 完成后重新估算。AI 辅助不替代真实节点、断线和客户端联调。

P1 起就保留幂等键和派发状态，P2 起就做授权与取消，P4 扩展故障覆盖；不能先写一个允许任意执行的通道，再计划发布前补安全。若超期优先裁掉日志便利能力和平台覆盖，不裁幂等、未知状态、超时或 Node 授权。

## 12. 首版验收清单

所有条目是未来测试要求，此次没有实现或运行它们。普通单元测试不调用真实 LLM；客户端端到端验证独立记录。

| 编号 | 场景 | 通过条件 |
| --- | --- | --- |
| A01 | 干净 Linux 环境启动 | 使用发布二进制和示例配置启动 Server/Node；记录实际步骤与依赖 |
| A02 | 真实 MCP 客户端 | 记录客户端、SDK、协议版本；发现、提交、查询、等待审批与取消均走通 |
| A03 | 两节点选择 | A 无目标资源，B 有；无需指定 B 就能选中 B，trace 能解释 |
| A04 | 硬约束负例 | 能力缺失、资源不能共址、revision 不符时不执行、不降级 |
| A05 | 节点占槽 | 两个并发请求不能同时占一个槽；等待审批/UNKNOWN 也不超发 |
| A06 | 幂等冲突 | 相同 request_id 同内容返回原记录，不同内容报冲突 |
| A07 | 重复派发 | 计数器副作用在重复消息/丢确认情况下只启动一次；模糊崩溃窗口报 UNKNOWN |
| A08 | Server/Node 重启 | 不凭内存空闲重跑；未核实的残留进程使 Node 不可调度 |
| A09 | 取消/超时 | 测试进程及受管理子进程退出、管道回收；完成竞争保留真实结果 |
| A10 | 文件访问边界 | `..`、绝对路径、外部符号链接、符号链接替换竞态不越界 |
| A11 | 审批绑定 | 参数变化、过期、策略变化、错误 Node 不会使用旧审批；DENY 不可绕过 |
| A12 | 身份与旧会话 | 错 token、撤销 token、伪造 node_id、旧 epoch 被拒；Node token 不能当 principal |
| A13 | 输出限制 | 大量 stdout 不耗尽内存或死锁；禁止返回的内容不经读取接口泄露 |
| A14 | Artifact 生命周期 | 分页、未授权、离线、过期、磁盘限额分别返回明确结果 |
| A15 | 脱离调用方会话 | 客户端关闭后已提交执行仍有记录；重开用 ID/CLI 可查询 |
| A16 | 存储一致性 | 状态和事件原子提交；磁盘不可写不启动新作业；备份恢复与迁移可验证 |

按变化选择测试：placement 用固定快照表驱动测试，协议用 JSON fixture，生命周期用真实子进程，幂等用故障注入与持久存储重启。执行 `go test ./...`、目标环境可运行的 race 检查、`go vet`、依赖漏洞检查与发布构建；不追求无意义的覆盖率数字。

GPU 并非基础验收必需；可以先用两节点不同的真实能力与资源验证路由。若发布说明声称支持 CUDA 训练，则必须另以真实 GPU 和最小任务验证，不能用声明了 `cuda: true` 的 mock 充当证明。

## 13. 首版 Demo 和后续演进

Demo 在同一个外部 Harness 任务中先读取 A 上的样例仓库信息，再到拥有数据样例的 B 上运行已准备好的统计脚本，最后由 Harness 综合结果。两个 execution 共用 correlation_id；Node 都展示实际资源版本和路由原因。

若 Demo 需要同一段代码在 A/B 运行，操作者提前准备相同 revision 的干净工作树。A 上临时修改的内容不会自动出现在 B；版本不符必须被拒绝。这验证跨节点执行，不虚构分布式工作区同步。

v0.0.1 之后先根据试用反馈改善安装、错误提示、日志和客户端适配。只有出现实际需求再增加自有 Agent、Web 审批、Windows Node、任务图或 artifact 上传；每项都复用现有执行服务和协议边界。

允许重构内部 Go 包，不承诺首版内部 API 永久稳定；对外协议及磁盘 schema 的变化必须有版本、迁移或明确的不兼容提示。可维护性来自清楚的责任和验证，扩展性来自稳定的语义边界，不要求第一版具备所有扩展实现。

本次只交付审计和方案。后续开工的第一项是 P0，再进入 P1 的 MCP 单节点纵向链路，而不是先搭建完整 Agent 框架。

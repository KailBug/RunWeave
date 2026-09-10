# RunWeave 3 个月开发规划书

| 项目 | 约定 |
| --- | --- |
| 文档职责 | 未来三个月 MCP 执行后端的阶段目标、工作包、实施约束和总验收 |
| 状态 / 更新日期 | 现行规划 / 2026-09-10；P0 已启动，完整阶段尚未通过 |
| 计划窗口 | 2026-09-10 至 2026-12-09，共 13 个工作周；日期是目标，不是发布承诺 |
| 软件目标 | v0.0.1；Go、MCP 优先、Linux Server/Node、单体控制平面 |
| 开发模式 | 1 人为主，AI 辅助；本计划不要求多 Agent 并行 |
| 维护责任 | 项目维护者；阶段复盘或范围/资源变化时重估 |

长期定位与范围依据见 [项目规划书](RunWeave项目规划书.md)。本文件回答“未来三个月交付什么、按什么顺序、怎样判定完成”；日常实际进度只在 [开发进度](docs/进度/开发进度.md) 维护，技术证据保存在 `docs/验证/`。

## 1. 目标与起点

三个月目标是让一个真实外部 MCP 客户端发现两个用户管理的 Linux Node，根据能力与资源需求提交操作，在 Node 授权后执行，并能查询、取消、处理超时与断线恢复、读取允许返回的结果。

当前已有 Go module 与独立 MCP stdio 探针。Windows、Ubuntu WSL 和 Codex 探针调用的历史通过情况见 [P0 验证记录](docs/验证/2026-09-10-P0-MCP探针验证.md)。这不等于 P0 完成，更不等于 Server–Node 业务链路成立。SQLite/WebSocket/YAML、Linux 受管理进程与业务契约仍待验证或冻结。

第一个可交付闭环是：外部客户端 → MCP bridge → Server → 单个 Linux Node 的授权 `fs.list` → 持久 execution 查询；重复提交返回同 ID，错误请求被拒。完整首版再扩展进程、双节点、故障和产物。

优先使用 Codex 完整验收；nanobot、loopx 随后按具体项目和版本验证，额外客户端不阻塞首版。自有 Harness 在 MCP 业务功能打通后另立开发目标，不进入本计划排期。

## 2. 交付范围与接口入口

正式交付一个 `runweave` 二进制，以子命令承担 Server、Node、CLI 和 MCP stdio bridge。当前 `mcp-probe` 是 P0 验证程序，不是正式产品入口。

| MCP 工具目标 | 语义 |
| --- | --- |
| `runweave.list_nodes` | 返回授权范围内的节点、能力、在线/可调度状态 |
| `runweave.list_resources` | 返回逻辑资源、位置和版本证据，不返回 Node 物理路径 |
| `runweave.execute` | 校验并持久提交一次明确操作，快速返回 execution_id |
| `runweave.get_execution` | 查询状态、审批提示、结果和轮询建议 |
| `runweave.cancel_execution` | 保存取消意图，明确是否已经确认停止 |
| `runweave.read_artifact` | 根据当前授权读取有界产物片段 |

业务操作为 `fs.list`、`fs.read`、`process.exec`。工具名称与 schema 在 P0 结合真实客户端冻结；表中是规划契约，未表示已经可调用。使用官方 SDK 的 inputSchema/outputSchema、structuredContent 与精简文本兼容输出，不自写 JSON-RPC，不依赖 MCP Tasks 扩展。

CLI 覆盖 server、mcp、node connect/list、node-token create/revoke、resources list、exec、execution list/get/cancel、node approvals/approve/deny、artifact read，以及调用方凭据的必要管理。具体 CLI 语法在实现对应能力时写入参考与指南。

execute 不等待长任务完成；正常负载下提交确认以 2 秒内为初始体验目标，P1 实测校准。取消 MCP 请求或关闭 bridge 不自动停止已提交 execution，需显式取消。

## 3. 时间假设与周计划

沿用原估算：P0 3–5、P1 5–7、P2 7–10、P3 4–6、P4 7–10、P5 5–8 个有效人日，共 31–46 人日；增加约 25%–35% 联调缓冲后按 8–13 周评估。假设熟悉 Go、每周可投入约 5 个有效开发日，且可取得两个真实 Linux Node 与一个可用客户端。现有一个 WSL 环境不能满足双节点验收前提；需在 W07 前准备第二个真实节点。

已有探针工作不重新计为未开始；也不能从局部通过直接推算剩余工期。P0 完成、月底及重大阻碍出现时重估。下面按连续七天划分 W01–W13，每周有效投入以实际为准。

| 月份 / 周 | 日期 | 重点与阶段结果 |
| --- | --- | --- |
| 第 1 月 W01 | 09-10～09-16 | P0：依赖/真实客户端/进程验证，冻结请求、状态与协议 |
| W02 | 09-17～09-23 | P1：入口、配置、SQLite、身份、独占锁、Node 注册/心跳 |
| W03 | 09-24～09-30 | P1：持久提交、fs.list、MCP/CLI 查询、幂等与结果确认 |
| W04 | 10-01～10-07 | P1 负例收口；P2 程序映射、journal、审批起步 |
| 第 2 月 W05 | 10-08～10-14 | P2：审批绑定、启动前复核与真实进程 |
| W06 | 10-15～10-21 | P2：进程组取消/超时/回收、输出限额和竞争验证 |
| W07 | 10-22～10-28 | P3：双节点能力/资源/版本过滤、事务占槽与路由解释 |
| W08 | 10-29～11-04 | P3 收口；P4 丢确认、重复派发、UNKNOWN 与结果补报 |
| 第 3 月 W09 | 11-05～11-11 | P4：Server 重启、重连对账、旧 epoch、状态/事件原子性 |
| W10 | 11-12～11-18 | P4：Node 崩溃、残留进程、离线取消、磁盘故障 |
| W11 | 11-19～11-25 | P5：fs.read、artifact 授权分页、过期与配额 |
| W12 | 11-26～12-02 | P5：干净安装、迁移/备份恢复、完整客户端和 A01–A16 |
| W13 | 12-03～12-09 | 缺陷修复、受影响回归、候选发布检查与缓冲 |

月底出口：第 1 月单节点真实 MCP 目录列表；第 2 月受控进程与双节点路由；第 3 月全部首版验收及可维护发布。进入下一周或月份不代表前一阶段完成。未达标则保留候选状态、重排任务，不以日期强行发布。

## 4. 工作包与阶段门槛

保留原工作项编号，便于关联历史和后续开发记录。阶段之间遵循 P0 → P1 → P2 → P3 → P4 → P5；已有阶段中就实现基础故障语义，P4 扩大覆盖。

### P0：技术验证与契约

| 编号 | 工作内容 | 出口条件 |
| --- | --- | --- |
| P0-01 | 环境、Go/SDK/SQLite/WebSocket/YAML 组合；真实客户端与 Linux 进程实验 | 版本锁定；依赖构建运行、事务迁移、帧限制/断线、子进程回收有记录 |
| P0-02 | Request/Result、三种 operation、错误码、字段/内容上限、资源引用与摘要规范化 | 合法/非法 JSON fixtures 明确；调用错误与执行失败、是否产生 ID、幂等行为无歧义 |
| P0-03 | 状态转换、槽位、cancel_requested、epoch、消息与 receipt/query/snapshot | 重复/迟到/省略中间消息、模糊启动窗口、取消竞争都有明确证据要求 |

### P1：第一条单节点链路

| 编号 | 工作内容 | 出口条件 |
| --- | --- | --- |
| P1-01 | 程序入口、版本化 YAML、凭据与日志 | 错配置明确失败；MCP stdout 不混日志 |
| P1-02 | Server/Node SQL 迁移、短事务、状态目录 OS 独占锁 | 重开库保留事实；第二 daemon 失败；不可写时拒绝启动 |
| P1-03 | 独立 principal/Node 身份、摘要存储、创建/撤销、非 loopback TLS | 错 token、角色混用、伪造节点被拒，撤销生效 |
| P1-04 | Node 完整资源快照、配置版本、epoch、心跳与单写循环 | 未认证/未对账/断线节点不可调度；旧会话无效 |
| P1-05 | 统一 Submit/Get/Cancel、幂等、合法候选、事务占槽与派发意图 | 同键同内容同 ID，冲突明确；并发不超发 |
| P1-06 | Node 防重记录、策略复核、目录句柄边界、非递归 fs.list | 正常目录有界返回；越界/未授权拒绝；重复派发不重执行 |
| P1-07 | 结果持久化、补报、状态/事件事务和 receipt | 丢确认可恢复；终态不会被重复结果改变 |
| P1-08 | HTTP、CLI 和 MCP 发现/提交/查询/取消 | 真实客户端读到目录；CLI 查到同一记录；会话关闭后记录仍在 |

P1 必须已有 UNKNOWN 占槽、基础对账和持久取消意图；未确认清理的状态保持隔离。文件操作也响应取消和超时，但不作为进程组验收。目录条目数与编码总字节均设上限，超额明确截断。P1 通过只是 A02 等项目的部分场景。

### P2：受控进程执行

| 编号 | 工作内容 | 出口条件 |
| --- | --- | --- |
| P2-01 | 本地程序标识、argv/cwd/资源参数、基础环境 | 无隐式 shell 插值，Node token 不进入子进程 |
| P2-02 | ALLOW/ASK/DENY、本地审批 CLI | process.exec 默认 ASK；未批准不启动；DENY 不可绕过 |
| P2-03 | 审批绑定、TTL、启动前复核 | 参数/节点/映射/策略变化、过期和重复批准不会误启动 |
| P2-04 | 接收/启动承诺/结果 journal、审批原子消费 | 模糊崩溃不重跑；等待审批仍占槽 |
| P2-05 | Linux 进程组、deadline、TERM/KILL 宽限及回收 | 进程与受管理子进程退出，管道回收，完成/取消竞争保留事实 |
| P2-06 | stdout/stderr 本地输出、字节计数、配额和排空 | 超量输出不死锁、不耗尽内存，返回仅含允许信息 |

以上六项共同构成首次进程能力的开放门槛。不能先暴露任意执行入口，再补审批和超时。

### P3：双节点路由

| 编号 | 工作内容 | 出口条件 |
| --- | --- | --- |
| P3-01 | 身份/会话/OS/能力/资源共址/版本/槽位过滤 | 任一硬条件不满足就不执行，不拆分单次操作 |
| P3-02 | repo commit/clean、dataset 证据来源与启动前复核 | 版本不符拒绝；人工标签不冒充内容已验证 |
| P3-03 | 合法候选偏好排序、事务复查与占槽 | 并发不双占；NODE_BUSY 与 NO_MATCHING_NODE 区分 |
| P3-04 | 约束、候选快照、排除原因与规则版本 trace | 自动选中合法 B，能解释为何未选 A，且不泄露未授权信息 |

### P4：故障闭环

| 编号 | 场景与工作 | 出口条件 |
| --- | --- | --- |
| P4-01 | 提交/派发/启动确认丢失 | 原键查询与对账，不建替代执行、不换节点重跑 |
| P4-02 | 结果或 receipt 丢失、重复补报 | 结果可恢复，Server 提交前不确认，终态冲突留事件 |
| P4-03 | Server 重启 | 恢复持久状态，Node 重新认证对账，不把内存空槽当空闲 |
| P4-04 | Node 重启、启动模糊窗口、残留进程 | 证据不足 UNKNOWN，节点隔离，不凭 PID 存在性猜测 |
| P4-05 | 离线取消、完成与取消竞争 | 重连优先处理取消/对账；未确认不报 CANCELLED |
| P4-06 | 旧 epoch、撤销、磁盘/journal/事件失败、重复 daemon | 旧会话无效，不能保存启动事实时不启动 |

使用真实临时存储、子进程和可控丢消息/断连接。副作用计数器检验重复启动；无法证明启动事实的窗口诚实报告 UNKNOWN。

### P5：内容、维护与发布

| 编号 | 工作内容 | 出口条件 |
| --- | --- | --- |
| P5-01 | fs.read、编码/二进制表示、offset/limit、当前内容授权 | 读取有界，越界和未授权拒绝 |
| P5-02 | 不透明 artifact ID、本地索引、分块和逐次授权 | 不接受任意物理路径；离线/过期/未授权分别报错 |
| P5-03 | 日志留存、Node 预算、防重记录/tombstone | 不清理运行文件或未确认 journal；空间不足拒绝新执行 |
| P5-04 | 列表/详情/trace、健康与 ready、配置、迁移和备份恢复 | 能诊断失败与未知；恢复步骤实际验证 |
| P5-05 | 发布构建、干净安装、完整 Demo、客户端及全套验收 | A01–A16 有证据，必要文档齐全，满足发布门槛 |

## 5. 实施约束基线

本节合并原实施方案的必要约束，供 P0 冻结和后续开发落实；它不是当前已实现的 API 文档。各主题进入开发时，在 `docs/设计/`、`docs/参考/` 建立具体定义并引用本节；配置默认值的实测调整记录理由，行为或范围变化需同步修订本规划。

### 5.1 技术与职责

Go 标准库承担 HTTP、context、slog、进程基础能力；MCP 使用官方 Go SDK；SQLite 采用 database/sql 和一个驱动，优先验证纯 Go `modernc.org/sqlite`；WebSocket 和 YAML 各选择一款维护中的库。精确版本以 go.mod/go.sum 与验证记录为准，不把候选写成已锁定。

按实际实现引入 `cmd/runweave`、`internal/contract`、`internal/protocol`、`internal/server`、`internal/node`、`internal/store`、`internal/api`、`internal/mcp`、`tests/integration`。contract 不依赖网络/数据库/MCP；protocol 只表达消息；Server 管执行服务与 placement；Node 管本地资源/策略/进程；store 管显式 SQL；接入层不访问数据库或直接派发 Node。小接口放使用方，不引入通用 Manager、Repository 基类、DI、CQRS 或插件框架。

### 5.2 请求与资源

请求包含 request_id、可选 correlation_id、requirements、operation、timeout_ms。P0 冻结 schema：未知业务请求字段拒绝，默认值补齐，集合规范化，argv 顺序和字符串原值保留，correlation_id 纳入摘要。区分业务请求字段与协议中允许忽略的可选扩展字段。

文件操作使用资源 URI 与相对路径；首条 fs.list 拟使用 `target={resource, relative_path}`，字段仍待 P0 定稿。进程 program 是 Node 配置标识，映射到本地绝对可执行路径；普通 args 作为 argv，不经过隐式 shell；跨资源参数用显式路径对象，不在字符串中替换 URI。Server 推导实际引用资源并检查全部声明在 requirements，Node 再复核；调用方不能任意注入环境变量，连接 token 不传给子进程。

ResourceLocation 分别保存 node_id、可用性、revision 来源及允许返回内容的策略，物理 root 仅在 Node。注册发送完整快照和单调配置版本。repo 固定版本检查完整 commit、跟踪和未跟踪文件；dataset 由所有者维护版本清单或摘要，不自动扫描上传整库。要求精确版本而缺证据时拒绝；未指定版本可访问当前内容，但结果记录实际证据或 unknown。首版 Demo 排除子模块、LFS、依赖缓存差异和并发外部编辑，不承诺快照隔离。

### 5.3 Placement

依次过滤授权范围、协议/认证/对账/在线、OS/能力、资源共址与版本、已知静态禁令和槽位；最终动态授权仍由 Node 决定。合法候选先 preferred_node，再 node_id 字典序；诊断性硬约束 node_id 不得绕过其他条件。事务中复查并占槽、写派发意图，派发前的竞争可重新选候选；已经可能派发的执行不能另选节点重跑。全部合法候选忙报 NODE_BUSY，无合法组合报 NO_MATCHING_NODE。

Heartbeat 仅作存活判断，用 Server 接收时间，不作延迟或性能排名。trace 保存当时约束、快照版本、排除原因、选中节点与规则版本。

### 5.4 状态、防重与恢复

| 状态 | 主要后续状态与证据要求 |
| --- | --- |
| CREATED：已持久化、未记派发意图 | DISPATCHING、REJECTED、CANCELLED |
| DISPATCHING：已占槽、可能到达 Node | WAITING_APPROVAL、RUNNING、可信已知终态、UNKNOWN |
| WAITING_APPROVAL：已接收、未启动 | RUNNING、REJECTED、CANCELLED、EXPIRED、UNKNOWN |
| RUNNING：已确认启动 | SUCCEEDED、FAILED、CANCELLED、TIMED_OUT、UNKNOWN |
| UNKNOWN：执行事实未确定 | 依 Node 持久证据恢复，不能凭心跳猜终态 |
| 已知终态 | 不自动变化；冲突证据记事件供排查 |

可信终态证据可跳过丢失的中间通知。cancel_requested 是独立事实；终态不保证副作用回滚。运行 timeout 与审批 TTL 分开，启动前再检查取消、过期、策略、映射和版本。

Server 唯一键为 `(principal_id, request_id)`：同键同摘要返回原 execution，异摘要报 IDEMPOTENCY_CONFLICT。Node 启动前持久化接收和“启动已承诺”；崩溃后即使可能尚未实际启动，也不得盲目重跑。防重清理需保留窗口内 tombstone；人工删库或窗口外不承诺防重。每状态目录单 daemon OS 独占锁，本地审批 CLI 只用短事务写决策，不能启动第二 daemon。

Node 断线而 daemon 存活时本地 deadline 继续，结果本地保存补报；Server 仅在状态与事件提交后 receipt。审批等待与未解决 UNKNOWN 都占槽。离线取消持久保存并保持结果未确认；重连先取消/对账，再开放槽。永久失联保持未知；重装丢失状态必须撤销旧身份，以新身份接入，残留进程由操作者核实，不自动补偿。

### 5.5 协议、身份、审批与进程

Node envelope 包含 protocol_version、type、message_id、connection_epoch、执行相关 execution_id 和 payload。覆盖 register/registered、heartbeat、execution.dispatch、accepted/approval_required/started/result、execution.cancel/query/snapshot、result.receipt 和 artifact request/response。软件/MCP/Node 协议/schema 分别版本化；未知操作和缺必填拒绝；Node 协议只承诺明确声明的兼容范围。

每次认证连接由 Server 生成 epoch；Node 只接受一个有效控制会话，旧连接不能更新状态或接受新派发，旧结果经新会话带原 ID/摘要补报。每连接受控单写循环，控制消息有时限和帧限制，心跳/取消不能被产物阻塞，不持续推送原始 stdout。

管理员预创建并绑定 Node token；principal token 独立，高熵 token 在 Server 存摘要，原凭据限权保存，支持撤销。非 loopback 强制 TLS 验证，不默认跳过证书验证。principal_id 来自认证上下文，不信任调用参数自报身份。

未配置资源/程序/未知能力 DENY；文件内容仅显式允许才返回；process.exec 默认 ASK，可对可信项目显式 ALLOW。审批绑定 execution、Node、规范化请求摘要、程序/argv/cwd、资源映射/策略版本和 TTL；本地展示完整操作，daemon 原子消费并复核，DENY 不可 approve 绕过，MCP 不提供批准工具。同 OS 账号的恶意本地进程不在该审批隔离承诺内。

文件 API 使用适合平台的目录句柄边界（如 os.Root），测试路径穿越、外部符号链接及替换竞态，不能只做字符串路径前缀检查。Node 以普通专用账号运行；授权代码仍能访问其账号有权使用的文件和网络。Linux 使用独立进程组，取消/超时先终止、宽限后强杀并回收；不能只依赖 CommandContext 默认行为。首版仅受管理前台作业，不支持 setsid/daemonize 逃离；崩溃后残留未核实则隔离，不凭可复用 PID 判定清理。

### 5.6 结果、存储与默认限制

ExecutionView 包括 ID、状态、Node、placement 原因、时间、审批提示与结果/错误。Result 返回退出码、耗时、字节计数、截断、资源版本、artifact_refs 等客观信息，默认不自动返回原始日志，不调用 LLM 压缩。mutations.coverage 默认 unknown；可选 Git 比较仅 best_effort，不覆盖仓库外、副作用还原或其他进程行为。

Artifact 使用不透明 ID 与本地索引，元数据含归属、Node、媒体类型、大小、到期时间、截断；按 ID/offset/limit 逐次授权。Node 离线 ARTIFACT_UNAVAILABLE，过期 ARTIFACT_EXPIRED。达到日志上限继续排空并丢弃超额字节；先清理可回收旧产物，仍空间不足拒绝新执行。运行中日志和未确认 journal 不被留存清理误删。

Server 持久化 nodes、resource_locations、executions、events、principals/token_records、schema_migrations；Node 持久化 received_executions、approvals、artifact_index、schema_migrations。表名为设计基线，迁移落地时形成开发参考。状态、槽位和事件同事务更新，SQL 带当前状态/版本条件；Node 接收防重、审批消费、启动意图事务边界明确。数据库本机磁盘，启用外键、WAL、合理 busy_timeout，限制写并发，事务内不等待网络或进程。持久 events 用于查事实，不做事件溯源/EventBus。

健康与 ready 分开；不可写时不能显示可接单。日志以 execution_id/node_id/request_id 关联，秘密及完整内容不进入常规日志。迁移先在临时状态目录验证，不兼容时明确失败；备份采用停机一致性流程，不运行中只复制主库遗漏 WAL。

| 参数 | 初始规划值 |
| --- | --- |
| 心跳 / 离线 | 10 秒 / 30 秒 |
| 运行 timeout | 默认 120 秒、上限 1 小时，Node 可更严格 |
| 审批 TTL | 10 分钟，不消耗进程运行 timeout |
| 幂等与 Node 防重窗口 | 默认 30 天，结果/journal 保留满足确认与防重约定 |
| 单次读取 / 结果正文 | 64 KiB；编码开销另计并校验完整帧 |
| 单 execution 日志 | stdout + stderr 合计 10 MiB |
| Node 产物预算 / 完成日志留存 | 1 GiB / 7 天 |
| 控制帧 / 产物分块 | 1 MiB / 不超过 64 KiB |

以上是待实测的起点，不是已达到的性能承诺。调整数值需记录场景、取舍和验证，不能为一次 Demo 无限放宽限制。

## 6. 总验收清单

保留 A01–A16 原编号，作为本计划唯一的总体验收清单。所有条目目前仍需业务层完整验收；历史探针通过不将 A02 标记完成。

| 编号 | 必须证明的行为 | 主要收口阶段 |
| --- | --- | --- |
| A01 | 干净 Linux 环境使用发布包/示例启动 Server 和 Node，记录步骤与依赖 | P5 |
| A02 | 真实客户端记录客户端/SDK/实际协议版本，完成发现、提交、审批等待、查询、显式取消及授权产物读取 | P1 基础、P5 完整 |
| A03 | A 缺目标资源、B 满足；不指定 B 自动选中，trace 可解释 | P3 |
| A04 | 能力缺失、资源不共址、revision 不符时不执行、不降级 | P3 |
| A05 | 并发不能双占一个槽；WAITING_APPROVAL/UNKNOWN 也不超发 | P1/P3/P4 |
| A06 | 同 request_id 同内容返回原记录，不同内容 IDEMPOTENCY_CONFLICT | P1 |
| A07 | 重复消息/丢确认副作用计数器仅启动一次；模糊启动窗口 UNKNOWN | P2/P4 |
| A08 | Server/Node 重启不凭内存空闲重跑；未核实残留使 Node 不可调度 | P4 |
| A09 | 取消/超时结束测试进程与受管理后代、回收管道；完成竞争保留事实 | P2/P4 |
| A10 | `..`、绝对路径、外部符号链接及替换竞态均不使文件 API 越界 | P1、P5 含 fs.read |
| A11 | 变参、过期、策略/映射变化、错误 Node 不复用审批；DENY 不可绕过 | P2 |
| A12 | 错/撤销 token、伪造节点、旧 epoch 被拒；Node token 不作 principal | P1/P4 |
| A13 | 大 stdout 不耗尽内存/死锁；禁止返回内容不经读取接口泄露 | P2/P5 |
| A14 | artifact 分页、未授权、离线、过期、磁盘限额都有明确结果 | P5 |
| A15 | 客户端关闭后已提交执行仍持久存在，重开可凭 ID/CLI 查询 | P1/P5 |
| A16 | 状态/事件原子性、存储不可写不启动、迁移与备份恢复可验证 | P1/P4/P5 |

验证方法：placement 固定快照表驱动、协议 JSON fixtures、生命周期真实子进程、幂等与重启真实持久存储、网络受控故障注入。按变化执行 go test、目标环境可运行的 race 检查、go vet、依赖漏洞检查及发布构建。普通单元测试不调用 LLM，真实 Harness 端到端独立记录；不追求无意义覆盖率或反复全量测试。

Linux Server/Node 是必验目标。Windows CLI/bridge 只有正式产品构建与客户端流程通过才声明支持；探针或 WSL 不等于 Windows 原生 Node 支持。额外平台不适用可注明，不能用不适用跳过 Linux 核心检查。

## 7. Demo、发布与文档交付

依次完成：单节点授权目录列表与受控进程；目标数据只在 B 的双节点路由及硬约束负例；同一 Harness 任务先读 A 仓库、再到 B 对本地样例数据运行已准备脚本并整合结果。多次执行共用 correlation_id。涉及同一代码时由操作者提前准备相同 revision 的干净工作树，不隐式 git pull/scp。基础路由不需 GPU；声称 CUDA 训练支持则另做真实 GPU 验证。

发布交付物包括验证平台的统一二进制与校验值、配置示例、至少一个真实客户端的完整接入记录、请求/协议/schema 参考、迁移文件、A01–A16 证据、安装/审批/取消/UNKNOWN/凭据撤销/产物限制/备份恢复指南，以及发布说明和已知限制。文档命令在适用干净环境验证，软件版本与协议版本分别记录。

存在可复现的重复启动、授权绕过、文件越界、执行状态丢失、取消虚报或无界资源消耗时阻止首发。完整验收与必要文档通过后才可标首版就绪；具体发布操作依项目正常流程执行，本计划不自动授权对外联系或发布。

按 [文档维护规范](docs/开发文档维护规范.md) 随实现维护设计/ADR、接口、指南与证据。每周选择可验证结果，记录范围、依赖和验收；周末更新开发进度、已知问题与下一步。阶段过关看证据和文档，不看目录、代码量或日历。

## 8. 风险与调整规则

| 风险 / 缺口 | 应对 |
| --- | --- |
| 第二个真实 Linux Node 或有效开发时间不足 | 尽早落实环境、记录实际投入并重排；不以两份 mock 声称双节点通过 |
| 客户端不会稳定构造请求或轮询 | 改善 schema、说明、错误与示例，锁定已验证版本；不把 SDK 自测替代用户流程 |
| 进程回收/崩溃语义迟迟不清楚 | 收紧前台边界，验证真实进程组与 UNKNOWN；不只杀父进程就报完成 |
| Demo 依赖同步或配置过繁 | 预准备明确版本，改善配置/诊断与显式可信策略；不暗中同步或默认任意执行 |
| 13 周内无法达标 | 优先延后 CLI 美化、额外客户端/平台、非核心解析器；重估日期，不裁幂等、授权、超时、对账及核心验收 |
| 新 Harness / 企业 / 平台需求进入 | 记录后续目标与成本；MCP 闭环通过前不扩张当前开发目标 |

六个 MCP 工具、三种操作及基础产物能力如需移出首版，必须明确修订范围和验收，不能继续按原目标宣称全部完成。原半年计划中第 4–6 月的试用、维护与扩展内容已转为项目规划中的长期方向，不作为本三个月计划的额外交付。

# RunWeave 项目规划书

> **A distributed agent execution layer — usable as its own harness, and as an MCP-compatible execution backend for any other agent harness.**

**Version:** v0.1 (基于 Fabric v0.0.1 草案修订)
**Status:** Architecture / MVP Planning
**Project Name:** RunWeave（原代号 Fabric，已因命名冲突正式更名）
**Category:** Distributed Agent Execution Layer / MCP Execution Backend

---

# 0. 本次修订说明

这一版在原 Fabric v0.0.1 规划的基础上做了三个方向性调整，理由见下方讨论记录，这里只列结论：

1. **正式更名为 RunWeave**。"Fabric" 与 Daniel Miessler 的开源项目 fabric（AI 提示词增强框架，社区知名度较高）以及 Microsoft Fabric（数据分析平台）存在实质性撞名，作为公开项目名不可用。原文档 Appendix 已预见这个风险，这里直接把决策落地，全文改名。
2. **目标用户从"多机器 ML 研究者"收窄并重新定位为 B 端 / Prosumer**：优先服务有多台设备、且对"把代码/数据发给纯云端 Agent"有顾虑的技术团队和独立开发者，明确不在 v0.1 追求 C 端消费者体验。
3. **新增"MCP 兼容执行层"作为贯穿全文的第二条产品主线**：RunWeave 除了可以作为独立的 Agent Harness 使用，还应该把自己的 placement + execution 能力封装成一个标准 MCP Server，让 Claude Code、Cursor、Cowork 等已经有用户基础的 Agent Harness 可以直接调用 RunWeave 的节点网络，而不需要各自重新实现一套多节点调度。这条线原本只是原文档第 84 节里的一句旁注（"MCP = Capability Interface，Fabric = Execution Runtime"），现在把它提升为核心战略。

另外，原文档 103 个章节里有相当一部分是用不同的 ASCII 图反复表达同一组核心思想（reason globally / move computation to data / task ≠ machine）。这一版做了合并精简，技术决策和可执行内容全部保留，重复的比喻性重申做了删减。

---

# 1. Executive Summary

RunWeave 不是另一个 Codex、Claude Code、Cline 或 Cursor。它要解决的不是"如何让一个 Agent 在一台机器上更聪明地写代码"，而是：

> **如何让一个 Agent 把用户分散在不同设备、服务器、云环境中的计算能力，当成一个统一的执行环境来调度？**

现代技术团队和独立开发者的真实计算环境天然是分布式的——笔记本上有工作目录和 IDE，实验室或家里的工作站上有 GPU 和私有数据集，云上有可以长时间运行的沙箱，生产服务器上有日志和数据库。但今天大多数 Agent Harness 仍然假设"Agent = 当前这台机器"，结果是：数据要么被搬到 Agent 所在的地方，要么用户自己在不同工具之间来回切换、手动 SSH、手动 scp。

RunWeave 的核心思想：

> **Reason globally, execute locally. Move computation to data, not data to computation.**

Agent Loop 可以运行在云端做推理决策，但真正的执行发生在拥有所需能力和数据的节点上，结果以结构化 observation 的形式返回给推理层。

**这一版规划的关键变化是：RunWeave 有两种使用形态，共享同一套底层引擎。**

- **形态一（自己的 Harness）**：RunWeave 提供一个云端 Agent Loop + Web/CLI 控制界面，用户直接用它管理和驱动跨设备任务。这是原有 Fabric 规划的主线，面向愿意直接采用一个新工具的技术团队和 prosumer。
- **形态二（MCP 执行层）**：RunWeave 把"发现节点能力 → 按能力和数据局部性做 placement → 执行 → 返回结构化结果"这整套引擎，通过标准 MCP 协议暴露成一个 Server。任何已经支持 MCP 的 Agent Harness（Claude Code、Cursor、Cowork 等）都可以把 RunWeave 当成一个工具挂载，遇到需要跨设备执行的任务时直接调用，不需要了解 Node、Capability、Placement 这些内部概念。

形态二把 RunWeave 从"一个要跟其他 Agent 产品抢用户的新工具"变成"任何 Agent 产品都可以依赖的执行基础设施"——不需要自己去获取终端用户，而是借助已经有分发渠道的 Harness 产品触达他们的用户。这是这一版规划里最重要的定位调整。

---

# 2. 项目定位与目标用户

## 2.1 RunWeave 是什么

RunWeave 是一个 **Distributed Agent Execution Layer**。它允许一个中心 Agent Runtime（自己的或第三方的）：

- 感知多个 Execution Node 及其能力（capability）与数据局部性（resource locality）；
- 根据任务的 requirements 自动完成 execution placement；
- 把工具调用或任务派发到合适节点执行；
- 收集执行结果，转化为结构化 observation 返回给推理层；
- 在多个节点之间连续推进同一个 Task，而不绑定任何一台机器或一次会话。

于是：

```text
Laptop / GPU Workstation / Cloud VM / Production Server / Browser Runtime
```

对 Agent 而言不再是互不相干的五个环境，而是 **One Execution Fabric**。

## 2.2 目标用户：B 端 / Prosumer 优先

原规划里的场景（笔记本 + 实验室 GPU + SAR 数据集）本质上是一个很好的起点，但只是"有多台异构设备的技术用户"这一大类里最窄的一个子集。v0.1 把目标用户明确为：

**首要用户：**

- 有多台设备（个人笔记本 + 家用/公司工作站 + 云资源）、习惯用命令行和 YAML 配置的独立开发者、小型技术团队；
- 对把源码、生产日志、内部数据库直接发给纯云端 Agent 有安全或合规顾虑的团队——RunWeave 的 "数据默认留在拥有它的 Node" 天然回应了这个顾虑。

**次要用户（通过 MCP 自然获得，不需要单独获客）：**

- 已经在使用 Claude Code、Cursor、Cowork 等 Agent Harness 的开发者。他们不需要知道 RunWeave 这个名字，只需要在自己的 Harness 里挂载一个 MCP Server，任务需要跨设备执行时会自动路由过去。这部分用户的获取成本几乎为零，靠已有产品的分发渠道。

**明确排除（见第 15 节"明确不做"）：**

- v0.1 不做面向普通消费者的"帮我在手机和电脑之间同步文件/照片"这类零配置、全图形界面的产品体验。那是一个几乎需要重新设计交互的独立产品方向，风险和投入应该单独评估，不应该现在分散精力。

## 2.3 与其他 Agent Harness 的关系

RunWeave 不是要替代 Claude Code、Cursor 这类 Harness，而是补齐它们目前普遍缺失的一块能力：**跨设备的、按能力和数据局部性感知的执行调度**。

```text
MCP  = Capability Interface（有什么工具？）
RunWeave = Execution Runtime（这个任务应该在哪里执行？）
```

二者互补而非竞争——这个判断在原规划第 84 节就已经提出，这一版把它从一句旁注升级为产品策略的核心。

---

# 3. RunWeave 不是什么

v0.1 必须主动控制范围。RunWeave **不是：**

- **另一个 Coding Agent**——更好的代码生成/diff/自动补全可以存在，但不是核心竞争方向；
- **另一个 MCP 聚合器**——"LLM + 50 个 MCP Server"不会自然形成新的 Harness architecture；RunWeave 关心的是执行发生在哪，而不是工具有多少；
- **一个万能 Personal Assistant**——v0.1 不追求邮件、日历、PPT、社交媒体这类 Application Width，RunWeave 的宽度是 **Execution Width**；
- **一个远程 Terminal Controller**——如果最终架构只是 "Cloud LLM → SSH → Remote Machine"，那么 RunWeave 没有成立，Remote Tool Calling 只是实现机制，真正的价值在 Requirement → Capability Resolution → Data Locality Resolution → Placement 这条链路；
- **一个面向 C 端消费者的设备同步/协同产品**——这是一个值得未来探索的方向，但需要几乎重新设计交互（移动优先、零配置、隐藏底层概念），v0.1 不做，见第 15 节。

---

# 4. 核心 Thesis

## Principle 1 — Reason globally
主要 Agent Loop、Task State、Context、Decision Making 位于统一控制平面。

## Principle 2 — Execute locally
任务应该尽可能在拥有所需数据和能力的环境中执行。

## Principle 3 — Move computation to data
默认不把 `Node Data → Upload → Cloud Agent`，而是 `Cloud Task → Node → Local Computation`。

## Principle 4 — Move information back to reasoning
Node 返回 observation / summary / metrics / structured result / artifact 引用 / mutation 信息，而不是无脑返回全部数据。

## Principle 5 — Schedule by capability, locality and policy
用户描述目标，Agent 描述 requirement，Runtime 决定 placement。

---

# 5. 核心抽象

RunWeave 第一版围绕九个实体设计：`Agent / Task / Node / Capability / Resource / Execution / Placement / Artifact / Policy`，而不是传统 Coding Agent 的 `Chat / Session / Workspace / Terminal`。

## 5.1 Agent

负责 Reasoning / Planning / Tool Selection / Task Progression / Observation Interpretation。v0.1 只需要 **One Primary Agent**，暂不做 Multi-Agent。Agent Loop 可以直接借鉴 nanobot 的极简思想（Input → Context → LLM → Tool Call → Observation → … → Final），RunWeave 真正新增的是 **Agent Loop 之下的 Execution Abstraction**，而不是重新发明 Agent Loop 本身。

## 5.2 Task

Task 表示"用户希望系统持续完成的一个目标"，例如 *Run tests for nanobot and investigate failures*。Task 有独立生命周期（`CREATED → PLANNING → RUNNING → WAITING → RUNNING → COMPLETED`，异常态 `FAILED / CANCELLED / BLOCKED`），且不与任何 Browser / Laptop / Cloud 会话绑定：

> **Task ≠ Session，Task ≠ Machine，Task ≠ Node。**

用户可以在 Web 上创建 Task，关闭网页，之后在手机上继续查看，Task 依然存在。

## 5.3 Node

Node 是能提供 interaction 或 execution capability 的实体。

- **Execution Node**：Laptop / Desktop / Lab GPU Server / Cloud VM / Sandbox / Browser Runtime，提供 filesystem / shell / git / docker / cuda / browser / database 等能力。
- **Interaction Node**：Web / Phone / CLI / IDE / Slack，主要负责 input / approval / notification / result presentation，不一定负责执行。
- 一台笔记本可以同时是 Interaction Node 和 Execution Node（Hybrid Node）。

**手机的定位**：不要强行把手机定义成计算 Node，它最大的价值是 Approval / Notification / Observation Node——"需要权限推送 fix/runtime 分支" → 手机上 [Allow]/[Deny]。

**Web 的定位**：Web 是 Control Interface，不是 Agent Runtime。用户可以创建 Task、查看 Node、审批操作、查看 Trace，关闭网页 Task 不消失。

**IDE 的定位**：未来的 VS Code Extension 也只是 Interaction Node + Local Context Provider，而不是整个 Harness。

## 5.4 Capability 与 Resource

Capability 回答"这个 Node 能做什么"，Resource 回答"这个 Node 拥有什么"。Node 不应该只是一个 IP 地址，而应该是：

```yaml
node:
  id: lab-pc
  capabilities: { shell: true, filesystem: true, git: true, docker: true, cuda: true }
  hardware: { gpu: { model: RTX-5060, vram: 8GB } }
  os: { type: linux }
```

Agent 应该表达 "I require CUDA + dataset://sar/gf3"，而不是 "run command on lab-pc"，由 RunWeave 决定落到哪个节点。Resource 采用逻辑 URI（`repo://` `dataset://` `logs://` `service://`，未来可扩展 `device://` `database://` `secret://`），例如：

```yaml
resources:
  repo://nanobot: { locations: [laptop, lab-pc] }
  dataset://sar/gf3: { locations: [lab-pc] }
```

用户不再需要告诉 Agent `D:\research\SAR\data\GF3`，LLM 只操作逻辑资源，Runtime 负责解析成物理路径 + 节点。

## 5.5 Execution 与 Placement

Execution 表示"Task 在某个 Node 上发生的一次具体执行"，必须独立记录 `execution_id / task_id / node_id / command / started_at / finished_at / status / result / artifacts / mutations`。

Placement 是"把 Execution 分配给某个 Node 的过程"，这是 RunWeave 与普通 Agent Harness最重要的区别：

```text
传统：tool_call → current environment
RunWeave：tool_call → requirements → placement → best node → execution
```

**Placement Algorithm（v0.1，确定性过滤，不做 AI Scheduler）：**

```python
candidates = nodes
candidates = match_capabilities(candidates)
candidates = match_resources(candidates)
candidates = enforce_policy(candidates)
candidates = filter_online(candidates)
node = select_best(candidates)
```

优先级：`Resource Locality > Capability Match > User Preference`。

**`select_best()` 的具体规则（原规划遗留的空白，这一版补齐）**：当多个候选节点在 capability 和 resource locality 上打平时，v0.1 按以下顺序决出唯一结果，保证行为确定、可复现、可测试：

1. 优先选择当前 `idle` 状态（而非刚执行完仍在冷却）的节点；
2. 其次选择最近一次 heartbeat 时间最新的节点（更可能真正在线、延迟更低）；
3. 仍打平则按 `node_id` 字典序取第一个。

后续版本再引入 latency / cost / load / security 的加权评分（`score(node) = w1·capability_match + w2·data_locality + w3·security + w4·latency + w5·cost + w6·availability + w7·load`），v0.1 不实现。

## 5.6 Artifact 与 Result Model

Execution 不应该把结果直接塞回 LLM Context，而应产出 `Observation + Artifact`：

```yaml
result:
  status: failed
  summary: "pytest failed with 3 tests"
  observations: { failed_tests: 3 }
  artifacts: [artifact://execution/123/test.log, artifact://execution/123/report.xml]
  mutations: [src/runtime.py]
```

统一结果模型：

```python
ExecutionResult:
    execution_id, status, summary, observations, artifacts, mutations, error
```

**为什么必须结构化**：否则远程执行会迅速退化成 `LLM → ssh command → 20000 token stdout → LLM`。RunWeave 要保证 `Execution output ≠ Agent observation`，这个区分从第一版就要保留。

Mutation Tracking 同样重要——Coding Agent 最关键的安全问题之一是"到底改了什么"，Execution Result 应包含 `files_modified / files_created`，为未来的 rollback / diff / approval 打基础。

## 5.7 Policy

远程 Agent 最大的问题不是"能不能运行命令"，而是"应不应该允许它运行"。v0.1 实现最简单的三态：

```text
read_file → ALLOW
pytest    → ALLOW
git commit / git push / rm → ASK
sudo      → DENY
```

收到 `ASK` 时 Execution 进入 `WAITING_APPROVAL`，通知 Interaction Node（通常是手机）。完整链路：`Agent → Execution Request → Placement → Policy → Approval? → Execution`。

---

# 6. MCP 兼容执行层：第二种接入方式（核心新增）

## 6.1 为什么要做

RunWeave 如果只做"自己的 Harness"，获客路径是从零开始跟 Cursor、Cognition、Claude Code 这类已经有大量用户和资金的产品正面竞争抢用户。但如果把 placement + execution 引擎暴露成一个标准 MCP Server，RunWeave 就变成这些产品可以直接依赖的基础设施——不需要它们放弃自己的 Agent Loop，只需要在需要跨设备执行时调用 RunWeave 这一个工具。这是一个"卖水"而不是"挖金"的策略：不需要自己获取终端用户，借助已有产品的分发渠道触达。

## 6.2 落地设计

RunWeave Server 增加一个 MCP Server 接口，暴露的核心工具大致是：

```json
{
  "name": "runweave.execute",
  "description": "Execute a task requirement against the RunWeave node network. RunWeave resolves capability and data-locality requirements to the best available node and returns a structured result.",
  "input_schema": {
    "requirements": {
      "capabilities": ["shell", "python", "cuda"],
      "resources": ["dataset://sar/gf3"],
      "constraints": { "os": "linux" }
    },
    "task": "Run the SAR training experiment and report the result."
  }
}
```

返回值直接复用第 5.6 节的 `ExecutionResult` 结构——这意味着"自己的 Agent Loop 调用 Placement"和"外部 Harness 通过 MCP 调用 Placement"走的是完全同一条内部路径，不需要维护两套逻辑。另外还应该暴露只读的发现类工具，比如 `runweave.list_nodes` / `runweave.list_resources`，让外部 Harness 的 Agent 在决定是否需要跨设备执行之前，先了解当前节点网络有什么能力和数据。

## 6.3 对第三方 Harness 的价值主张

对方不需要理解 Node / Capability / Placement 这些内部概念，只需要知道："挂载这个 MCP Server 之后，遇到需要访问不在本机上的能力或数据的任务，直接调用它，会自动路由到正确的地方并拿到结构化结果。" 这个价值主张对已经在维护自己 Agent Loop、但没有多节点调度能力的团队来说，成本极低，收益直接。

## 6.4 与"自己的 Harness"共享基础设施

两种形态共享同一套 Node Registry、Placement Scheduler、Execution Router、Policy Engine、Artifact Store：

```text
                Placement / Execution / Node / Policy Engine
                              ▲                ▲
                              │                │
              自己的 Agent Loop         MCP Server 接口
                    (形态一)                 (形态二)
```

唯一的区别是"谁在发起 execution request"——自己的 Agent Loop，还是第三方 Harness 通过 MCP 协议。这也意味着形态二不需要额外的开发投入去重新实现调度逻辑，主要工作量是包装一层标准 MCP 协议接口 + 权限/计费边界（谁的哪些 Node 可以被哪个外部 MCP 调用方访问）。

---

# 7. 架构总览与三大组件

```text
                         RUNWEAVE SERVER
┌──────────────────────────────────────────────────────┐
│                    Agent Runtime (可选，形态一使用)      │
│                        │                              │
│                   Main AgentLoop                      │
│                        │                              │
│  ┌─────────────────────┴─────────────────────┐        │
│  │            MCP Server 接口 (形态二使用)       │        │
│  └─────────────────────┬─────────────────────┘        │
│                        │                              │
│                Execution Request                      │
│                        │                              │
│               Capability Resolver                     │
│               Placement Scheduler                     │
│                 Execution Router                      │
└────────────────────────┼─────────────────────────────┘
                    RunWeave Protocol
            ┌────────────┼────────────┐
            ▼            ▼            ▼
       Laptop Node    Lab Node     Cloud Node
```

**三个核心模块：**

- **runweave-server**：Agent Loop（可选）、MCP Server 接口、Task State、Node/Capability Registry、Placement、Execution Routing、Result Collection、Web/API。
- **runweave-node**：运行在 Laptop / Desktop / Server / VM 上的 lightweight daemon，负责 connect / heartbeat / capability advertise / resource advertise / receive execution / policy enforcement / local execution / artifact collection / result return。
- **runweave-protocol**：定义 Node registration、Heartbeat、Capability advertisement、Execution request/event/result、Artifact reference、Approval、Cancellation。

**协议选择**：WebSocket + JSON Messages。理由：双向、简单、debug 方便、Node 主动发起 outbound 连接（不要求用户开放本地端口，Server 不主动访问用户局域网），后续容易迁移到 protobuf/gRPC。

**关键消息示例：**

```json
// Node 注册
{ "type": "node.register", "node_id": "lab-pc", "capabilities": ["shell","filesystem","git","docker","cuda"] }

// Execution 请求
{ "type": "execution.request", "execution_id": "exec_001", "tool": "shell", "arguments": { "command": "pytest" } }

// Execution 流式事件
execution.started / execution.stdout / execution.stderr / execution.artifact / execution.completed / execution.failed
```

**Tool Abstraction**：`shell` 不是一个具体的 shell，而是一种 capability，不同节点均可以提供：`ShellTool → ExecutionRouter → NodeShellExecutor`。Tool call 应携带 `ExecutionContext(task_id, requirements, resources, preferred_node)`，由 Execution Router 自动 placement。

---

# 8. Control Plane / Execution Plane

```text
              CONTROL PLANE
          RunWeave Server
               │
        Agent + Scheduler
               │
────────────────────────────────
             Protocol
────────────────────────────────
               │
         EXECUTION PLANE
    Node A      Node B      Node C
```

Control Plane 负责 Agent / Task / Scheduling / State / Policy Definition / Registry / Observation；Execution Plane 负责 filesystem / shell / git / docker / GPU / browser / database。长期可以进一步拆出 **Data Plane**（大体积 artifact 不必经过 Agent Server，直接落 Object Storage，Cloud Brain 只拿到 `artifact://123` 引用），v0.1 不展开。

---

# 9. 安全与信任边界（面向 B 端的可信故事）

这一节是把原规划里分散的安全设计（Filesystem Safety / Shell Safety / Trust Boundary / Secret / Privacy Story）合并，并用 B 端采购者更容易理解的语言重新表达——这一部分现在应该是对外销售叙事里最重要的一块，因为它直接回应了企业和团队"能不能把 Agent 接进内网"这个核心顾虑。

## 9.1 核心原则：Node 是最终授权方

> Cloud Brain 默认不拥有 Node 的全部权限。Node 自己是最终授权方。

```text
Cloud → request → Node Policy Engine → allow / deny
```

而不是 "Cloud has root"。这是项目可信度的基础，也是 RunWeave 相对于"直接把 SSH 密钥交给云端 Agent"这类方案最大的差异点。

## 9.2 默认安全设计（v0.1 就要有）

- **Filesystem Safety**：Agent 不应该看到整台电脑。Node 配置 `filesystem.roots`（如 `/home/kail/projects`, `/data/sar`），所有 filesystem 操作限制在 roots 内。
- **Shell Safety**：不直接把物理路径暴露给 Cloud Agent，Node 自己把 `repo://nanobot` 解析成本地路径；execution 请求必须带 `working_directory / timeout / output_limit`。
- **Resource 手动配置，不自动扫描**：v0.1 不自动扫描用户文件系统去发现资源，既减少复杂度也减少隐私风险。
- **Human Approval 三态**（`ALLOW / DENY / ASK`，见 5.7）。

## 9.3 Privacy Story（对外叙事的核心卖点）

用户的 Private Dataset / Source Code / Credentials / Logs 可以始终保持在 User-controlled Node 上，Cloud Agent 只获得任务需要的 Observation。这不是"上传数据给云 Agent"，而是"云 Agent 向数据位置派发 computation"。对有安全和合规顾虑、又想用 AI Agent 处理内部代码和数据的团队来说，这个价值主张不需要过多解释。

## 9.4 面向企业的能力路线图（明确标注：不在 v0.1 范围内，但要提前规划）

以下能力是安全/合规负责人在评估阶段一定会问到的，v0.1 不实现，但应该写进路线图而不是完全不提，否则会被解读为"这东西以后管不住"：

- Node 身份的签发与吊销机制（目前 v0.1 用简单 token，见 9.5）；
- SSO / 团队级 RBAC；
- 审计日志导出（基于第 10 节的 Event Model，天然可以扩展）；
- 更细粒度的 Policy（按用户、按项目、按时间窗口）。

## 9.5 v0.1 的 Node Enrollment（够用即可）

```bash
# Server 生成 token
runweave node-token create
# → rw_xxxxxxxxx

# Node 连接
runweave node connect --server https://runweave.example.com --token rw_xxxxxxxxx
```

Node 身份记录 `node_id / name / token / capabilities / resources / metadata`。

---

# 10. 状态、存储与可观测性

## 10.1 Execution Lifecycle 与 Failure Model

```text
CREATED → PLACED → DISPATCHED → RUNNING → COMPLETED
异常：FAILED / CANCELLED / REJECTED / TIMEOUT / NODE_LOST
审批：WAITING_APPROVAL
```

分布式 Runtime 最大的区别之一是 **Node 会消失**。即使 v0.1 不实现自动恢复，也必须承认 `NODE_LOST` 是一种正常的 execution failure，不能假设 tool call 总会返回。相应地，v0.1 就要支持 `execution.cancel`（用户可能启动 pytest/training/download 后取消，Node executor 必须保留 process handle），以及强制 `timeout`（否则一次 tool call 可能永远阻塞 Agent Loop）。并发策略保持简单：每个 Node `max_concurrent_executions = 1`，不要过早设计复杂 worker pool。

## 10.2 存储

v0.1 用 **SQLite** 保存 `tasks / nodes / executions / resources / artifacts / events`，不要一开始上 PostgreSQL——目标是"开发者 clone 后 5 分钟启动"。Node 在线状态可以是 ephemeral。

## 10.3 Event Model 与 Observability

所有重要状态变化都应产生 Event（`task.created` / `node.connected` / `execution.started` / `execution.completed` / `artifact.created` 等），初期用 In-Memory EventBus，Redis/NATS/Kafka 都不进入 v0.1。这套 Event 是未来 Observability / Replay / Audit / Debug / 实时 UI 的共同基础。

Execution Trace 至少要能回答"Agent 为什么决定在这台机器执行"：

```text
Task #17
12:41 Agent requested shell
12:41 Required: git + repo://nanobot
12:41 Candidate nodes: laptop, lab-pc
12:41 Selected: laptop
12:41 Execution #51 started → completed
12:41 Result returned to Agent
```

## 10.4 Agent Context

不要把所有 Execution history 塞进 prompt。Agent Context 应只包含 Task Goal / Recent Decisions / Relevant Observations / Artifact 引用 / Current State，Execution 日志存在外部——这是让长任务保持可扩展的关键。

---

# 11. UI / CLI / 多端角色

## 11.1 v0.1 UI（不需要好看，四个页面够用）

`Chat/Task`（Prompt + Task Status + Agent Output）、`Nodes`（在线状态 + capability 列表）、`Executions`（execution 记录）、`Resources`（逻辑 URI → 所在节点）。

## 11.2 CLI

```bash
runweave server                              # 启动 Server
runweave node connect <server> --token ...   # Node 连接
runweave node list                           # 查看节点
```

`runweave task / runweave run / runweave exec` 留到后续版本。

## 11.3 Dashboard 的长期形态

未来 Dashboard 不应该主要长得像 ChatGPT，更应该像一个运维面板：`Tasks / Nodes / Resources / Executions / Artifacts / Trace`。Chat 只是 Task 的一种入口，这也呼应了第 2 节里"Task 是产品对象，Chat 只是交互方式之一"的定位。

---

# 12. Killer Demo 与 MVP 用户故事

## 12.1 三个渐进式 Demo（沿用原规划，验证核心 thesis）

**Demo 1 — 单节点远程操作**：Cloud Agent 操作另一台电脑上的 Git 仓库（inspect → run pytest → 解释失败原因），验证最基本的 remote execution 链路。

**Demo 2 — 按能力 placement**：接入第二个节点（GPU Workstation），用户说"跑 SAR 训练实验"，RunWeave 自动识别需要 `dataset://gf3 + cuda`，placement 到 GPU 节点而不是笔记本。这时 **Capability-based Execution** 第一次真正成立。

**Demo 3 — 单任务跨节点**：一个 Task 内部分派多个 execution（inspect source → 笔记本，inspect dataset → GPU，reason，modify source → 笔记本，validate → GPU）。这时 **Task ≠ Machine** 真正成立。

## 12.2 MVP 用户故事（重新对齐 B 端 / prosumer 定位）

**Story A（个人/团队多设备）**：开发者在笔记本上装了 RunWeave Node，Web 端说"分析一下 repo://nanobot 的架构"，Agent 远程读取代码并回答——不需要 SSH，不需要 clone 到本地。

**Story B（安全顾虑场景，B 端核心卖点）**：团队想用 AI Agent 排查生产问题，但生产日志和数据库不能离开内网。RunWeave 在内网机器上部署一个 Node，Cloud Agent 只发出诊断请求、拿到结构化 observation，源数据始终不出内网。

**Story C（能力 + 数据局部性）**：团队有第二台 `lab-gpu`，说"用 dataset://sar/gf3 跑训练"，系统自动选择 `lab-gpu` 而不是笔记本。

**Story D（跨机器单任务）**：一个任务跨机器完成——分析数据集在 GPU 上，改代码在笔记本上，RunWeave 自动串联。

**Story E（第三方 Harness 通过 MCP 调用，新增，验证第 6 节战略）**：开发者平时在 Cursor 里正常工作，某个任务需要访问一台没有直接连接到 Cursor 的机器上的资源（比如内网数据库）。Cursor 里挂载的 RunWeave MCP Server 自动识别这部分请求、路由到正确节点、拿到结构化结果返回给 Cursor 的 Agent Loop。用户全程不需要打开一个新工具，也感知不到"RunWeave"这个名字的存在。

---

# 13. 开发阶段规划

## Phase 0 — Kernel Extraction
保留 nanobot-style Agent Kernel（AgentLoop / Model abstraction / Tool abstraction / Context / EventBus）。重点：Agent Loop 不依赖 Node。

## Phase 1 — Node Connectivity
实现 runweave-server / runweave-node / WebSocket / register / heartbeat。验收：Server 能看到 Node 在线。

## Phase 2 — Remote Execution
实现 shell capability / execution request / stdout streaming / result / timeout / cancel。验收：Cloud Agent 能远程执行 `pwd`。

## Phase 3 — Filesystem
实现 read_file / write_file / list_dir / filesystem roots。验收：Agent 能分析远程仓库。

## Phase 4 — Resource Registry
实现 `repo://` `dataset://` 与 resource advertise。验收：Agent 不再需要物理路径。

## Phase 5 — Capability Placement
两台节点（Node A: shell+git，Node B: shell+git+cuda），需要 CUDA 的任务自动落到 Node B。

## Phase 6 — Data Locality Placement
`dataset://gf3` 只在 Node B，需要该数据集的任务自动落到 Node B。验收：**Move computation to data** 成立。

## Phase 7 — Multi-Node Task
同一 Task 在 Node A（inspect source）和 Node B（execute training）之间连续推进。验收：**Task ≠ Machine** 成立。

## Phase 8 — MCP Server Exposure（新增，v0.1 的收官阶段）
把 Phase 1-7 建好的 placement + execution 引擎，通过标准 MCP 协议暴露为 `runweave.execute` / `runweave.list_nodes` / `runweave.list_resources`。验收：至少一个不是 RunWeave 自己的 Agent Harness（如 Claude Code 或 Cursor），能够连接这个 MCP Server，发起一次真实的跨节点 execution 请求，并拿到结构化结果——全程不需要了解 Node/Placement 的内部实现。

---

# 14. v0.1 Success Criteria

不看模型数量、工具数量、MCP 数量，看以下六件事情：

- **S1**：Cloud Agent 可以可靠调用远程 Node。
- **S2**：两个 Node 可以同时连接。
- **S3**：每个 Node 能 advertise 不同 Capability。
- **S4**：Execution 可以根据 Capability 自动 placement。
- **S5**：Execution 可以根据 Resource locality 自动 placement。
- **S6（新增）**：至少一个第三方 Agent Harness 能通过 MCP 协议成功调用 RunWeave，完成一次跨节点 execution。

S1-S5 验证的是"核心 thesis 技术上成立"；S6 验证的是这一版最重要的战略假设——"RunWeave 值得被当作基础设施依赖，而不只是又一个独立工具"。两者都成立，v0.1 才算真正成功。

---

# 15. 明确不做

这是规划中最重要的部分之一。v0.1：

不做 Multi-Agent；不做 DAG Scheduler；不做 Kubernetes；不做 P2P；不做 Agent-to-Agent Protocol；不做自动迁移文件；不做 GPU Scheduler；不做 Container Orchestration；不做 MCP Marketplace；不做 IDE Extension；不做完整 Browser Agent；不做 Vector Database；不做复杂 Memory；不做 RAG Platform；不做 Multi-Tenant SaaS；不做 Enterprise RBAC（路线图见 9.4，非 v0.1）；不做复杂 Cost Scheduler；不做 Local LLM。

**新增两条（呼应本次定位调整）**：

- **不做面向 C 端消费者的零配置体验**（移动优先、隐藏所有技术概念的"设备协同"App）。这是一个独立的产品方向，需要单独评估投入，v0.1 不分心。
- **不做 Mobile App（原生）**——手机在 v0.1 的角色仅限于 Approval / Notification（见 5.3），通过 Web 的响应式界面或轻量通知即可满足，不需要单独开发原生 App。

这份清单是保持 nanobot 式 architecture clarity 的关键。

---

# 16. 与已有系统的差异化定位

## 16.1 与 Kubernetes 的类比

```text
Kubernetes：Workload → Resource Requirement → Scheduler → Node
RunWeave： Agent Task → Capability Requirement → Placement Scheduler → Execution Node
```

区别：Kubernetes 调度 containerized workload；RunWeave 调度 **Agent-generated execution**，并且多了 data locality、human approval、LLM reasoning、agent observation 这几个额外维度。这个类比本身是合理的，但对外沟通时需要更进一步回答"为什么不直接用 Nomad/Ray/SkyPilot 这类已经解决了异构节点调度的现成系统"——答案是：这些系统调度的是预先定义好的 batch/service workload，而 RunWeave 调度的 requirement 是 **Agent 在推理过程中动态生成的**，调度决策需要和 LLM 的推理循环双向交互（execution 的结果会改变下一步的 requirement），这是传统 workload 调度器不需要处理的维度。

## 16.2 与 Operating System 的类比

传统 OS 管理 Process / Memory / Filesystem / Device / Scheduler；RunWeave 管理 Task / Context / Resource / Node / Placement。长期看 RunWeave 更像一个 **Agent-oriented Distributed Operating Layer**，而不只是一个 Web Backend。

## 16.3 与 MCP、Sandbox、Browser Runtime 的关系

MCP 与 RunWeave 互补的关系已经在第 6 节详细展开。Sandbox 只是一种特殊 Node（`node: { type: ephemeral-cloud-sandbox }`），Browser Runtime 同理——未来 Cloud Sandbox / Laptop / GPU / Server 都可以统一落在同一个 Node 模型里，这是一个值得保留的干净抽象。

---

# 17. 长期演进路线（v0.1 之后）

```text
v0.1   Distributed Execution + MCP Exposure works.
v0.1.x Capability Scheduling / Resource Registry / Policy / Artifacts / Execution Trace 打磨。
v0.2   Task Graph / Concurrent Placement / Checkpoint / Recovery → Distributed Task Runtime works.
v0.3   Edge Agent / Local Agent Loop / Delegation → Hierarchical Agent Runtime works.
v0.4+  Peer Nodes / Node federation / Task migration / Enterprise policy → Agent Computing Fabric.
```

几个值得提前埋下伏笔、但不在 v0.1 展开的方向：

- **Task Migration**：用户不应该感知"session migration"，Task 始终是同一个 Task，真正发生的是 execution placement 的变化。真正意义上的 runtime migration（Agent State Checkpoint → Transfer → Resume）需要 Node 上运行局部 Agent Runtime，属于 v0.3+。
- **Edge Agent**：如果每一步 `run → observe → modify → run` 都要走 `GPU → Cloud → LLM → GPU`，延迟和成本会很大。未来 Cloud Agent 可以把目标 delegate 给 Edge Agent 做本地循环，形成 Hierarchical Distributed Agent Runtime——但这会引入 distributed context / state / agent identity / subagent recovery 等一整套新问题，v0.1 明确不做。

---

# 18. 工程哲学

## 18.1 Small enough to understand

RunWeave 应该延续 nanobot 的优点——不要因为"分布式系统"这个标签就立刻拆成几十个 service。v0.1 推荐：`1 Server Process + N Node Processes + SQLite + WebSocket`，足够。

## 18.2 Monolith First，Protocol First

Server 端（Agent / Task Manager / Scheduler / Registry / API / MCP 接口）都在一个进程里，不要拆成 `agent-service / scheduler-service / node-service` 这类微服务——那只会降低开发速度。但 `Server ↔ Node` 的 protocol 必须清晰，因为这才是 RunWeave 真正的系统边界，也是第 6 节 MCP 兼容能否成立的基础。

## 18.3 依赖方向

```text
Agent → Execution API → Placement → Node/Resource/Capability
Execution Router → Protocol → Node
MCP Server 接口 → Execution API（与 Agent 平级接入，不绕过 Placement）
```

## 18.4 第一阶段不要优化 Agent 智商

RunWeave 的独特价值不应该首先通过"更好的 coding benchmark"证明，而应该证明"同一个 Agent + 更好的 execution substrate"可以完成传统 Agent 很难自然完成的任务。即：**竞争力来自 Harness topology，而不仅是 model intelligence。**

## 18.5 最值得花时间研究的八个工程问题

Execution abstraction（怎样描述与 Node 无关的一次执行）、Capability model、Resource locality、Placement、Result abstraction（Node 该向 Agent 返回什么）、Failure model（Node 离线怎么办）、Security boundary（Cloud Agent 到底有什么权力）、State model（Task/Execution 如何持久化）——这八个问题比"Prompt 写什么"重要得多。

---

# 19. 核心术语与架构不变量

**统一术语**：Control Plane / Execution Plane、Agent / Task、Node / Capability / Resource、Execution / Placement、Observation / Artifact、Policy。尽量避免混用 machine / environment / workspace / remote computer / worker / client，除非特指。

**架构不变量（可直接写进 CONTRIBUTING）：**

1. Agent 不应该知道 physical node details。
2. Agent 不应该依赖 physical filesystem path。
3. Tool semantics 与 execution location 分离。
4. Node 保留最终执行授权权。
5. Task 生命周期不依赖 Interaction Session。
6. Execution 必须可观察。
7. Result 应尽量结构化，而不是 raw output。
8. Data 默认留在拥有它的 Node。

---

# 20. 命名决策：RunWeave

**结论已定，不再需要继续讨论命名**，这里只记录理由，避免团队后续反复纠结：

- "Fabric" 概念表达和简洁程度都很好，但与 Daniel Miessler 的开源项目 fabric（AI 提示词增强框架，GitHub 上有相当规模的社区）以及 Microsoft Fabric（数据分析平台）撞名，公开项目名不可用。
- **RunWeave** 保留了"编织分布式执行"的隐喻，强调 execution，不是典型的 Chat/Agent 命名，适合做 CLI（`runweave node` / `runweave task` / `runweave exec`）。
- Tagline：**"One agent. Many nodes. One execution fabric."** 或 **"Reason globally. Execute where the data lives."**
- "Fabric" 作为内部架构概念词（Execution Fabric）仍然可以保留使用，只是不再作为项目/品牌名。

---

# 21. 产品 Thesis 与 North Star

RunWeave 的世界观不是 "Every computer should have an AI agent"，而是：

> **Every computer can become an execution node of one agent.**
> **Your devices should not each host an isolated agent. Together, they should form the execution substrate of one persistent intelligence — accessible either directly, or through any MCP-compatible harness.**

**v0.1 一句话目标：**

> 让一个 Agent ——不管是 RunWeave 自己的 Agent Loop，还是通过 MCP 协议接入的第三方 Harness——能够发现两个具有不同 capability 和不同 data locality 的计算节点，并自动将 execution 放置到正确节点上。

如果这句话跑通（S1-S6 全部成立），RunWeave v0.1 就成功了。

```text
                    Global Reasoning
                  自己的 Agent  或  第三方 Harness (via MCP)
                           │
                         Task
                           │
                       Scheduler
             ┌─────────────┼─────────────┐
          Laptop         GPU PC         Cloud
             │             │             │
           Data          Compute        Sandbox
             └─────────────┼─────────────┘
                       Artifacts / Observations
                           ▼
                    回到发起调用的 Agent
```

**One Agent (or Any MCP-Compatible Harness). Many Nodes. One Execution Fabric.**

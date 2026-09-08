# Fabric v0.0.1 项目规划书

> **A distributed agent harness that turns your computers, servers and cloud environments into one execution fabric.**

**Version:** v0.0.1  
**Status:** Initial Architecture / MVP Planning  
**Project Codename:** Fabric  
**Category:** Distributed Agent Harness / Agent Runtime / Execution Fabric

---

# 0. Executive Summary

Fabric 不是另一个 Codex、Claude Code、Cline 或 Kimi Code。

Fabric 希望解决的问题不是：

> 如何让一个 Agent 在一台机器上更聪明地写代码？

而是：

> **如何让一个 Agent 将用户分散在不同设备、服务器、云环境和数据域中的计算能力，视为一个统一的执行环境？**

Fabric 的基本假设是：

现代用户真正拥有的计算环境已经天然是分布式的。

例如：

```text
Laptop
├── 当前工作目录
├── 浏览器
├── SSH credentials
├── IDE
└── 私有文件

Lab Workstation
├── GPU
├── SAR Dataset
├── Docker
└── 大容量磁盘

Cloud VM
├── 公网
├── 长时间任务
├── CI Environment
└── 可临时销毁

Production Server
├── Logs
├── Database
└── Internal Network

Mobile / Web
└── User Interaction
```

但今天的大多数 Agent Harness 仍然围绕：

```text
Agent
  ↓
Workspace
  ↓
Current Machine
```

设计。

这导致：

- 数据必须被移动到 Agent 所在环境；
- 用户频繁在人、CLI、IDE、Web Agent 之间切换上下文；
- Agent Session 与某台机器绑定；
- 多台设备之间缺乏统一资源模型；
- Agent 不理解“数据在哪里”；
- 用户必须亲自决定在哪里运行任务；
- 云 Agent 很难安全使用本地私有数据；
- 本地 Agent 又无法自然利用云端持续运行能力。

Fabric 希望改变这个基本模型。

Fabric 的核心思想是：

> **Reason globally, execute locally.**

Agent Loop 可以运行在云端。

真正的数据和计算则可以发生在任何适合的节点。

```text
Intent
   ↓
Cloud Agent
   ↓
Task
   ↓
Requirements
   ↓
Placement
   ↓
Execution Node
   ↓
Result / Artifact
   ↓
Cloud Brain
   ↓
Next Decision
```

Fabric 不把所有数据送给 Agent。

而是：

> **Move computation to data, not data to computation.**

执行完成之后：

> **Move information back to reasoning.**

最终：

> **Task → Execution Placement**

将成为 Fabric 最核心的 runtime primitive 之一。

---

# 1. 项目定位

## 1.1 Fabric 是什么

Fabric 是一个：

> **Distributed Agent Harness**

它允许一个中心 Agent Runtime：

- 感知多个 Execution Node；
- 感知每个 Node 的能力；
- 感知数据与资源所在位置；
- 根据 Task requirements 自动完成 execution placement；
- 将工具调用或任务派发到合适节点；
- 收集执行结果；
- 将结构化 observation 返回主 Agent Loop；
- 在多个节点之间连续推进同一个 Task。

因此：

```text
Laptop
GPU Workstation
Remote Server
Cloud VM
Sandbox
Browser Runtime
```

对于 Agent 而言不再是五个彼此独立的计算环境。

而成为：

```text
One Execution Fabric
```

---

# 2. Fabric 不是什么

Fabric v0.0.1 必须主动控制范围。

Fabric **不是：**

## 2.1 另一个 Coding Agent

Fabric 不以：

```text
better code generation
better diff generation
better repository indexing
better autocomplete
```

作为最主要竞争方向。

这些能力可以存在，但不是项目核心。

---

## 2.2 另一个 MCP 聚合器

Fabric 不等于：

```text
LLM
 ↓
50 MCP Servers
```

更多 Tool 并不会自然形成新的 Harness architecture。

---

## 2.3 一个万能 Personal Assistant

v0.0.1 不追求：

- 邮件；
- 日历；
- PPT；
- 智能家居；
- 社交媒体；
- Office Agent；
- Knowledge Agent。

Fabric 的宽度是：

> **Execution Width**

而不是：

> Application Width。

---

## 2.4 一个远程 Terminal Controller

如果最终架构只是：

```text
Cloud LLM
   ↓
SSH
   ↓
Remote Machine
```

那么 Fabric 没有成立。

Remote Tool Calling 只是实现机制。

Fabric 真正希望提供：

```text
Task
 ↓
Requirement
 ↓
Capability Resolution
 ↓
Data Locality Resolution
 ↓
Placement
 ↓
Execution
```

---

# 3. 核心 Thesis

Fabric v0.0.1 建立在以下五条原则之上。

## Principle 1

> **Reason globally.**

主要 Agent Loop、Task State、Context、Decision Making 位于统一控制平面。

## Principle 2

> **Execute locally.**

任务应该尽可能在拥有所需数据和能力的环境中执行。

## Principle 3

> **Move computation to data.**

默认情况下不要：

```text
Node Data
   ↓
Upload
   ↓
Cloud Agent
```

而应该：

```text
Cloud Task
   ↓
Node
   ↓
Local Computation
```

## Principle 4

> **Move information back to reasoning.**

Node 返回：

- observation；
- summary；
- metrics；
- structured result；
- artifact references；
- mutation information；

而不是无脑返回全部数据。

## Principle 5

> **Schedule by capability, locality and policy.**

用户描述目标。

Agent 描述 Requirement。

Runtime 决定 Placement。

---

# 4. Fabric 的核心抽象

Fabric 第一版应该围绕以下九个实体设计：

```text
Agent
Task
Node
Capability
Resource
Execution
Placement
Artifact
Policy
```

而不是传统 Coding Agent 的：

```text
Chat
Session
Workspace
Terminal
```

---

# 5. Agent

Agent 是负责：

```text
Reasoning
Planning
Tool Selection
Task Progression
Observation Interpretation
```

的逻辑实体。

v0.0.1：

```text
One Primary Agent
```

即可。

暂时不要 Multi-Agent。

Agent Loop 可以直接借鉴 nanobot 的极简思想：

```text
Input
 ↓
Context
 ↓
LLM
 ↓
Tool Call
 ↓
Observation
 ↓
LLM
 ↓
...
 ↓
Final
```

Fabric 并不需要重新发明 Agent Loop。

Fabric 真正新增的是：

```text
Agent Loop
     ↓
Execution Abstraction
```

---

# 6. Task

Task 表示：

> 用户希望系统持续完成的一个目标。

例如：

```text
Run tests for nanobot and investigate failures.
```

Task 不等于 Chat Message。

Task 应该拥有独立生命周期：

```text
CREATED
  ↓
PLANNING
  ↓
RUNNING
  ↓
WAITING
  ↓
RUNNING
  ↓
COMPLETED
```

以及：

```text
FAILED
CANCELLED
BLOCKED
```

Task 与：

```text
Browser
Laptop
Cloud
```

均不绑定。

因此：

> **Task ≠ Session**

也：

> **Task ≠ Machine**

---

# 7. Node

Node 是 Fabric 中能够提供 interaction 或 execution capability 的实体。

## 7.1 Execution Node

例如：

```text
Laptop
Desktop
Lab GPU Server
Cloud VM
Cloud Sandbox
Production Server
Browser Runtime
```

Node 可以提供：

```text
filesystem
shell
git
docker
cuda
browser
database
network
```

等能力。

---

# 8. Interaction Node

不是所有 Node 都负责执行。

例如：

```text
Web
Phone
CLI
IDE
Slack
```

可能主要负责：

```text
input
approval
notification
monitoring
result presentation
```

因此：

```text
Node
├── Interaction Node
├── Execution Node
└── Hybrid Node
```

一个 Laptop 可以同时承担：

```text
Interaction
+
Execution
```

两种角色。

---

# 9. Capability

Capability 是 Fabric 最重要的一等公民之一。

Node 不应该仅仅表示：

```text
192.168.1.20
```

而应该表示：

```yaml
node:
  id: lab-pc

  capabilities:
    shell: true
    filesystem: true
    git: true
    docker: true
    cuda: true

  hardware:
    gpu:
      model: RTX-5060
      vram: 8GB

  os:
    type: linux
```

Agent 应该尽量避免：

```text
run command on lab-pc
```

而表达：

```text
I require CUDA + dataset://sar/gf3
```

由 Fabric 决定：

```text
→ lab-pc
```

---

# 10. Resource

Capability 回答：

> 这个 Node 能做什么？

Resource 回答：

> 这个 Node 拥有什么？

例如：

```text
repo://nanobot

dataset://sar/gf3

logs://production/api

database://research-postgres

service://internal-model
```

---

# 11. Data Locality

Fabric 必须把数据位置当成一等公民。

例如：

```yaml
resources:

  repo://nanobot:
    locations:
      - laptop
      - lab-pc

  dataset://sar/gf3:
    locations:
      - lab-pc

  logs://prod:
    locations:
      - production-server
```

于是用户不再必须告诉 Agent：

```text
D:\research\SAR\data\GF3
```

Agent 可以操作逻辑资源：

```text
dataset://sar/gf3
```

Fabric Runtime 决定：

```text
physical path
+
node
```

---

# 12. Execution

Execution 表示：

> Task 在某个 Node 上发生的一次具体执行。

例如：

```text
Task
  ↓
Execution #12
  ↓
lab-pc
  ↓
pytest
```

Execution 必须独立记录：

```text
execution_id
task_id
node_id
command/tool
started_at
finished_at
status
result
artifacts
mutations
```

---

# 13. Placement

Placement 是：

> 将 Execution 分配给某个 Node 的过程。

这是 Fabric 与普通 Agent Harness 最重要的区别之一。

传统模式：

```text
tool_call
 ↓
current environment
```

Fabric：

```text
tool_call
 ↓
requirements
 ↓
placement
 ↓
best node
 ↓
execution
```

---

# 14. Placement Requirement

一个 execution request 可以表达：

```yaml
requirements:

  capabilities:
    - shell
    - python
    - cuda

  resources:
    - dataset://sar/gf3

  constraints:
    os: linux
```

然后 Scheduler 查询：

```text
Node Registry
```

寻找候选 Node。

---

# 15. Placement Algorithm v0.0.1

第一版不要做复杂 AI Scheduler。

采用 deterministic filtering 即可。

流程：

```text
Execution Request
       ↓
Capability Match
       ↓
Resource Locality
       ↓
Policy Check
       ↓
Availability
       ↓
Placement
```

伪代码：

```python
candidates = nodes

candidates = match_capabilities(candidates)
candidates = match_resources(candidates)
candidates = enforce_policy(candidates)
candidates = filter_online(candidates)

node = select_best(candidates)
```

v0.0.1 可以优先：

```text
Resource Locality > Capability Match > User Preference
```

后续再增加：

```text
latency
cost
load
energy
network
security
```

等 scoring。

---

# 16. Future Placement Model

后续可以演化为：

```text
score(node) =

w1 * capability_match
+
w2 * data_locality
+
w3 * security
+
w4 * latency
+
w5 * cost
+
w6 * availability
+
w7 * load
```

但：

**v0.0.1 不实现复杂评分。**

---

# 17. Artifact

Fabric 不应该让所有执行结果直接塞回 LLM Context。

Execution 应产生：

```text
Observation
+
Artifact
```

例如：

```yaml
result:

  status: failed

  summary:
    pytest failed with 3 tests

  observations:
    failed_tests: 3

  artifacts:
    - artifact://execution/123/test.log
    - artifact://execution/123/report.xml

  mutations:
    - src/runtime.py
```

Agent 可以先看到：

```text
summary
```

需要深入时再访问：

```text
artifact
```

---

# 18. Fabric Result Model

推荐统一结果：

```python
ExecutionResult:
    execution_id
    status
    summary
    observations
    artifacts
    mutations
    error
```

这将比简单 stdout/string 强很多。

---

# 19. 为什么 Result 必须结构化

否则远程执行会迅速退化成：

```text
LLM
 ↓
ssh command
 ↓
20000 token stdout
 ↓
LLM
```

Fabric 应该逐渐实现：

```text
Raw Execution
     ↓
Observation Extraction
     ↓
Structured Result
     ↓
Agent
```

因此：

> **Execution output ≠ Agent observation**

这个区分值得从第一版就保留。

---

# 20. Policy

远程 Agent 最大的问题不是：

> 能不能运行命令。

而是：

> 应不应该允许它运行。

Fabric 必须把 Policy 放在 execution path 上。

例如：

```yaml
policy:

  shell:
    allow:
      - git
      - python
      - pytest

    require_approval:
      - rm
      - docker
      - sudo

    deny:
      - shutdown
```

完整链路：

```text
Agent
 ↓
Execution Request
 ↓
Placement
 ↓
Policy
 ↓
Approval?
 ↓
Execution
```

---

# 21. Trust Boundary

Fabric 的核心安全原则：

> Cloud Brain 默认不拥有 Node 全部权限。

Node 自己是最终授权方。

即：

```text
Cloud
  ↓ request
Node Policy Engine
  ↓
allow / deny
```

而不是：

```text
Cloud has root
```

这是未来项目可信度的重要基础。

---

# 22. 整体架构

v0.0.1 推荐：

```text
                         FABRIC SERVER

┌──────────────────────────────────────────────────────┐
│                                                      │
│                    Agent Runtime                     │
│                        │                             │
│                   Main AgentLoop                     │
│                        │                             │
│                       Task                           │
│                        │                             │
│                Execution Request                     │
│                        │                             │
│               Capability Resolver                    │
│                        │                             │
│               Placement Scheduler                    │
│                        │                             │
│                 Execution Router                     │
│                        │                             │
└────────────────────────┼─────────────────────────────┘
                         │
                Fabric Protocol
                         │
            ┌────────────┼────────────┐
            │            │            │
            ▼            ▼            ▼

       Laptop Node    Lab Node     Cloud Node
            │            │            │
         Executor      Executor     Executor
            │            │            │
       filesystem       CUDA        sandbox
       shell            data        shell
       git              docker      browser
```

---

# 23. v0.0.1 三大组件

项目最初只需要三个核心模块。

## 23.1 fabric-server

职责：

```text
Agent Loop
Task State
Node Registry
Capability Registry
Placement
Execution Routing
Result Collection
Web/API
```

## 23.2 fabric-node

运行在：

```text
Laptop
Desktop
Server
VM
```

上的 lightweight daemon。

负责：

```text
connect
heartbeat
capability advertise
resource advertise
receive execution
policy enforcement
local execution
artifact collection
result return
```

## 23.3 fabric-protocol

定义：

```text
Node registration
Heartbeat
Capability advertisement
Execution request
Execution event
Execution result
Artifact reference
Approval
Cancellation
```

---

# 24. 推荐协议模型

第一版不要发明复杂 wire protocol。

推荐：

```text
WebSocket
+
JSON Messages
```

原因：

- 双向；
- 简单；
- Debug 方便；
- Node 主动连接 Server；
- 不要求用户开放本地端口；
- 后续容易迁移 protobuf/gRPC。

连接方式：

```text
Node
 ↓
outbound WebSocket
 ↓
Fabric Server
```

非常重要：

**Server 不主动访问用户局域网。**

Node 主动建立 persistent connection。

---

# 25. Node Registration

例如：

```json
{
  "type": "node.register",
  "node_id": "lab-pc",
  "name": "Lab PC",
  "capabilities": [
    "shell",
    "filesystem",
    "git",
    "docker",
    "cuda"
  ]
}
```

Server 保存：

```text
NodeRegistry
```

---

# 26. Heartbeat

Node 周期发送：

```json
{
  "type": "node.heartbeat",
  "node_id": "lab-pc",
  "status": "idle"
}
```

Server 状态：

```text
ONLINE
OFFLINE
BUSY
UNKNOWN
```

---

# 27. Execution Request

Server：

```json
{
  "type": "execution.request",
  "execution_id": "exec_001",
  "tool": "shell",
  "arguments": {
    "command": "pytest"
  }
}
```

Node：

```text
Policy Check
 ↓
Executor
```

---

# 28. Execution Events

Node 应支持 streaming events：

```text
execution.started

execution.stdout

execution.stderr

execution.artifact

execution.completed

execution.failed
```

这样 Web UI 可以实时展示。

---

# 29. Tool Abstraction

nanobot 原始模式可能类似：

```python
tool.run(...)
```

Fabric 应增加：

```text
Logical Tool
     ↓
Execution Backend
```

例如：

```text
shell
```

不是一个具体 shell。

而是：

```text
shell capability
```

不同节点均可以提供。

因此：

```text
ShellTool
   ↓
ExecutionRouter
   ↓
NodeShellExecutor
```

---

# 30. Execution Context

Tool call 不能只包含参数。

应逐渐拥有：

```text
ExecutionContext
```

例如：

```python
ExecutionContext(
    task_id="task_001",
    requirements=["shell", "git"],
    resources=["repo://nanobot"],
    preferred_node=None,
)
```

然后：

```text
Execution Router
```

自动 placement。

---

# 31. v0.0.1 的第一个 Killer Demo

不要第一版就做宏伟场景。

最应该做：

> **Cloud Agent 操作另一台电脑上的 Git Repository。**

完整流程：

```text
User Browser
     ↓
Fabric Server
     ↓
Agent Loop
     ↓
"I need to inspect repo"
     ↓
resource://nanobot
     ↓
Placement
     ↓
Laptop Node
     ↓
read files
     ↓
Observation
     ↓
Cloud Agent
     ↓
run pytest
     ↓
Laptop Node
     ↓
Result
     ↓
Cloud Agent
     ↓
Explain failure
```

这就是第一个 End-to-End Milestone。

---

# 32. 第二个 Killer Demo

增加第二台 Execution Node：

```text
Laptop
+
GPU Workstation
```

例如：

```text
Laptop:
repo://sar

GPU:
repo://sar
dataset://gf3
cuda
```

用户：

```text
Run the SAR training experiment.
```

Fabric 自动：

```text
Requirement:
dataset://gf3
cuda

Placement:
GPU
```

这时：

> **Capability-based Execution**

第一次真正成立。

---

# 33. 第三个 Killer Demo

一个 Task 使用两个 Node。

例如：

```text
Task:
Analyze experiment failure and modify code.
```

执行：

```text
Cloud
 ↓
inspect source → Laptop
 ↓
inspect dataset → GPU
 ↓
reason
 ↓
modify source → Laptop
 ↓
validate → GPU
```

这时：

> **Task ≠ Machine**

真正成立。

---

# 34. Task Graph

v0.0.1 不需要完整 DAG Scheduler。

但内部数据结构应该避免设计死。

未来：

```text
Task
 ↓
SubTask
 ↓
Execution
```

可以升级为：

```text
Task Graph
```

例如：

```text
                    Task
                      │
             ┌────────┴────────┐
             │                 │
       Analyze Code       Analyze Dataset
          Laptop              GPU
             │                 │
             └────────┬────────┘
                      │
                    Patch
                      │
                  Validation
```

未来 Fabric 更准确的模型会从：

> Task → Execution Placement

演化为：

> **Task Graph → Distributed Execution Plan**

---

# 35. Main Agent Loop

第一版坚持：

```text
One Cloud Agent Loop
```

Node 不运行 Agent。

Node 是：

```text
Deterministic Executor
```

原因：

1. 更容易 debug；
2. Context 集中；
3. 状态管理简单；
4. 行为一致；
5. Observability 简单；
6. 不需要处理 Agent-to-Agent protocol。

---

# 36. 为什么 v0.0.1 不应该做 Edge Agent

未来可能：

```text
Cloud Coordinator
       ↓
Local Agent
       ↓
local iterative loop
```

优点：

```text
less latency
less token round trip
local privacy
long execution autonomy
```

但会引入：

```text
distributed context
distributed state
delegation
agent identity
subagent recovery
conflict
consistency
```

因此属于 v0.1+。

---

# 37. Session 与 Task

Fabric 必须从第一天就区分：

```text
Conversation
```

和：

```text
Task
```

Conversation 是 UI 概念。

Task 是 runtime 概念。

用户可能：

```text
Web
 ↓
create Task A
```

关闭网页。

之后：

```text
Phone
 ↓
inspect Task A
```

任务依然存在。

因此：

> **Session ≠ Task**

---

# 38. Node 与 Task

Task 不归属于任何 Node。

Node 只是执行资源。

```text
Task
├── Execution 1 → Laptop
├── Execution 2 → Cloud
├── Execution 3 → GPU
└── Execution 4 → Laptop
```

因此：

> **Task ≠ Node**

---

# 39. Agent 与 Node

Agent 本身也不属于 Node。

至少在 Fabric 的逻辑模型里：

```text
Agent
```

存在于：

```text
Control Plane
```

Node 属于：

```text
Execution Plane
```

因此：

> **Agent ≠ Machine**

---

# 40. Control Plane / Execution Plane

这是 Fabric 非常重要的架构边界。

## Control Plane

负责：

```text
Agent
Task
Scheduling
State
Policy Definition
Node Registry
Resource Registry
Observation
```

## Execution Plane

负责：

```text
filesystem
shell
git
docker
GPU
browser
database
```

完整：

```text
              CONTROL PLANE

          Fabric Server
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

---

# 41. Data Plane

后续甚至可以进一步区分：

```text
Control Plane
Execution Plane
Data Plane
```

因为 artifact/data 不一定需要穿过 Agent Server。

例如以后：

```text
GPU Node
   ↓
large artifact
   ↓
Object Storage
```

而 Cloud Brain 只得到：

```text
artifact://123
```

但 v0.0.1 暂时不展开。

---

# 42. Storage

v0.0.1 推荐：

```text
SQLite
```

保存：

```text
tasks
nodes
executions
resources
artifacts
events
```

不要一开始 PostgreSQL。

原因：

Fabric v0.0.1 目标应该是：

> 一个开发者 clone 后 5 分钟启动。

---

# 43. Event Model

强烈建议从 v0.0.1 开始所有重要状态变化都产生 Event。

例如：

```text
task.created
task.started

node.connected
node.disconnected

execution.created
execution.started
execution.output
execution.completed

artifact.created
```

未来：

```text
Observability
Replay
Audit
Debug
WebSocket UI
```

全部可以建立在 Event 上。

---

# 44. Event Bus

如果 nanobot 已有 Message Bus/Event Bus 思想，可以复用。

初期：

```text
In-Memory EventBus
```

即可。

未来：

```text
Redis
NATS
Kafka
```

都不应该进入 v0.0.1。

---

# 45. Observability

Harness 项目如果无法追踪：

> Agent 为什么决定在这台机器执行？

那么很难调试。

因此执行 Trace 应至少包含：

```text
Task
 ↓
Agent Decision
 ↓
Execution Request
 ↓
Placement
 ↓
Node
 ↓
Tool
 ↓
Result
```

Web UI 最终可以显示：

```text
Task #17

12:41 Agent requested shell
12:41 Required: git + repo://nanobot
12:41 Candidate nodes: laptop, lab-pc
12:41 Selected: laptop
12:41 Execution #51 started
12:41 git status
12:41 Execution #51 completed
12:41 Result returned to Agent
```

这会成为非常好的 developer experience。

---

# 46. v0.0.1 UI

Web UI 不需要漂亮。

第一版只需要四个页面。

## Chat / Task

```text
Prompt
Task Status
Agent Output
```

## Nodes

例如：

```text
● kail-laptop

  online

  shell
  filesystem
  git

● lab-gpu

  online

  shell
  filesystem
  cuda
  docker
```

## Executions

```text
exec_001
task_001
node: lab-gpu
tool: shell
status: completed
```

## Resources

```text
repo://nanobot
→ kail-laptop

dataset://sar/gf3
→ lab-gpu
```

这已经足够展示核心思想。

---

# 47. CLI

第一版 CLI：

```bash
fabric server
```

启动 Server。

Node：

```bash
fabric node connect <server>
```

或者：

```bash
fabric-node connect <server>
```

查看：

```bash
fabric node list
```

未来才增加：

```bash
fabric task
fabric run
fabric exec
```

---

# 48. Node Enrollment

第一版可以简单采用 token：

Server：

```bash
fabric node-token create
```

生成：

```text
fab_xxxxxxxxx
```

Node：

```bash
fabric node connect \
  --server https://fabric.example.com \
  --token fab_xxxxxxxxx
```

然后保存 credential。

---

# 49. Node Identity

每个 Node 拥有：

```text
node_id
name
token
capabilities
resources
metadata
```

例如：

```yaml
id: node_01
name: lab-gpu

platform:
  os: linux
  arch: x86_64

capabilities:
  - shell
  - filesystem
  - git
  - docker
  - cuda
```

---

# 50. Resource Registration

第一版 resource 可以手动配置：

```yaml
resources:

  - uri: repo://nanobot
    path: /home/kail/projects/nanobot

  - uri: dataset://sar/gf3
    path: /data/GF3
```

Fabric Node 启动后向 Server advertise。

暂时不要自动扫描用户文件。

这同时减少隐私风险。

---

# 51. Filesystem Safety

Agent 不应该看到整台电脑。

Node 配置：

```yaml
filesystem:

  roots:
    - /home/kail/projects
    - /data/sar
```

所有 filesystem operation 必须限制在 roots 内。

这是非常重要的默认安全设计。

---

# 52. Shell Safety

v0.0.1：

```text
shell capability
```

需要：

```text
working_directory
timeout
environment
output_limit
```

例如：

```json
{
  "command": "pytest",
  "cwd": "repo://nanobot",
  "timeout": 300
}
```

不要直接把 physical path 暴露给 Cloud Agent。

Node 自己解析：

```text
repo://nanobot
 ↓
/home/kail/projects/nanobot
```

---

# 53. Logical Resource URI

这是我非常建议保留的设计。

统一采用：

```text
repo://
dataset://
logs://
artifact://
service://
```

未来甚至：

```text
device://
database://
secret://
```

LLM 操作 logical resource。

Runtime 处理 physical locality。

这能够显著减少：

```text
OS path
machine path
environment coupling
```

---

# 54. Artifact Model

Artifact 可以包括：

```text
file
log
image
json
patch
report
model checkpoint
```

例如：

```text
artifact://exec_100/loss.png
```

Metadata：

```yaml
artifact:
  id: art_123
  execution: exec_100
  node: lab-gpu
  type: image
  mime: image/png
  size: 238192
```

第一版 artifact 可以仍然通过 Server 传输。

以后再优化 locality/storage。

---

# 55. Mutation Tracking

Coding Agent 一个非常关键的安全问题是：

> 到底改了什么？

因此 Execution Result 可以包含：

```text
mutations
```

例如：

```yaml
mutations:

  files_modified:
    - src/agent/runtime.py

  files_created:
    - tmp/output.json
```

未来可以：

```text
rollback
approval
diff
```

---

# 56. Human Approval

v0.0.1 可以只实现最简单的：

```text
ALLOW
DENY
ASK
```

例如：

```text
read_file → ALLOW

pytest → ALLOW

git commit → ASK

git push → ASK

rm → ASK

sudo → DENY
```

Node 收到 ASK 时：

```text
Execution
 ↓
WAITING_APPROVAL
```

然后通知 Interaction Node。

---

# 57. 手机真正应该是什么

手机不要被强行定义成计算 Node。

手机最大的价值可能是：

```text
Approval Node
Notification Node
Observation Node
```

例如：

```text
Agent:
Need permission to push branch fix/runtime.

Phone:
[Allow]
[Deny]
```

所以：

```text
phone
```

是 Fabric 的一部分。

但它不一定运行 Agent。

---

# 58. Web 真正应该是什么

Web 是：

```text
Control Interface
```

而不是：

```text
Agent Runtime
```

用户可以：

```text
create task
inspect node
approve action
inspect artifact
watch trace
```

Web 关闭：

Task 不消失。

---

# 59. IDE 真正应该是什么

未来 VS Code Extension 也只是：

```text
Interaction Node
+
Local Context Provider
```

而不是整个 Harness。

例如：

```text
VS Code
 ↓
selected code
 ↓
Fabric Task
```

Agent 可以随后将 validation dispatch 给 GPU server。

---

# 60. MVP 用户故事

## Story A

用户：

```text
I have nanobot on my laptop.
```

连接：

```text
fabric node connect
```

Web：

```text
Analyze the architecture of repo://nanobot.
```

Agent 远程读取代码并回答。

## Story B

用户：

```text
Run pytest and diagnose failures.
```

Cloud Agent：

```text
requires:
shell
repo://nanobot
```

Fabric：

```text
placement → laptop
```

执行并返回。

## Story C

拥有第二台：

```text
lab-gpu
```

用户：

```text
Run training using dataset://sar/gf3.
```

系统选择：

```text
lab-gpu
```

而不是 laptop。

## Story D

一个任务跨机器：

```text
Analyze SAR dataset, modify code and rerun training.
```

Fabric：

```text
inspect dataset
→ lab-gpu

inspect source
→ laptop

modify source
→ laptop

validation
→ lab-gpu
```

---

# 61. v0.0.1 Success Criteria

Fabric v0.0.1 成功不看：

```text
多少模型
多少工具
多少 MCP
多少 Agent
```

而看下面五件事情。

### S1

Cloud Agent 可以可靠调用远程 Node。

### S2

两个 Node 可以同时连接。

### S3

每个 Node 能 advertise 不同 Capability。

### S4

Execution 可以根据 Capability 自动 placement。

### S5

Execution 可以根据 Resource locality 自动 placement。

如果这五个成立：

> Fabric 的核心 thesis 已经得到验证。

---

# 62. v0.0.1 明确不做

这是规划中最重要的一部分。

v0.0.1：

**不做 Multi-Agent。**

**不做 DAG Scheduler。**

**不做 Kubernetes。**

**不做 P2P。**

**不做 Agent-to-Agent Protocol。**

**不做自动迁移文件。**

**不做 GPU Scheduler。**

**不做 Container Orchestration。**

**不做 MCP Marketplace。**

**不做 IDE Extension。**

**不做 Mobile App。**

**不做完整 Browser Agent。**

**不做 Vector Database。**

**不做复杂 Memory。**

**不做 RAG Platform。**

**不做 Multi-Tenant SaaS。**

**不做 Enterprise RBAC。**

**不做复杂 Cost Scheduler。**

**不做 Local LLM。**

这会让 Fabric 保持 nanobot 式的 architecture clarity。

---

# 63. 推荐代码结构

如果基于 nanobot 演进：

```text
fabric/

├── agent/
│   ├── loop.py
│   ├── context.py
│   └── tools.py
│
├── tasks/
│   ├── model.py
│   ├── manager.py
│   └── state.py
│
├── nodes/
│   ├── model.py
│   ├── registry.py
│   └── heartbeat.py
│
├── capabilities/
│   ├── model.py
│   └── registry.py
│
├── resources/
│   ├── model.py
│   ├── registry.py
│   └── resolver.py
│
├── placement/
│   ├── requirements.py
│   ├── resolver.py
│   └── scheduler.py
│
├── execution/
│   ├── request.py
│   ├── result.py
│   ├── router.py
│   └── manager.py
│
├── artifacts/
│   ├── model.py
│   └── store.py
│
├── policy/
│   ├── engine.py
│   └── rules.py
│
├── protocol/
│   ├── messages.py
│   ├── websocket.py
│   └── events.py
│
├── server/
│   ├── app.py
│   └── api.py
│
├── node/
│   ├── daemon.py
│   ├── client.py
│   ├── executor.py
│   └── tools/
│       ├── shell.py
│       ├── filesystem.py
│       └── git.py
│
└── cli/
    └── main.py
```

---

# 64. 更重要的依赖方向

核心 dependency direction 建议：

```text
Agent
  ↓
Execution API

Execution API
  ↓
Placement

Placement
  ↓
Node / Resource / Capability

Execution Router
  ↓
Protocol

Protocol
  ↓
Node
```

Agent 不应该知道：

```text
WebSocket
SSH
IP
OS Path
```

Agent 只应该理解：

```text
Capability
Resource
Execution
Observation
```

这会决定项目以后能否保持干净。

---

# 65. Tool 与 Runtime 解耦

错误设计：

```python
class ReadFileTool:
    def run(self):
        open("/home/...")
```

推荐：

```text
ReadFile Tool
      ↓
Execution Request
      ↓
Runtime
      ↓
Node
```

即：

```text
Tool Semantics
≠
Tool Execution Location
```

这是 Fabric 最应该守住的架构边界之一。

---

# 66. Local Tool

不是所有工具都需要 remote。

可以存在：

```text
ExecutionTarget:
LOCAL_CONTROL_PLANE
NODE
```

例如：

```text
memory lookup
task management
```

在 Control Plane。

而：

```text
filesystem
shell
git
```

在 Execution Node。

---

# 67. Node Capability Discovery

v0.0.1 可以自动 detect：

```text
OS
CPU
git
python
docker
cuda
```

然后结合用户配置：

```text
filesystem roots
resources
permissions
```

形成最终 advertise。

---

# 68. Capability Schema

建议 Capability 不要只是 string。

内部逐渐允许：

```yaml
capability:

  name: cuda

  attributes:
    version: "13"
    devices: 1
    memory_mb: 8192
```

这样未来：

```text
cuda.memory >= 16000
```

可以成为 Requirement。

但 v0.0.1 API 可以暂时 string 化。

---

# 69. Execution Lifecycle

推荐：

```text
CREATED
  ↓
PLACED
  ↓
DISPATCHED
  ↓
RUNNING
  ↓
COMPLETED
```

异常：

```text
FAILED
CANCELLED
REJECTED
TIMEOUT
NODE_LOST
```

Approval：

```text
WAITING_APPROVAL
```

---

# 70. Failure Model

Distributed Runtime 最大区别之一：

> Node 会消失。

因此即使 v0.0.1 不实现 recovery，也必须承认：

```text
NODE_LOST
```

是一种正常 Execution failure。

不要假设：

```text
tool call always returns
```

---

# 71. Cancellation

Fabric Protocol 第一版就最好支持：

```text
execution.cancel
```

因为用户可能启动：

```text
pytest
training
download
```

随后取消。

Node executor 必须保留 process handle。

---

# 72. Timeout

任何远程 execution 必须有：

```text
timeout
```

否则一个 tool call 可以永远阻塞 Agent Loop。

---

# 73. Concurrency

v0.0.1：

每 Node：

```text
max_concurrent_executions = 1
```

即可。

不要过早设计复杂 worker pool。

之后再提升。

---

# 74. Persistence

至少保存：

```text
Task
Execution
Node metadata
Resource metadata
Artifact metadata
Event
```

Node 在线状态可以是 ephemeral。

---

# 75. Agent Context

不要将所有 Execution history 放进 prompt。

Agent Context 应包含：

```text
Task Goal
Recent Decisions
Relevant Observations
Artifact references
Current State
```

Execution logs 存在外部。

这能保持：

```text
long-horizon task
```

更可扩展。

---

# 76. Fabric 的真正产品对象

传统 Coding Agent：

```text
User
 ↓
Chat
 ↓
Agent
 ↓
Workspace
```

Fabric：

```text
User
 ↓
Task
 ↓
Agent
 ↓
Execution Fabric
```

这就是产品模型变化。

---

# 77. Fabric Dashboard 的核心应该是什么

未来 Dashboard 不应该主要长得像 ChatGPT。

更应该像：

```text
Tasks
Nodes
Resources
Executions
Artifacts
Trace
```

Chat 只是 Task 的一种入口。

---

# 78. 长期架构演进

## v0.0.1

```text
Cloud Brain
+
Remote Execution Node
```

目标：

> Distributed Execution works.

## v0.0.x

```text
Capability Scheduling
Resource Registry
Policy
Artifacts
Execution Trace
```

目标：

> Execution Fabric works.

## v0.1

```text
Task Graph
Concurrent Placement
Checkpoint
Recovery
```

目标：

> Distributed Task Runtime works.

## v0.2

```text
Edge Agent
Local Agent Loop
Delegation
```

目标：

> Hierarchical Agent Runtime works.

## v0.3+

可能：

```text
Peer Nodes
Dynamic discovery
Node federation
Task migration
Multi-agent
Enterprise policy
```

目标：

> Agent Computing Fabric.

---

# 79. Task Migration

这是长期非常值得追求的 feature。

Task：

```text
Cloud
 ↓
analysis
```

发现需要本地数据：

```text
Laptop
 ↓
execution
```

然后：

```text
Cloud
 ↓
reasoning
```

又需要 GPU：

```text
GPU
 ↓
validation
```

用户不应该感知：

```text
session migration
```

Task 始终是同一个 Task。

真正发生的是：

```text
Execution Placement changes.
```

---

# 80. 真正的 Task Migration

更远期：

Node 上可以运行局部 Agent Runtime。

此时才可能存在：

```text
Agent State Checkpoint
 ↓
Transfer
 ↓
Resume
```

这属于真正意义上的 runtime migration。

v0.0.1 暂时只做：

> Execution Migration。

---

# 81. Edge Agent

未来某些任务：

```text
run
observe
modify
run
observe
```

如果每一步都：

```text
GPU
→ Cloud
→ LLM
→ GPU
```

延迟与成本都很大。

因此未来：

```text
Cloud Agent
 ↓
delegate goal
 ↓
Edge Agent
 ↓
local loop
 ↓
result
```

形成：

> **Hierarchical Distributed Agent Runtime**

这是 Fabric 很自然的长期方向。

---

# 82. Fabric 与 Kubernetes 的类比

Kubernetes：

```text
Workload
 ↓
Resource Requirement
 ↓
Scheduler
 ↓
Node
```

Fabric：

```text
Agent Task
 ↓
Capability Requirement
 ↓
Placement Scheduler
 ↓
Execution Node
```

区别是：

Kubernetes 主要调度：

```text
containerized workload
```

Fabric 调度：

```text
Agent-generated execution
```

并且拥有：

```text
data locality
human approval
LLM reasoning
agent observation
```

作为额外维度。

---

# 83. Fabric 与 Operating System 的类比

传统 OS：

```text
Process
Memory
Filesystem
Device
Scheduler
```

Fabric：

```text
Task
Context
Resource
Node
Placement
```

因此长期看 Fabric 更像：

> **Agent-oriented Distributed Operating Layer**

而不只是 Web Backend。

---

# 84. Fabric 与 MCP 的关系

MCP 可以成为 Fabric Tool Interface 的一种来源。

但：

```text
MCP
```

回答：

> 有什么工具？

Fabric：

> 这个任务应该在哪里执行？

所以：

```text
MCP = Capability Interface

Fabric = Execution Runtime
```

二者互补，不竞争。

---

# 85. Fabric 与 Sandbox 的关系

Sandbox 只是特殊 Node。

例如：

```text
node:
  type: ephemeral-cloud-sandbox
```

于是未来：

```text
Cloud Sandbox
Laptop
GPU
Server
```

全部属于统一 Node model。

这是一个非常漂亮的统一抽象。

---

# 86. Fabric 与 Browser Runtime

Browser 也可以视为：

```text
Capability:
browser
```

来自：

```text
Laptop Browser
Cloud Browser
Mobile Browser
```

然后根据：

```text
cookie
network
security
locality
```

决定 placement。

---

# 87. Fabric 与 Secret

未来 Secret 也应该遵守：

> secret locality。

例如：

```text
secret://company-vpn
```

只存在：

```text
laptop
```

Agent 不获得 secret value。

只知道：

```text
resource available on laptop
```

于是需要此 secret 的 computation 自动在 laptop 执行。

这将是：

> Move computation to credentials.

也是非常重要的安全方向。

---

# 88. Fabric 的 Privacy Story

用户的数据：

```text
Private Dataset
Source Code
Credentials
Logs
```

可以保持在：

```text
User-controlled Node
```

Cloud Agent 只获得任务需要的：

```text
Observation
```

这不是：

> 上传数据给云 Agent。

而是：

> 云 Agent 向数据位置派发 computation。

这个价值主张非常强。

---

# 89. Fabric 的最终 UX

理想状态：

用户：

```text
Find why my latest SAR experiment failed,
fix the implementation,
rerun the experiment,
and prepare the patch.
```

Fabric：

```text
Cloud:
understand task

Laptop:
inspect source

Lab GPU:
inspect dataset + reproduce

Cloud:
reason

Laptop:
modify source

Lab GPU:
validate

Cloud:
summarize
```

用户完全不需要：

```text
switch terminal
scp data
ssh server
copy prompt
open another agent
```

---

# 90. 开发阶段规划

## Phase 0 — Kernel Extraction

目标：

保留 nanobot-style Agent Kernel。

完成：

```text
AgentLoop
Model abstraction
Tool abstraction
Context
EventBus
```

重点：

Agent Loop 不依赖 Node。

## Phase 1 — Node Connectivity

实现：

```text
fabric-server
fabric-node
WebSocket
register
heartbeat
```

验收：

```text
Server can see Node online.
```

## Phase 2 — Remote Execution

实现：

```text
shell capability
execution request
stdout streaming
result
timeout
cancel
```

验收：

```text
Cloud Agent can execute `pwd` remotely.
```

## Phase 3 — Filesystem

实现：

```text
read_file
write_file
list_dir
filesystem roots
```

验收：

```text
Agent can analyze remote repository.
```

## Phase 4 — Resource Registry

实现：

```text
repo://
dataset://
resource advertise
```

验收：

```text
Agent does not need physical path.
```

## Phase 5 — Capability Placement

两台 Node。

例如：

```text
Node A:
shell
git

Node B:
shell
git
cuda
```

Task requiring CUDA：

```text
→ Node B
```

## Phase 6 — Data Locality Placement

例如：

```text
dataset://gf3
```

只在：

```text
Node B
```

Task requiring GF3：

```text
→ Node B
```

验收：

> Move computation to data 成立。

## Phase 7 — Multi-Node Task

Agent 同一 Task：

```text
Node A → inspect source
Node B → execute training
Node A → patch
```

验收：

> Task ≠ Machine 成立。

---

# 91. v0.0.1 最终 Demo

建议录制一个 1～2 分钟 Demo。

画面：

```text
Browser
+
Laptop terminal
+
GPU machine terminal
```

Web：

```text
Connected Nodes

● kail-laptop
  shell
  filesystem
  git

● lab-gpu
  shell
  filesystem
  git
  cuda
  dataset://sar/gf3
```

Prompt：

```text
Inspect the SAR project,
run the experiment using the GF3 dataset,
and tell me why it fails.
```

Agent：

```text
Need:
repo://sar
dataset://sar/gf3
cuda
```

Placement：

```text
lab-gpu
```

执行。

Result 返回。

Agent 分析。

用户看到：

```text
Execution placed on lab-gpu
because:
- CUDA available
- GF3 dataset local
```

这一刻 Fabric 的价值无需解释。

---

# 92. README 第一屏建议

```text
# Fabric

A distributed agent harness that turns your computers,
servers and cloud environments into one execution fabric.

Reason globally.
Execute locally.

Move computation to data,
not data to computation.
```

然后：

```text
Laptop ──┐
         │
GPU PC ──┼── Fabric ── Agent
         │
Cloud ───┘
```

---

# 93. 核心术语

项目内部最好统一以下语言：

```text
Control Plane
Execution Plane

Agent
Task

Node
Capability
Resource

Execution
Placement

Observation
Artifact

Policy
```

尽量避免含混使用：

```text
machine
environment
workspace
remote computer
worker
client
```

除非特指。

---

# 94. Architecture Invariants

这些可以直接写进 CONTRIBUTING。

### Invariant 1

Agent 不应该知道 physical node details。

### Invariant 2

Agent 不应该依赖 physical filesystem path。

### Invariant 3

Tool semantics 与 execution location 分离。

### Invariant 4

Node 保留最终执行授权权。

### Invariant 5

Task 生命周期不依赖 Interaction Session。

### Invariant 6

Execution 必须可观察。

### Invariant 7

Result 应尽量结构化，而不是 raw output。

### Invariant 8

Data 默认留在拥有它的 Node。

---

# 95. Fabric v0.0.1 最重要的工程问题

真正值得花时间研究：

### 1. Execution abstraction

怎样描述一次与 Node 无关的 execution？

### 2. Capability model

怎样描述节点能力？

### 3. Resource locality

怎样表示“数据在哪里”？

### 4. Placement

怎样决定任务在哪运行？

### 5. Result abstraction

Node 应该向 Agent 返回什么？

### 6. Failure model

远程 Node 离线怎么办？

### 7. Security boundary

Cloud Agent 到底有什么权力？

### 8. State model

Task/Execution 如何持久化？

这八个问题比：

```text
Prompt 写什么？
```

重要得多。

---

# 96. 第一阶段不要优化 Agent 智商

Fabric 的独特价值不应该首先通过：

```text
更好的 coding benchmark
```

证明。

首先证明：

```text
same Agent
+
better execution substrate
```

可以完成传统 Agent 很难自然完成的任务。

即：

> **竞争力来自 Harness topology，而不仅是 model intelligence。**

---

# 97. 项目技术哲学

Fabric 应该尽量延续 nanobot 带来的一个优点：

> **Small enough to understand.**

不要因为 Distributed Systems 就立刻变成几十个 service。

v0.0.1 推荐：

```text
1 Server Process
+
N Node Processes
+
SQLite
+
WebSocket
```

足够。

---

# 98. Monolith First

Server：

```text
Agent
Task Manager
Scheduler
Registry
API
```

都在一个进程。

不要：

```text
agent-service
scheduler-service
node-service
task-service
artifact-service
```

Microservice 只会降低开发速度。

---

# 99. Protocol First

虽然实现保持 Monolith，

但是：

```text
Server ↔ Node
```

的 protocol 必须清晰。

因为这才是 Fabric 的真正系统边界。

---

# 100. Fabric 的护城河最终可能在哪里

如果长期做成，真正有价值的不是：

```text
LLM Wrapper
```

而可能是：

### Execution Graph

Agent 怎样将目标拆成 execution。

### Placement Intelligence

什么 computation 应该在哪里发生。

### Resource Graph

哪些数据、能力、身份存在于哪里。

### Policy

什么操作允许在哪些环境发生。

### Runtime Reliability

长任务如何跨 Node、断线和失败持续运行。

### Context Locality

哪些信息应该离开 Node，哪些不应该。

这六个东西才可能形成真正的 Harness 深度。

---

# 101. 最终产品 Thesis

Fabric 的世界观不是：

> Every computer should have an AI agent.

而是：

> **Every computer can become an execution node of one agent.**

进一步：

> **Your devices should not each host an isolated agent.  
> Together, they should form the execution substrate of one persistent intelligence.**

---

# 102. v0.0.1 一句话目标

> **让一个运行在云端的 Agent，能够发现两个具有不同 capability 和不同 data locality 的计算节点，并自动将 execution 放置到正确节点上。**

如果这一句话跑通：

Fabric v0.0.1 就成功了。

---

# 103. North Star

Fabric 最终不是：

```text
Cloud Agent with remote tools.
```

而是：

```text
Distributed Agent Runtime
```

最终形态：

```text
                    Global Reasoning

                         Agent
                           │
                         Task
                           │
                     Task Graph
                           │
                       Scheduler
                           │
             ┌─────────────┼─────────────┐
             │             │             │
          Laptop         GPU PC         Cloud
             │             │             │
           Data          Compute        Sandbox
             │             │             │
             └─────────────┼─────────────┘
                           │
                       Artifacts
                           │
                      Observations
                           │
                           ▼
                         Agent
```

这就是：

# One Agent.

# Many Nodes.

# One Execution Fabric.

---

# Appendix A — 名称建议

## Fabric

概念表达：★★★★★  
简洁程度：★★★★★  
品牌独特性：★☆☆☆☆

作为内部 architecture codename 非常漂亮。

作为公开项目名存在较严重撞名风险。

---

## RunWeave

推荐指数：★★★★★

含义：

```text
Run
+
Weave
```

即：

> weave distributed execution into one runtime.

优点：

- 保留 Fabric 的“编织”隐喻；
- 强调 execution；
- 不是典型 Chat/Agent 命名；
- 很适合 CLI：

```bash
runweave node
runweave task
runweave exec
```

Tagline：

> **Weave every machine into one agent runtime.**

---

## NodeWeave

推荐指数：★★★★☆

更加直接强调：

```text
Nodes → Fabric
```

Tagline：

> **Weave your machines into one execution fabric.**

缺点是产品未来如果抽象超过 physical nodes，会略显局限。

---

## TaskWeave

推荐指数：★★★★☆

更加偏：

```text
Task Graph
+
Distributed Execution
```

非常适合未来做 task scheduling。

但弱化了 Node/Infrastructure 的感觉。

---

## Weft

推荐指数：★★★★☆

`weft` 是织物中的“纬线”。

极其简洁。

技术感强。

但词义比较生僻，传播成本高。

---

## Fabric Runtime

推荐指数：★★★☆☆

能暂时作为：

```text
fabric-runtime
```

repo 名。

但仍然解决不了 Fabric 本身的品牌冲突。

---

# Appendix B — 当前推荐命名策略

项目开发早期：

```text
Codename: Fabric
```

架构概念：

```text
Execution Fabric
```

如果准备公开：

```text
Project: RunWeave
```

核心描述：

> **RunWeave is a distributed agent harness that turns your computers, servers and cloud environments into one execution fabric.**

Tagline：

> **Reason globally. Execute where the data lives.**

或者：

> **One agent. Many nodes. One execution fabric.**

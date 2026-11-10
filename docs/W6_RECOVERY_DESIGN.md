# W6 断线恢复：复现与设计草案

日期：2026-11-10  
状态：**设计草案，待确认；尚未实施**  
范围：模拟器、MQTT 与 twin 后端之间的模拟事件；不接入真实设备或 MES。

## 复现：后端离线导致尾部缺失

基线：`eed91c3`；独立 Compose 项目 `w6-repro-offline-20261110`，W2/W3/W6、Mosquitto 默认配置、100 板 normal 场景、seed 2026、速度 20×。执行步骤：

1. 使用下述临时覆盖文件启动本地栈，在模拟器运行期间停止该项目的 backend，保留 broker 和 simulator。
2. 停止时 PostgreSQL 的 `runs.last_sequence` 为 **536**，完整性为 `uncertain`。
3. 模拟器继续运行，`docker wait` 返回 **0**；确定性 dry-run 生成 **3411** 条事件，末条是 `workorder.completed`。
4. 发布器退出后重新启动 backend：快照与数据库均停在 **536**，工单仍为 `running`，状态 `uncertain`。没有后续事件来揭示序号缺口。

```sh
cat > /tmp/opencode/w6-disconnect-repro.yml <<'EOF'
services:
  simulator:
    command: ["run", "--scenario", "/scenarios/normal.json", "--run-id", "demo-run", "--seed", "2026", "--speed", "20", "--mqtt-url", "tcp://mosquitto:1883"]
EOF
docker compose -p w6-repro-offline-20261110 -f compose.w2.yml -f compose.w3.yml -f compose.w6.yml -f /tmp/opencode/w6-disconnect-repro.yml up -d --build
docker stop w6-repro-offline-20261110-backend-1
docker exec w6-repro-offline-20261110-postgres-1 psql -U smt_demo -d smt_twin -Atc "select last_sequence,integrity from runs where run_id='demo-run'"
docker wait w6-repro-offline-20261110-simulator-1
docker start w6-repro-offline-20261110-backend-1
curl -fsS --retry 10 --retry-all-errors --retry-delay 1 http://127.0.0.1:8080/api/runs/demo-run/snapshot
docker compose -p w6-repro-offline-20261110 -f compose.w2.yml -f compose.w3.yml -f compose.w6.yml -f /tmp/opencode/w6-disconnect-repro.yml stop
```

停止时序不同，536 等具体值会变化；需保证停止发生在生产期间。项目卷 `w6-repro-offline-20261110_w6_postgres` 被保留；上述命令不会删除卷。不要对已有项目复用该项目名。事件期望值可在 `simulator/` 运行 `go run ./cmd/simulator run --scenario scenarios/normal.json --run-id demo-run --seed 2026 --speed 100000 --dry-run > /tmp/opencode/w6-disconnect-expected.ndjson` 得到；dry-run 中墙钟字段不同，不应直接按原始字节比较两次独立运行。

## 根因与覆盖边界

- `backend/internal/ingest/mqtt.go` 使用进程随机 client ID，Paho 默认 clean session；离线订阅没有可供该实例恢复的持久会话。
- `deploy/mosquitto/mosquitto.conf` 的 `persistence false` 意味 broker 重启也不保留会话/队列。即使启用持久会话，现有有界队列仍可能在溢出时丢消息。
- `simulator/internal/publisher/mqtt.go` 等待的是 broker 对发布的 QoS 1 PUBACK；它不证明后端接收或提交。发布器没有持久发件箱，进程崩溃时无法确保未提交事件可重发。
- 后端只接受连续序号；只有收到更高序号才能发现中间缺口。尾部缺失没有后续事件提示；`uncertain` 是合理的保守结果，而不是已修复。
- 本次证据只覆盖一次后端正常停止、发布器继续运行并结束的场景。它没有单独证明 broker 重启或发布器进程崩溃的行为。此前 `docs/W6_REPORT.md` 的 100000× 溢出是另一次历史实验，不冒充本次重现。

## 目标与非目标

**目标：**在明确的本地磁盘容量、队列和故障模型内，后端离线或重启后能够从持久来源补齐缺失事件；正常完成的 run 只有在终止边界和所有序号均已持久化时才可声明完整。保留现有 W5 追溯、W6 重放、WebSocket snapshot-first，以及浏览器默认本地演示。

**非目标：**生产级高可用、无限离线容量、真实设备控制、认证/TLS、任意节点永久丢盘后的无损恢复。不能宣称 MQTT 或系统具备普遍的 exactly-once 交付；业务投影依靠幂等去重。

## 推荐架构（待确认）

采用**发布端持久日志 + 后端按序补拉/确认**；MQTT 只作为低延迟通知通道。稳定 MQTT client ID、持久会话及 broker 持久化可作为短暂断线优化，但不能单独承担可靠性保证。

1. **发布端持久日志。**模拟器在发布之前，将已生成事件的完整不可变 envelope（包括首次 `emittedAt`）按 `(runId,sequence)` 原子写入本地持久存储。存储支持顺序读取和唯一性约束；建议优先评估本地 SQLite，验证 Go 驱动与容器构建兼容性后再锁定依赖。保留 volume 不覆盖既有运行。序号和事件 ID 在重发中完全相同；不能通过重新执行 dry-run 并重新生成 `emittedAt` 冒充同一个事件。
2. **重启与来源边界。**日志记录场景配置、seed、模拟器/事件 schema 版本、发布状态与终止标记。模拟器崩溃后重启须先重发已持久化事件；只有能证明模拟器状态恢复一致时才能继续生成，否则停止并明确标记未完成，不能默默重用 run ID。为恢复生成进度，必须设计并测试模拟器状态 checkpoint 与事件批次的原子一致性；仅保存已发事件不等于支持从进程崩溃后继续模拟。
3. **后端确认与补拉。**新增仅本地/容器网络可达的只读补拉接口，或等价的请求/响应通道；后端从其数据库已提交的连续游标 `lastSequence+1` 分页读取发布端日志，逐条经现有 schema 验证、dedup 和 reducer 后事务提交。服务端在提交后提供连续持久游标确认；源端只能依据此业务确认回收，而不能依据 MQTT PUBACK 回收。补拉协议必须限制页大小、并发和超时，且禁止让浏览器直连 broker。
4. **直播/补拉交接。**每个 run 单写入者串行化补拉与 MQTT 摄取。缺口发生后暂停实时投影并缓冲或拒绝新事件，按持久日志补齐，再原子地完成追赶并发布新 snapshot/epoch；不得跳过序号，也不能把此前 `gap` 静默改成 `complete`。相同 `(runId,eventId)` 与内容为重复；不同内容或序号冲突保持 `conflict`，不自动修复。
5. **终止边界与完整性。**持久来源生成并保存带最终序号的明确 run 完结记录（建议作为版本化控制记录，与业务事件区分）。后端仅在能验证来源身份、相同 run 的最终边界、序号 1..N 连续并提交且无冲突时标示完整；否则保持 `uncertain`、`gap` 或 `conflict`。源日志无法访问、缺少终止标记及缺失尾部均不可推断为完整。
6. **容量和安全。**发件箱达到可配置上限时必须对模拟器施加背压并暴露状态，不允许静默丢弃。事件保留/回收策略需要以已提交确认及终止状态为准，禁止覆盖已有 run；默认只绑定 localhost 或隔离 Compose 网络，数据明确标识 SIMULATED。broker、源日志或数据库丢失其持久卷属于单独故障，需要明确恢复/人工处理，不能承诺无损。

### 两个关键取舍

- 仅启用 MQTT 持久会话：改动小、可缓解短暂 backend 离线；但 broker 持久卷丢失、队列溢出、发布器崩溃及缺失尾部仍无法保证修复，**不足以满足目标**。
- 持久日志 + 业务游标补拉：改动较大，要解决本地存储、恢复协议和有界背压；可提供明确可测试的恢复边界，推荐作为本次设计方向。**本草案不主张对用户数据自动清理、无限重试或静默重建。**

## 实施前验证计划

1. 先将上述离线尾部案例自动化，断言当前实现无法恢复；新增可追溯、唯一临时卷的隔离测试，清理仅限测试创建的资源。
2. 覆盖源端崩溃点：写日志前、日志提交后发布前、broker PUBACK 后业务确认前、未完成批次、日志已满、磁盘失败；重发 envelope 必须字节稳定。
3. 覆盖 backend 崩溃点：接收前、事务前/中、提交后确认前、恢复期间再次崩溃；断言已提交前缀、去重、W5 计数和快照/WS revision 一致。
4. 分别验证 broker 短暂断开、broker 重启、backend 离线到源结束、消息重复/乱序、队列溢出、source 不可用、缺失尾部和冲突；任何不能证实的情况保持非完整状态。
5. 对 100 和 1000 板在明确限速与突发条件下检查完整唯一 boardId 集合、连续序号、最终边界、存储/队列占用、耗时；记录 CPU、内存和实际故障注入条件。高压溢出仍应能恢复，或者明确失败并保持非完整，不得静默成功。
6. 运行前端 lint、typecheck、unit/E2E、build、两个 Go 模块 race/vet 和 PostgreSQL/MQTT 集成；已有测试不因新服务成为默认必选而退化。

## 需要确认的产品边界

本草案假设允许给**独立模拟器**增加本地持久卷和只读补拉服务；浏览器本地演示仍不依赖它们。需要决定：当持久发件箱满或发布端持久卷不可用时，是否接受模拟生产暂停并等待人工处理，而不是继续生成或丢弃事件。

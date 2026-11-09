# W3 Twin Backend Implementation Plan

> **For agentic workers:** Execute only after explicit user approval, using the available inline execution workflow. Checkboxes track implementation; this document does not authorize implementation or commits.

**Goal:** 为 W2 合成事件建立独立 Go 孪生后端，提供可检查完整性的内存投影、REST 快照和无竞态的 WebSocket 快照/增量。

**Architecture:** MQTT QoS 1 输入经严格契约和 topic 校验后，由单一串行状态所有者执行逐 run 去重、序列检查和投影；同一临界区完成快照与订阅注册。W3 不持久化或伪造缺失历史，状态完整性、进程 epoch 和断线期间的不确定性对客户端可见。

**Tech Stack:** Go 1.24+、Paho MQTT v1.5.0、jsonschema/v6 v6.0.2、gorilla/websocket v1.5.3、Mosquitto 2.0.21、Docker Compose；版本以实现时模块锁定为准。

**Spec:** `docs/W2_W6_ROADMAP.md` §3、§6；输入契约 `contracts/event.schema.json`、`docs/EVENT_CONTRACT.md`。本计划的协议和假设为建议，须由用户批准。

## 全局约束

- 全部数据 SIMULATED；只读，无生产设备、PLC、真实 MES、数据库或浏览器 MQTT；不改本地演示及其行为。
- 逐 `(runId,eventId)` 去重；全局逐 run 序列从 1 开始；`emittedAt`、`simTimeMs` 和后端 UTC `ingestedAt` 分离。
- W2 的 `simulator/internal/contract` 受 Go internal 规则限制，新模块不能直接 import；共享 schema 作为契约真源，不复制为第二个漂移的 schema。
- 非保留 MQTT 事件无法补发：晚加入、重启或断线后无法保证完整性；W3 只支持显式离线有序 fixture 重建，不声称持久恢复。
- W2 没有 process-start、队列容量或独立 board-created 事件；只投影可观察的站点归属、处理完成、检查、最终离站，不推断队列位置、加工开始或准确 WIP 的内部细节。路线由 W2 场景定义，不能假定所有自定义 run 都是默认路线。
- HTTP 服务及 broker 宿主机端口默认仅绑定 127.0.0.1；本地 demo，不提供生产认证、TLS 或公网部署保证。

## Review Focus

1. 初次收到序列大于 1、订阅晚于发布、断线尾部丢失但无后续事件：暴露 incomplete/uncertain，不静默宣称完整（任务 2、3、6）。
2. 同 ID 不同内容、同序号不同 ID、乱序补到：隔离冲突、拒绝非连续事件，不重复投影（任务 2）。
3. snapshot 获取与事件并发、慢消费者和重启 revision 重用：原子订阅、显式 resync、epoch 区分（任务 4）。
4. 不合法 schema、topic 与 envelope 不一致、超大消息和未知 schema：拒绝且记录原因，绝不污染状态（任务 1、3）。
5. 末站不固定、检查失败仍流转、跨站 exit/enter 中间状态：不误报最终完成或丢失 PCB（任务 2）。

## 基准和风险

- W2 实现：`simulator/`、`contracts/`、`compose.w2.yml`；报告 `docs/W2_REPORT.md` 尚标记待用户验收。`docs/IMPLEMENTATION_BASELINE.md` 是 W2 之前的历史快照，其“无 Go/MQTT”描述已过时。
- W2 为每条事件生成连续 sequence；非 retained；QoS 1 重试可能重复。`pcb.exited` 后紧邻下一站 `pcb.entered`，但两者是独立事件；不含明确末站或完整拓扑，除非由后端配置提供。
- W2 发送 machine 的 observedState 和已经计算的 flowState/reason；后端原样分别保存，并标识 flow 为 simulator 派生，不能假装后端独立重算。
- 本次审查：`cd simulator && go test -race ./...`、`go vet ./...`、`W2_MQTT_INTEGRATION=1 go test -count=1 ./internal/publisher -run Integration -v` 通过；前端 lint、typecheck、19 单测、build、9 E2E 通过。Go race 输出显示缓存；集成测试非缓存。build 有 >500 kB chunk 警告。
- 固定配置与 run 关联是 W3 限制：注册 run 时给定有序 station IDs；未知 run 或拓扑不明确的导入应拒绝或标记未知，不能默认为已完成。W2 默认站点 `loader,printer,spi,mounter,reflow,aoi,unloader`；自定义场景必须明确配置。

## 文件与接口

- 新建 `backend/go.mod`, `backend/go.sum`, `backend/Dockerfile`, `backend/cmd/twin-backend/main.go`：独立服务及构建。
- 新建 `backend/internal/contract/`：嵌入构建时复制或挂载的 canonical schema（实现时选可重复构建方式，测试字节/hash 与根 schema 一致）、严格解码、topic 验证。
- 新建 `backend/internal/twin/`：run 注册、序列/去重、线程安全状态、revision、订阅和 reducer；投影类型和快照。
- 新建 `backend/internal/ingest/`：MQTT 订阅、连接状态与离线不确定性；`backend/internal/api/`：REST、WS；`backend/internal/assets/`：静态注册表。
- 新建各包相邻 `_test.go`、`backend/testdata/` 测试 fixture；新建 `compose.w3.yml` 扩展 `compose.w2.yml`，不改变 W2 默认命令；更新 `README.md`、`docs/ARCHITECTURE.md`、`docs/EVENT_CONTRACT.md`，新增 `docs/W3_REPORT.md`。

### Task 1：事件接收边界

**Files:** `backend/internal/contract/{event.go,event_test.go}`, `backend/go.mod`, `backend/go.sum`，根 schema 作为输入。

**Interfaces:** `DecodeAndValidate(topic string, body []byte) (Event, error)`；`Event` 包含完整 v1 envelope 和保留的原始 payload。

- [ ] 写测试：九类 fixtures 全部通过；拒绝未知字段、未知版本、无 synthetic、非 UTC、completion 数量不平衡、topic 的 run/machine 与 envelope 不一致、line/machine topic 不匹配、超过 1 MiB 的输入、非法 JSON。
- [ ] 执行 `cd backend && go test ./internal/contract -v`，确认新测试先失败。
- [ ] 实现 JSON Schema draft 2020-12 格式验证及 completion 算术检查；拒绝重复 JSON key，校验 topic 的精确分段；构建过程保证 canonical schema 与服务使用的版本一致。
- [ ] 再执行该测试确认通过，并测试实际 W2 dry-run 的每条事件，而非只测静态 fixture。

### Task 2：有序投影、完整性与可观察语义

**Files:** `backend/internal/twin/{store.go,reducer.go,types.go,store_test.go,reducer_test.go}`。

**Interfaces:** `NewStore(epoch string, runs map[string][]string) *Store`；`Apply(e contract.Event, ingestedAt time.Time) (ApplyResult, error)`；`Snapshot(runID string) (Snapshot, bool)`。`Snapshot` 包含 `runId`, `epoch`, `revision`, `lastSequence`, `integrity`, `machines`, `boards`, `workOrders`；所有对外快照必须深拷贝。

- [ ] 写测试：1、2 按序投影；相同 `(runId,eventId)` 与原始规范化内容完全一致时忽略且 revision 不变；相同 ID 不同内容及相同序号不同 ID 标记 conflict；初始 2、1 后收到 3 时先收到 3 均标记 gap 并冻结 run，不把后续事件当成可信投影；重复 1 不改变计数；两个 run 独立。
- [ ] 写生命周期测试：首站 entered 创建板；exit 后 board 处于 `in_transit`，下一 entered 转移所有权；仅配置的末站 exit 标记终态；inspection false 保留并继续流转；workorder.completed 的数量分别保存并核对可观察终态，不从检查数推断报废；reason 和 observed/flow 原样分离。测试未知站点、错序转换和 workOrder 不匹配进入 conflict。
- [ ] 运行 `cd backend && go test ./internal/twin -v` 确认失败，再实现 reducer。`revision` 只在接受新事件时递增；保存事件 ID 与序列的内容指纹直至进程退出，不限量增长是 W3 内存限制并在报告说明。
- [ ] 完整性固定为 `complete | uncertain | gap | conflict`；首次事件必须 sequence 1 才能为 complete；一旦 gap/conflict 冻结，不能因为迟到事件自动清除，需从完整 fixture 创建新 epoch 重建；明确返回原因及 lastSequence。状态和告警不能被拒绝的事件修改。
- [ ] 再运行测试及 `cd backend && go test -race ./internal/twin`，通过。

### Task 3：MQTT 消费与失联策略

**Files:** `backend/internal/ingest/{mqtt.go,mqtt_test.go,mqtt_integration_test.go}`。

**Interfaces:** `Start(ctx context.Context, brokerURL string, store *twin.Store) error`；订阅 `demo/smt/+/machines/+/events` 与 `demo/smt/+/line/events`，QoS 1；连接健康暴露 connected/subscribed/lastError。

- [ ] 先写假 transport 测试：成功订阅前不 ready；重连后重新订阅；任何订阅错误和断线立即将受影响所有 run 从 complete 改为 uncertain（新 run 也不自动证明完整）；后续事件不能消除 uncertain；非法消息/未知 run 可观测地拒绝；回调并发及队列满不得静默丢弃，应标记 uncertain 并报警。
- [ ] `cd backend && go test ./internal/ingest -v` 先失败；实现单有序处理通道、有限队列、Paho 自动重连与显式 resubscribe；连通性变化与事件按同一串行顺序提交，避免误判健康。未知 run 按明确配置拒绝并记录；初始化成功并开始订阅后才能启动模拟器。
- [ ] Docker 集成测试先启动干净 broker 与 backend、等待订阅就绪再发布 W2 样本；覆盖重复、异常 topic、broker 重启和断线末尾无后续事件仍 uncertain。执行 `cd backend && W3_MQTT_INTEGRATION=1 go test -count=1 ./internal/ingest -v`。

### Task 4：REST 与原子 WebSocket 协议

**Files:** `backend/internal/api/{http.go,websocket.go,http_test.go,websocket_test.go}`，`backend/internal/twin/` 订阅接口。

**Interfaces:** `GET /api/health` 返回 broker/订阅就绪及原因；`GET /api/assets` 返回静态 registry；`GET /api/runs/{id}/snapshot` 返回 snapshot，未知 run 404；`GET /ws/runs/{id}` upgrade 后第一帧 `{"type":"snapshot","epoch":"...","revision":N,"snapshot":{...}}`，后续 `{"type":"delta","epoch":"...","revision":N+1,"event":{...},"integrity":"..."}`。

- [ ] 写测试：两个并发订阅者收到同一 snapshot 和相同有序 delta；在订阅注册边界并发 Apply 不漏、不重；深拷贝不可变；未知 run 404；非法 method 405；health 未订阅时 503，订阅就绪时 200（run 完整性仍由 snapshot 单独报告）。
- [ ] 写测试：受限队列溢出时明确发送 `{"type":"resync_required","reason":"slow_consumer"}` 后关闭；若无法发送则立即关闭，客户端必须重连并用新 snapshot；integrity 改变须推送 `{"type":"status","epoch":"...","integrity":"...","reason":"..."}`，无事件序号或 revision 假增量；同一进程 revision 连续，重启 epoch 不同。
- [ ] 运行 `cd backend && go test ./internal/api -v` 确认失败；在 store 同锁内复制快照并注册后续队列；每个 WS 连接只允许一个 writer；对外无任意命令，限制消息、超时与同源/localhost Origin（允许无 Origin 的本地 CLI）；明确 reconnect 总是重新获取原子 snapshot，不实现 delta 回放。
- [ ] 运行 `cd backend && go test -race ./internal/api ./internal/twin`，验证通过。集成测试以两个真实 WebSocket 客户端检查一致性、重连和 epoch 变化。

### Task 5：静态资产注册表与部署

**Files:** `backend/internal/assets/{registry.go,registry_test.go}`, `backend/cmd/twin-backend/main.go`, `backend/Dockerfile`, `compose.w3.yml`。

**Interfaces:** `List() []Asset`，Asset 至少包含 `machineId`, `stationType`, `modelURL`, `representation`；七个默认 station ID 与 `src/data/stations.ts` 对齐，printer/mounter 指向现有 `/models/*.glb`，其余标记 procedural 且 modelURL 为 null。URL 为前端静态部署引用，不表示 backend 提供模型文件。

- [ ] 测试七条 ID 唯一、模型映射正确、返回的切片不影响注册表；配置自定义拓扑与缺失资产明确区分，注册表不是事件自动发现机制。
- [ ] 实现 CLI/env：broker、HTTP listen、run ID、逗号分隔有序 stations；run ID 和 station IDs 验证符合 topic 规则、非空唯一；默认 HTTP `127.0.0.1:8080`。容器内监听 `0.0.0.0:8080`，Compose 宿主映射 `127.0.0.1:8080:8080`。
- [ ] Compose 基于 W2 配置添加 backend，并确保 simulator 等待 backend 健康订阅，而不是只等待 broker；健康检查和 runId 与 simulator 命令一致。测试冷启动不会漏 sequence 1；不要修改或删除已有 W2 stack。
- [ ] 运行 `docker compose -f compose.w2.yml -f compose.w3.yml config`，然后在隔离的 Compose project 中启动，查询 health/assets/snapshot 并验证运行结束时 100 板，完成后只关闭本次创建的服务。

### Task 6：可复现重建、端到端与文档

**Files:** `backend/cmd/twin-backend/`、`backend/internal/` 对应测试、`README.md`, `docs/ARCHITECTURE.md`, `docs/EVENT_CONTRACT.md`, `docs/W3_REPORT.md`。

- [ ] 添加离线 fixture 导入模式：仅空 store、指定 run 与拓扑、从 sequence 1 起严格完整顺序逐行重放 W2 NDJSON，校验同一事件/拓扑规则；缺失、冲突、无首事件时失败且不发布部分结果；导入后用新 epoch 提供快照。禁止把导入误述为 MQTT 自动补偿或持久恢复。
- [ ] 写端到端测试：W2 normal 100 PCB 发布到真实 broker，两个 WS 客户端收敛且工作单完成；QoS 1 人工重复不改变 revision 或计数；遗漏 sequence 显式 gap；后端中途启动标记 gap 或 uncertain；断线尾部、慢消费者、backend 重启与 fixture 重建均显式演示。固定 seed 比较导入结果的业务字段，排除 epoch、ingestedAt、emittedAt。
- [ ] 执行 `cd backend && go test -race ./...`、`go vet ./...`、有 Docker 的 W3 集成命令，以及 `cd simulator && go test -race ./...` 和 W2 MQTT gated 测试；根目录运行 `npm run lint`、`npm run typecheck`、`npm test`、`npm run build`、`npm run test:e2e`。
- [ ] 文档记录端点 JSON、消息协议、订阅就绪、示例运行/关闭命令、拓扑注册、断线/重启恢复步骤、状态限制、所有实际测试输出和已知风险；不标记没有运行的测试为通过。完成 W3 后停止，等待 W4 批准。

## 验收关卡

- [ ] 两个 WS 客户端一致，快照/增量衔接无丢失，重复不导致双计数。
- [ ] 缺失、乱序、冲突、晚订阅、断线和重启均可见；不可恢复时不得标记完整。
- [ ] 100 PCB 的真实 W2 MQTT 流可投影；重放可重建可观察状态，不声称恢复 simulator 内部队列。
- [ ] REST、资产清单、健康检查和 Compose 可复现；仅限本地 synthetic、浏览器仍独立运行。
- [ ] 所有检查有实际输出；报告已记录局限，等待用户验收 W3。

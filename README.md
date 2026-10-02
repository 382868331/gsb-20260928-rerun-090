# 游戏内容编排平台（任务 090）

剧情编排 + 条件选项 + 奖励发放 + 存档继续的完整可运行实现。

## 目录

- `backend/`：Go 1.27 服务
  - `internal/story`：条件求值、剧情定义、实例运行时、奖励幂等、试演分支、文件/内存存储
  - `internal/seed`：内置剧情《雾港抉择》
  - `internal/api`：HTTP API
  - `cmd/server`：服务入口（同时托管 `frontend/dist`）
- `frontend/`：React 19 + Vite + TypeScript 页面

## 运行

```bash
# 后端（默认 :8080，存档写入 backend/data）
cd backend
go run ./cmd/server

# 前端开发服务器（/api 代理到 8080）
cd frontend
npm run dev
```

生产构建后直接由后端托管：`cd frontend && npx vite build`，再启动后端访问
`http://localhost:8080/`。

## 关键规则（均在服务端强制）

- 页面只返回当前事实下满足条件的选项（`GET /api/instances/{id}` 的
  `availableChoices`）；隐藏选项被直接提交时返回 `409`，因为提交时会按
  实例所固定的剧情版本（`defHash`）重新求值条件。
- 奖励按「同一节点的同一次访问 + 同一选项」只发一次；重复裸提交返回
  `409`。客户端每次点击生成 `idempotencyKey`，网络重试命中缓存结果
  （`applied.duplicate=true`），绝不二次发奖。
- 开始实例时固定剧情版本哈希；旧定义随版本归档持久化，重启后旧存档仍可继续。
- 剧情修订后先通过迁移审查列出每个存档：已完成事件和已领奖励保持原样，
  只有未来选择受影响。作者必须为某个分支显式采用新规则；未采用的分支继续旧版本。
- 若修订删除了存档当前节点，迁移会停在审查处并要求明确选择接续节点，系统不会跳到默认位置。
  迁移记录写入剧情日志，重开时说明新旧规则交界；撤回或重复确认尚未采用的修改不会改写历史。
- `POST /api/instances/{id}/forks` 在当前选择之前建立「试演」分支：分支与
- 存档以每实例一个 JSON 文件原子落盘，中断/重启后 `GET` 可继续。
- 空存档列表是正常状态：`GET /api/instances` 在全新数据下返回
  `{"instances":[]}` 而非 `null`，页面展示空态而非白屏；列表真正读取失败时
  页面保留「重试」入口。
- 页面支持「比较分支」：对同一选择点分出的分支并排展示，共同前缀只读，
  各分支的后续进度、条件状态与奖励各自独立；切回旧分支继续不会重复领奖。
- 「剧情记录」面板按存档中的日志逐步解释：节点、所做选择、该选择发放的
  奖励，重开存档、重复提交或条件变化后记录仍可逐步复盘。

## HTTP API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/stories` | 剧情列表（含版本哈希） |
| GET | `/api/stories/{id}` | 剧情定义 |
| POST | `/api/instances` | 开始新故事 |
| GET | `/api/instances` | 存档与分支列表 |
| GET | `/api/instances/{id}` | 继续：当前节点 + 允许的选项 |
| POST | `/api/instances/{id}/choices` | 提交选择 `{choiceId, idempotencyKey}` |
| POST | `/api/instances/{id}/forks` | 建立试演分支 `{label}` |
| PUT | `/api/stories/{id}` | 发布剧情修订并归档新版本 |
| GET | `/api/migrations` | 列出各存档/分支的规则影响审查 |
| GET | `/api/instances/{id}/migration` | 查看单个存档迁移审查 |
| POST | `/api/instances/{id}/migration` | 仅将该存档采用到最新版本，节点丢失时需 `{target}` |

## 测试

```bash
cd backend && go test ./...      # 条件/幂等/分支隔离/版本漂移/重启恢复/并发重复提交
cd frontend && npx vitest run    # 只渲染允许选项、重复提交被服务端拒绝、分支隔离
```

故事检查覆盖：重复选择不多拿、事实变化后条件重新确认、奖励后中断（含模拟
进程重启）继续，且正确性来自服务端状态机而非前端按钮禁用。

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
- 开始实例时固定剧情版本哈希；之后剧情被修改会得到 `409 version drift`，
  旧存档仍按旧定义继续。
- `POST /api/instances/{id}/forks` 在当前选择之前建立「试演」分支：分支与
  父实例的事实、道具、奖励、领取记录全部深拷贝隔离，父实例仅记录分支标记。
- 存档以每实例一个 JSON 文件原子落盘，中断/重启后 `GET` 可继续。

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

## 测试

```bash
cd backend && go test ./...      # 条件/幂等/分支隔离/版本漂移/重启恢复/并发重复提交
cd frontend && npx vitest run    # 只渲染允许选项、重复提交被服务端拒绝、分支隔离
```

故事检查覆盖：重复选择不多拿、事实变化后条件重新确认、奖励后中断（含模拟
进程重启）继续，且正确性来自服务端状态机而非前端按钮禁用。


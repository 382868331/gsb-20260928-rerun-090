# 游戏内容编排平台（本轮：条件对话 + 剧情进度）

本轮交付可运行的最小工程：Go 管理剧情定义、版本与运行实例（存档），
React 提供分支编辑与试玩。任务系统、角色关系不在本轮范围。

## 目录

- `backend/` — Go 1.27，模块 `storyplatform`
  - `internal/story` — 剧情模型、条件求值、互斥校验、运行器、存储、种子短故事
  - `internal/server` — JSON HTTP API
  - `cmd/server` — 入口（默认 `:8080`，数据写 `./data`）
- `frontend/` — React 19 + Vite + zustand，`src/Editor.tsx` 编辑，`src/Player.tsx` 试玩

## 运行

```bash
# 后端（环境变量 ADDR、DATA_DIR 可选）
cd backend && go run ./cmd/server

# 前端（开发服务器把 /api 代理到 8080）
cd frontend && npm run dev   # 或 node node_modules/vite/bin/vite.js
```

## 关键规则

- **选择改变状态**：节点进入效果与选择效果支持 `set` / `add` / `grant`；
  `once: true` 或 `grant` 在整个存档生命周期内只生效一次。回到旧节点
  （条件不满足时绕路返回）不会重复发放一次性奖励，`visited` 记录到达次数。
- **条件互斥**：同一节点上带条件的选项，其条件必须可被证明两两不相交
  （等值不等、数值区间不相交）；无法证明的跨键条件对会在发布时拒绝。
  无条件选项始终可用。
- **页面与运行器一致**：API 只返回当前允许的选项（`view.choices`），
  前端只渲染这些按钮；选择被隐藏的选项返回 `400`，并有 HTTP 测试锁定。
- **存档绑定版本**：实例保存 `storyId + version`。发布定义必须版本 +1；
  旧存档 resume 时版本不符返回 `409 VersionMismatchError`，不做隐式迁移。
- **删除节点不乱跳**：存档当前节点（或历史节点）在定义中缺失时返回
  `409 MissingNodeError`，页面显示错误而不是重定向到起点或其他节点。

## API

- `GET /api/stories`，`GET|PUT /api/stories/{id}`
- `POST /api/stories/{id}/start`，`POST /api/stories/{id}/choose`
- `POST /api/stories/{id}/saves/{saveId}/resume`
- `GET /api/saves`，`GET|DELETE /api/saves/{saveId}`

## 验证用短故事（seed: crossroads）

篝火 → 市集（+3，仅一次）/ 密林（+5，仅一次）→ 城门汇合 →
按 `gold >= 5` / `gold < 5` 互斥分流；穷路径可返回旧节点验证奖励不重发，
中途可随时以命名存档保存/继续；发布删节点的 v2 后旧存档 resume 被拒。

## 测试

```bash
cd backend && go test ./...
cd frontend && node node_modules/vitest/vitest.mjs run
```

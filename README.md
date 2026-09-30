# 游戏内容编排平台（task-090）

剧情实例引擎 + 文件存档 + HTTP API + React 页面。所有状态规则都在服务端
强制执行，前端按钮是否禁用不影响正确性。

## 运行

```bash
# 后端（默认 :8080，数据目录可用 STORY_DATA 覆盖）
cd backend && go run ./cmd/server

# 前端（:5173，已配置 /api 代理到 8080）
cd frontend && npm run dev
```

首次启动会自动播种内置故事 `hero-road`（v1 过路税 10 金币，v2 过路税 25）。

## 关键规则

- **只显示当前允许的选项**：`GET /api/instances/{id}/view` 用实例当前事实
  重新求值每个选项条件，只返回允许项；前端只渲染返回内容。
- **提交时按固定版本复核**：实例启动时记录剧情 `version + 内容 hash`；每次
  选择都按该版本重新求值条件，新版本发布不会让旧存档悄悄迁移。
- **重复提交不多拿奖励**：客户端为每次点击生成 `requestId`；服务端记录
  requestId → (节点, 选项)，重放返回 `replayed: true` 且不再应用效果。
  即使绕过前端直接 POST，未满足条件的选项返回 409。
- **试演分支**：`POST /api/instances/{id}/fork` 在“下一次选择之前”深拷贝
  事实、已领奖励、幂等键与日志，分支间互不污染。
- **存档继续**：每次提交先原子写入 JSON 再应答；重启后通过
  `DefinitionFor` 按版本/hash 找回旧剧情继续。

## API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | /api/stories | 列出当前剧情 |
| GET | /api/stories/{id} | 当前版本定义 |
| POST | /api/stories/{id}/versions | 发布新版本 |
| GET | /api/instances | 存档/分支列表 |
| POST | /api/instances | 开始存档 `{storyId, facts}` |
| GET | /api/instances/{id} | 存档 + 当前视图 |
| GET | /api/instances/{id}/view | 当前节点与允许选项 |
| POST | /api/instances/{id}/choices | 提交 `{choiceId, requestId}` |
| POST | /api/instances/{id}/fork | 建立试演分支 `{name}` |
| DELETE | /api/instances/{id} | 删除存档 |

## 测试

```bash
cd backend && go test ./...
cd frontend && npx vitest run
```

后端故事检查覆盖：隐藏/禁止不满足条件的选项、同一 requestId 重复提交只发
一次奖励、奖励后“崩溃重启”继续、试演分支的标记/道具隔离、新版本发布后旧
存档仍按 v1 走完。

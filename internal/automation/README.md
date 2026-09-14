# 自动化模块 (Automation)

WeKnora 的定时智能体执行模块: 管理员把"某个智能体 + 一条指令 + 一个 cron
周期"声明为自动化任务, 平台按计划在独立会话中运行智能体并保留完整运行
历史。智能体的全部能力 (工具/知识库/沙箱/Action) 在定时场景天然可用。

## 架构

```
前端 (frontend/src/automation/ + views/automation/, 自包含)
   │  /api/v1/automations/*
后端 internal/automation/
   ├─ 元数据: automations / automation_runs (migration 000902, 900+ 保留段)
   ├─ scheduler.go: robfig/cron 进程内调度, 只负责"到点入队"
   │    └─ 触发 → TaskEnqueuer.Enqueue(automation:run) → 队列 automation
   ├─ runner.go: Asynq worker 执行 (maintenance 池)
   │    └─ Redis 单飞锁 → 建会话 + user/assistant 消息
   │       → eventBus 订阅 FinalAnswer 收割输出
   │       → sessionService.AgentQA (与 IM 通道同款程序化调用)
   │       → 更新 assistant 消息 + Run 状态/输出/耗时
   └─ handler.go: REST (viewer 读 / contributor 写与运行 / admin 删)
```

## 关键设计决定

- **调度与执行分离**: cron 每实例都会跑, 入队靠 Asynq 天然去重 + Redis
  单飞锁 (`automation:lock:{id}`) 保证多副本不重复执行; 重叠策略:
  skip (默认, 记一条 skipped) 或 queue (等待, 上限为超时预算)
- **每次运行新会话**: 无跨运行上下文 (设计决定); 运行详情 = 回放该
  会话消息, 复用现有会话渲染, 零额外存储
- **失败不自动重试**: MaxRetry=0, 定时任务常带副作用, 宁可显式失败
- **超时**: run ctx 带 timeout, 到期取消并标记 timeout; ctx 取消即
  终止智能体执行
- **指令模板变量**: {{date}} {{time}} {{datetime}} {{weekday}}
- **写库不做在本模块**: 与 Action 模块同一原则, 定时任务要产生业务
  副作用时, 智能体经 Action (webhook) 调业务系统 API

## API (/api/v1/automations)

`GET/POST /automations` `GET/PUT/DELETE /automations/:id`
`POST /automations/:id/run` (手动触发, 入队) `GET /automations/:id/runs`

## 部署

无独立组件。依赖: Redis (Asynq 队列 + 单飞锁)、既有 Asynq worker 池
(automation 队列挂 maintenance 池)。迁移 000902 自动应用。

## 已知限制 (v1)

- 通知推送 (webhook 回调 / IM) 未实现; notify_config 字段预留
- 运行产物自动入库知识库未做 (设计上排除)
- ru/ja/ko 前端文案暂以英文兜底
- 未来 3 次执行预览: 列表页由后端精确计算; 编辑抽屉内仅对常见 preset
  做本地近似预览

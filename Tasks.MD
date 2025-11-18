# 开发任务拆解

## 准备与环境
- [x] 建立 Go 模块与基础目录结构（cmd/internal/web 等），确保 go.mod/go.sum 能通过 `go mod tidy`。
- [x] 添加基础配置文件模板（config.example.yaml），引导配置项（fetcher/email/server）。
- [x] 设置 Makefile/任务脚本以运行构建与测试（make run / go test ./...）。

## 数据模型与存储
- [x] 定义 `internal/model/job.go` 的 `Job` 模型（含 Tags JSON、时间字段、Source/URL 等）。
- [x] 实现 `internal/storage/store.go`：初始化 SQLite，自动迁移，提供 CRUD/去重接口（按 Job.ID）。
- [x] 为存储层编写单元测试（使用临时 SQLite 文件/内存），覆盖迁移与增删改查。

## 抓取器
- [x] 定义 `internal/fetcher/fetcher.go` 抽象接口 `JobFetcher` 与配置结构。
- [x] 实现 `internal/fetcher/eleduck.go`：抓取 HTML、解析 `__NEXT_DATA__` JSON，筛选“远程工作”，支持分页、max_age_days、max_pages。
- [x] 为抓取器编写单元测试：解析固定 HTML 样例，验证筛选与分页终止逻辑。

## 业务逻辑与调度
- [x] 实现 `internal/scheduler/cron.go`：根据配置定时调用抓取器与存储，使用 errgroup/context 管理。
- [x] 实现增量更新逻辑：检测新职位并返回新增集合；为调度层编写单元测试（假 fetcher/store）。

## 通知
- [x] 实现 `internal/notifier/email.go`：SMTP 配置，发送新增职位摘要邮件；避免重复通知。
- [x] 为邮件通知编写单元测试（mock SMTP/接口包装），覆盖无新增/有新增场景。

## Web API 与前端
- [x] 实现服务入口 `cmd/server.go`：加载配置、初始化存储/抓取器/调度/通知，暴露 REST API（职位列表、手动刷新）。
- [x] 创建前端页面（`web/index.html`）展示职位列表并支持刷新。
- [x] 为 API 层编写单元测试（httptest），覆盖列表接口与刷新接口。

## 配置与文档
- [x] 完善 `.gitignore`（忽略二进制、数据库文件、node_modules、dist 等）。
- [ ] 更新 README/usage：运行步骤、配置示例、测试命令。
- [x] 确认所有测试通过（`go test ./... -cover`；前端可选 `pnpm test`/`pnpm build`）。

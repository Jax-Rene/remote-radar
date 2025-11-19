# 远程职位抓取与通知系统

该项目是一个用 Go 实现的远程职位聚合器，定期从 **电鸭社区** 获取最近 30 天内发布的远程工作机会，并通过邮件发送新职位通知，同时提供一个简单的前端界面查看当前职位列表。后续可扩展到其他平台（如小红书）。

## 功能概述

- **数据抓取**：使用自定义爬虫从电鸭招聘分类（`/categories/5?sort=new`）页面抓取职位信息。页面的 `__NEXT_DATA__` 脚本中包含完整的 JSON 数据，每个职位的 `tags` 字段用于判断是否为“远程工作”【249504313504972†L24-L31】。
- **增量更新**：首轮抓取仅获取最近 30 天发布的帖子；之后按设定的时间间隔抓取最新页，避免重复抓取历史数据。
- **数据库存储**：使用 **SQLite** 保存职位数据，结合 GORM 自动迁移表结构并完成 CRUD 操作。
- **邮件通知**：当有新职位产生时，通过配置好的 SMTP 服务发送电子邮件提醒。
- **Web API 与前端**：使用 Gin 构建 REST API，前端基于 HTMX + AlpineJS + TailwindCSS + Vite，提供职位列表页面并支持刷新。

## 技术栈

- **语言**：Go 1.21+
- **框架**：
  - Web 服务： [Gin](https://github.com/gin-gonic/gin)
  - ORM： [GORM](https://gorm.io)
- **数据库**：SQLite
- **前端**：HTMX、AlpineJS、TailwindCSS、Vite

## 项目结构

```text
remote-jobs/
├── cmd/               # 主程序入口
│   └── server.go      # 启动 HTTP 服务并注册路由
├── internal/
│   ├── fetcher/
│   │   ├── fetcher.go # 抓取接口定义
│   │   └── eleduck.go# 电鸭实现
│   ├── model/
│   │   └── job.go     # 职位数据模型
│   ├── storage/
│   │   └── store.go   # 数据库初始化和 CRUD
│   ├── notifier/
│   │   └── email.go   # 邮件通知逻辑
│   └── scheduler/
│       └── cron.go    # 定时任务管理
├── web/               # 前端源码（Vite 项目）
├── config.yaml        # 配置文件
└── README.md
```

## 抓取策略

1. **解析页面数据**：电鸭使用 Next.js 渲染页面，页面中的 `<script id="__NEXT_DATA__">` 标签包含序列化的 JSON 数据。抓取器读取该标签内容，并从 `postList.posts` 列表中提取职位【313429956044464†L1-L4】。
2. **筛选远程职位**：每条职位信息有 `tags` 字段，其中包含“工作方式”标签，如 `"远程工作"`，抓取器据此过滤出远程职位。
3. **分页抓取**：分类页支持通过 `?page=N&sort=new` 指定页数。首次抓取时从第 1 页开始遍历，遇到发布时间早于设定天数（默认 30 天）即停止翻页。后续定时任务只抓取前 `max_pages` 页，遇到旧数据即可停止。
4. **去重与更新**：每条职位具有唯一 ID，抓取时检查数据库是否已存在；若不存在则写入并记录发布时间，用于增量更新。

## 数据模型

`Job` 模型使用 GORM 定义，字段示例如下：

```go
// Job 表示一个远程职位
type Job struct {
    ID          string    `gorm:"primaryKey"` // 电鸭帖子 ID
    Title       string
    Summary     string
    PublishedAt time.Time
    Source      string    // 平台名称，如 "eleduck"
    URL         string    // 原始链接
    Tags        datatypes.JSONMap // 标签键值对
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

## 配置说明

项目通过 `config.yaml`（或环境变量）配置以下内容：

```yaml
fetcher:
  max_age_days: 30    # 抓取的最旧天数
  max_pages: 3        # 每次爬取的最大页数
  interval: "0 */2 * * *" # Cron 表达式，示例为每 2 小时一次

email:
  host: smtp.example.com
  port: 587
  username: user@example.com
  password: secret
  from: "notifier@example.com"
  to: ["you@example.com"]

notifier:
  driver: email # email 或 log

server:
  addr: ":8080"

```

## 使用方式

1. **克隆项目并安装依赖**：

```bash
git clone <repo-url>
cd remote-jobs
go mod tidy
```

2. **配置应用**：复制 `config.example.yaml` 为 `config.yaml`，根据实际情况修改抓取间隔、邮件服务器等配置。

3. **启动服务**：

```bash
# 首次运行会初始化数据库并进行首次抓取
make run
```

如需立即触发一次抓取而不等待定时任务，可使用：

```bash
make run-once
```

4. **访问前端**：启动服务后，在浏览器中打开 `http://localhost:8080` 查看职位列表。

开发阶段若无需真实发信，可将 `config.yaml` 中的 `notifier.driver` 设置为 `log`，系统会在控制台打印新增职位。

## 扩展开发

- **新增平台**：实现 `JobFetcher` 接口即可接入其他平台。每个实现负责构建请求、解析响应，并返回统一的 `[]Job`。
- **订阅关键字**：可在抓取器中增加关键字过滤，或在通知模块中根据用户偏好筛选推送内容。
- **部署**：可将后端部署为二进制程序或 Docker 容器，利用 systemd 或容器编排工具保持服务常驻。

## 致谢

本项目的抓取逻辑参考了社区博客的实践，其中指出可以通过解析电鸭页面 `__NEXT_DATA__` 标签中的结构化 JSON 获取帖子列表【249504313504972†L24-L31】。由此实现的抓取方案既简洁又高效。

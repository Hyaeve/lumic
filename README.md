<p align="center">
  <img src="./Lumic-icon.png" alt="Lumic Logo" width="88">
</p>

# Lumic · 拾光

Lumic（拾光）是一个自托管的个人内容聚合与归档服务。它将微博、Pixiv、哔哩哔哩等平台的订阅内容汇入统一时间线，在本地保存动态正文、原始图片和视频，并提供搜索、标签、收藏、作者视图、瀑布流以及移动端适配。

项目采用 Vue 3 + Vite 构建前端，Go 提供 API、平台连接器、同步任务与静态资源服务。正式镜像将前后端打包为一个容器，适合通过 Docker Compose 长期运行。

## 主要功能

- 多平台时间线：统一浏览微博、Pixiv 和哔哩哔哩动态，并可按平台、作者或标签筛选。
- 订阅来源：支持启用或停用、Cron 定时同步、来源标签、关键词包含/排除、包含图片和包含视频等规则。
- 增量与历史获取：定时同步和“拉取最新”仅获取增量内容；“获取历史动态”会遵循来源过滤设置，并跳过本地已经完整归档的内容。
- 动态浏览：支持正文、Emoji、原图、视频、最多九宫格预览、列表和瀑布流布局。
- 搜索排序：可搜索动态正文、作者与标签，并按最新或最早排序。
- 内容管理：支持收藏、右键菜单、多选操作、删除动态及其本地文件、删除来源的全部动态。
- 图片查看：列表使用较轻的预览图，大图模式加载原图；桌面端支持缩放、旋转、下载和拖拽，移动端支持触控手势。
- 响应式界面：桌面端提供完整侧栏，手机竖屏自动切换为紧凑的图标菜单和双列瀑布流。
- 本地归档：原始媒体与作者正文文件保存在可直接访问和备份的目录中。
- 配置备份：设置页可备份和恢复项目配置。
- Lumir 接口：提供带客户端令牌认证和游标分页的 `/api/v1`，避免一次加载大量动态。

## 平台支持

| 平台 | 当前能力 |
| --- | --- |
| 哔哩哔哩 | 扫码或 Cookie 接入；订阅 UP 主图文动态；可选视频动态与专栏；可添加账号“收藏专栏”来源 |
| 微博 | 账号密码、扫码或 Cookie 接入；订阅博主；可添加登录账号“我的点赞”来源；保存正文、原图和可选视频 |
| Pixiv | 使用已登录浏览器的请求头信息接入；订阅画师；可添加账号“P站收藏”来源，并自动附加 `#P站收藏` 标签 |
| Twitter | 已预留平台入口与数据模型，账号连接和采集器尚未开放 |

平台接口和风控策略可能发生变化。Lumic 已对图片下载进行节流，但实际可用性仍取决于账号状态、网络环境和平台限制。

## Docker Compose 部署

### 1. 准备目录

创建一个部署目录，并在其中保存下面的 `docker-compose.yml`。建议同时创建三个持久化目录：

```powershell
New-Item -ItemType Directory -Force data, flow, previews
```

Linux 或 macOS：

```bash
mkdir -p data flow previews
```

### 2. Compose 配置

```yaml
services:
  lumic:
    image: ghcr.io/hyaeve/lumic:latest
    container_name: lumic
    ports:
      - "15500:5500"
    volumes:
      # 配置、密钥、账号凭证、内容索引和同步状态。
      - ./data:/data
      # 动态正文、原始图片、视频和专栏内容。
      - ./flow:/flow
      # 列表浏览时按需生成的预览图，可独立清理。
      - ./previews:/previews
    environment:
      TZ: Asia/Shanghai
      LUMIC_PREVIEW_ROOT: /previews
    restart: unless-stopped
```

平台账号凭证、项目代理和登录账号均在 Lumic 设置页中管理，不需要在 Compose 中配置 Pixiv Client ID、Client Secret 或平台 Cookie。

### 3. 启动

```bash
docker compose pull
docker compose up -d
```

启动后访问：`http://localhost:15500`

首次启动的初始账号和密码均为 `Lumic`。登录后请立即前往设置页修改账号和密码。

### 4. 常用命令

查看运行状态与日志：

```bash
docker compose ps
docker compose logs -f lumic
```

更新到最新镜像：

```bash
docker compose pull
docker compose up -d
```

停止服务：

```bash
docker compose down
```

`docker compose down` 不会删除上述绑定目录。不要使用会主动删除宿主机数据目录的清理命令。

### 可选环境变量

| 变量 | 用途 | 默认值 |
| --- | --- | --- |
| `TZ` | 容器时区，影响界面时间和 Cron 执行时间 | 系统时区 |
| `LUMIC_PREVIEW_ROOT` | 预览图缓存目录 | `/previews` |
| `LUMIC_FLOW_ROOT` | 动态正文与原始媒体目录 | `/flow` |
| `LUMIC_COOKIE_SECURE` | HTTPS 反向代理后设为 `true`，令登录 Cookie 仅通过 HTTPS 发送 | `false` |
| `LUMIC_AUTH_FILE` | 登录账号配置文件路径 | `/data/auth.json` |
| `LUMIC_CONTENT_FILE` | 动态索引与应用状态文件路径 | `/data/content.json` |
| `LUMIC_BILIBILI_FILE` | 平台加密凭证文件路径 | `/data/bilibili.enc` |
| `LUMIC_SECRET_FILE` | AES-GCM 本地密钥文件路径 | `/data/secret.key` |
| `LUMIC_IMAGE_DOWNLOAD_DELAY_MS` | 平台图片请求之间的最小间隔，用于降低触发风控的概率 | 内置节流值 |

通常只需保留推荐 Compose 中的 `TZ` 和 `LUMIC_PREVIEW_ROOT`。修改存储路径时，应同步调整对应的卷挂载。

## 登录与安全

- 浏览器登录会话有效期为 24 小时，服务重启后现有会话也会失效，需要重新登录。
- Web 会话使用 `HttpOnly`、`SameSite=Lax` Cookie；部署在 HTTPS 后时建议启用 `LUMIC_COOKIE_SECURE=true`。
- Lumir 等客户端通过 `/api/v1/auth/login` 获取 24 小时 Bearer Token；服务重启后令牌失效。
- 平台 Cookie、Token 和请求头不会返回给前端，服务使用 AES-GCM 加密后写入 `/data/bilibili.enc`，密钥写入 `/data/secret.key`。
- `data` 目录包含账号、密钥与连接凭证，应限制访问权限、定期备份，并且不要提交到 Git 仓库。

## 平台接入

平台账号统一在“设置 → 平台凭证”中管理，右键平台卡片可打开详细配置。

### 哔哩哔哩

推荐使用手机客户端扫码登录，也可手动导入有效 Cookie。连接后可搜索并添加 UP 主，添加来源本身不会立即拉取动态。UP 主来源默认采集图文动态，可在来源设置中额外启用视频动态和专栏。

账号的“收藏专栏”可作为独立来源添加。该来源用于手动获取历史动态和后续增量同步，不会与普通 UP 主目录混合。

### 微博

支持账号密码、移动端扫码和手动 Cookie。账号密码仅用于当次换取登录会话，不会以明文写入项目配置；遇到验证码或二次验证时请改用扫码或 Cookie。

登录后可订阅博主，也可将“我的点赞”添加为一个普通来源。它参与统一动态流，但不自动归入 Lumic 侧栏的“收藏”。

### Pixiv

从已登录 Pixiv 的浏览器开发者工具中取得并填写 User-Agent、Baggage、Cookie、用户 ID，以及页面要求的可选请求头。连接后可订阅画师，也可添加账号的“P站收藏”来源。

“P站收藏”来源默认带有 `#P站收藏` 标签，归档内容统一保存在该来源目录中。

## 来源与同步规则

每个来源都可以独立配置：

- 启用或停用自动同步。
- Cron 表达式，默认 `0 6 * * *`，即每天 06:00。
- 标签，多个标签使用空格分隔；保存后会应用到该来源已有和后续拉取的全部动态。
- 包含关键词和排除关键词，多个关键词使用空格分隔。
- “包含图片”：启用后排除纯文字动态。
- “包含视频”：将符合条件的视频动态纳入拉取范围。
- 哔哩哔哩来源可额外选择是否包含专栏。

添加订阅只创建来源，不会自动触发历史拉取：

- “立即同步”或“全部拉取最新”只获取各来源的新内容。
- “获取历史动态”由用户手动触发；符合当前来源设置、且本地尚未完整归档的内容会被拉取，已经完整归档的动态会自动跳过。
- 获取历史动态同样遵循来源的图片、视频、专栏和关键词过滤设置，可用于断点补全本地内容。

## 动态浏览

- “全部动态、今日动态、收藏动态”统计会随当前平台筛选变化。
- 搜索同时匹配正文、作者名称和标签。
- 支持最新/最早排序，进入作者页或标签页后仍可继续搜索和排序。
- 列表模式展示完整动态信息；瀑布流模式以第一张图片作为封面，桌面端自适应 3–5 列，手机端固定 2 列。
- 多图动态最多展示九宫格，超过九张时最后一格显示剩余数量，完整内容可在详情或大图模式中查看。
- 删除动态时会同时删除关联的本地正文、原始媒体和预览缓存；批量删除提供二次确认。

## 数据目录

### `/data`

保存 Lumic 的运行状态，包括：

- 登录账号配置。
- AES-GCM 密钥与加密后的平台凭证。
- 订阅来源、过滤规则、同步游标和任务状态。
- 动态索引、收藏状态与其他应用配置。

### `/flow`

保存采集到的正文和原始媒体，基本结构如下：

```text
/flow/
├─ bilibili/
│  ├─ UP主名称/
│  │  ├─ post_contents.txt
│  │  └─ 原始媒体文件
│  └─ 收藏专栏/
│     ├─ post_contents.txt
│     └─ 原始媒体文件
├─ weibo/
│  ├─ 博主名称/
│  │  ├─ post_contents.txt
│  │  └─ 原始媒体文件
│  └─ 我的点赞/
│     ├─ post_contents.txt
│     └─ 原始媒体文件
└─ pixiv/
   ├─ 画师名称/
   │  ├─ post_contents.txt
   │  └─ 原始图片
   └─ P站收藏/
      ├─ post_contents.txt
      └─ 原始图片
```

普通订阅以作者为目录，一个作者使用一个 `post_contents.txt` 持续记录动态正文，而不是每条动态创建一个文本文件。微博“我的点赞”、Pixiv“P站收藏”和哔哩哔哩“收藏专栏”等账号集合来源统一写入各自来源目录，不再按原动态作者拆分文件夹。

正文中的 Emoji 作为 Unicode 文本写入 `post_contents.txt`，不会单独下载为图片。

图片按作者与发布日期命名：

```text
作者-20260813.jpg
作者-20260813·1.jpg
作者-20260813·2.jpg
```

同一动态仅一张图片时不加序号；多张图片从 `·1` 开始编号。Lumic 保存平台可获取的原图，不会用列表预览图替换 `/flow` 中的原始文件。

### `/previews`

动态列表不会直接加载原图，而是在 `/previews` 中按需生成最长边 900 像素、JPEG 质量 88 的预览图。进入大图模式时仍读取 `/flow` 中的原始文件。

预览缓存回收规则：

- 删除动态或原图时同步删除关联预览。
- 服务启动时以及每 12 小时清理孤儿文件、旧版本缓存和超过 1 小时的临时文件。
- 连续 30 天未使用的预览会被删除，下次浏览时自动重建。
- 已访问预览的使用时间最多每 24 小时更新一次，减少磁盘写入。
- `./previews` 可以独立清空，不会影响 `/flow` 中的正文与原始媒体。

## 网络代理

“设置 → 网络代理”支持 `http://`、`https://`、`socks5://` 和 `socks5h://`，可供平台登录、验证与采集共用。

容器内的 `127.0.0.1` 指向容器本身。代理运行在 Docker 宿主机的 `7890` 端口时，应填写：

```text
http://host.docker.internal:7890
```

或：

```text
socks5://host.docker.internal:7890
```

代理程序还需要允许 Docker 网络访问。设置页提供连接测试，保存时只显示脱敏后的代理地址。

## 配置备份与恢复

设置页提供配置备份和恢复。备份前请确认其中是否包含敏感账号配置，并将备份文件保存在可信位置。媒体归档和预览目录体积通常较大，应通过宿主机文件备份单独保护 `flow`；`previews` 可随时重新生成。

## Lumir 客户端 API

面向 Lumir 安卓客户端的稳定接口位于 `/api/v1`，客户端健康检查位于 `/api/v1/health`。完整字段和请求示例见 [`docs/api-v1.md`](docs/api-v1.md)。

核心接口：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/v1/auth/login` | 使用 Lumic 账号密码换取 Bearer Token |
| `GET` | `/api/v1/auth/session` | 检查客户端令牌会话 |
| `POST` | `/api/v1/auth/logout` | 注销当前令牌 |
| `GET` | `/api/v1/posts?limit=30&cursor=...` | 游标分页读取动态，默认 30 条，单页最多 100 条 |
| `GET` | `/api/v1/feeds` | 获取统一订阅来源 |
| `POST` | `/api/v1/sync` | 触发所有已启用来源的增量同步 |
| `GET` | `/api/v1/health` | 客户端 API 健康检查 |

动态接口支持 `source`、`liked`、`author`、`tag`、`q` 和最新/最早顺序等筛选条件。受保护接口使用：

```http
Authorization: Bearer <token>
```

客户端应持续使用响应中的游标加载下一页，不要一次请求全部动态。媒体响应同时提供适合列表的预览资源和查看模式使用的原始资源。

## 本地开发

需要 Node.js 22、Go 1.22 以及 npm。

启动后端：

```bash
cd backend
go run .
```

另开终端启动前端：

```bash
cd frontend
npm install
npm run dev
```

后端默认监听 `http://localhost:5500`，Vite 开发服务器默认监听 `http://localhost:5173`。

执行检查与构建：

```bash
cd backend
go test ./...

cd ../frontend
npm run build
```

本地构建容器镜像：

```bash
docker build -t lumic:local .
```

如需使用本地镜像启动，将 Compose 中的 `image` 临时改为 `lumic:local` 后执行 `docker compose up -d`。

## 镜像发布

GitHub Actions 工作流 [`.github/workflows/docker-publish.yml`](.github/workflows/docker-publish.yml) 会在以下情况构建镜像：

- 推送到 `main` 分支。
- 推送 `v*` 版本标签。
- 手动触发工作流。
- Pull Request 仅执行构建验证，不发布镜像。

发布镜像支持 `linux/amd64` 和 `linux/arm64`，目标为 `ghcr.io/hyaeve/lumic`。

## 使用说明

Lumic 用于归档你本人有权访问的内容。请遵守各平台服务条款、版权要求、访问频率限制和当地法律，不要公开分发他人的受限内容或泄露账号凭证。

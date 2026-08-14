# Lumic · 拾光

Lumic 是一个聚合个人内容流的 Docker 项目，中文名为「拾光」。它将微博、pixiv、哔哩哔哩中关注的内容整理为一条按时间排序的时间线，并保留动态作者、头像、配文、标签、媒体和来源信息。

## 当前版本

当前仓库包含可运行的 Vue 前端仪表盘和 Go API 基础服务。后端目前使用演示动态作为数据层占位；B 站已经支持加密保存账号凭证、搜索 UP 主和持久化图文/专栏订阅，实际动态同步解析器以及微博、pixiv 连接器仍需继续接入。

## 启动开发环境

```bash
cd frontend
npm install
npm run dev
```

另开终端启动 API：

```bash
cd backend
go run .
```

API 默认监听 `http://localhost:5500`。Vite 开发服务器默认监听 `http://localhost:5173`；开发时可通过反向代理或将 API 服务映射到同源路径。

## API

- `GET /api/posts?source=all|weibo|pixiv|bilibili`：获取按发布时间倒序的动态
- `GET /api/feeds`：获取来源与同步配置
- `POST /api/feeds`：新增一个来源配置
- `POST /api/sync`：触发同步任务
- `GET|PUT /api/project/settings`：读取脱敏项目设置或保存项目代理
- `POST /api/project/settings`：测试代理能否访问 pixiv
- `GET|PUT /api/bilibili/account`：读取脱敏配置状态或手动验证并保存 B 站凭证
- `GET|POST /api/bilibili/qr`：创建 B 站登录二维码或携带 `id` 轮询扫码状态
- `GET|PUT /api/pixiv/account`：读取 Pixiv 连接状态或验证并加密保存浏览器请求头凭证
- `GET|POST /api/weibo/qr`：读取微博账号状态、创建二维码或携带 `id` 轮询扫码状态
- `GET|PUT /api/weibo/account`：读取微博连接状态，或通过账号密码 / Cookie 建立并加密保存会话
- `GET /api/bilibili/search?keyword=名称`：搜索 B 站 UP 主
- `GET|POST /api/bilibili/subscriptions`：读取或新增 UP 主图文/专栏订阅
- `GET /api/health`：健康检查

## Docker

项目已配置使用 GitHub Container Registry 镜像 [`ghcr.io/hyaeve/lumic:latest`](https://ghcr.io/hyaeve/lumic)。Compose 中宿主机端口 `15500` 映射到容器端口 `5500`，因此访问地址为 `http://localhost:15500`；容器内 API 和前端仍由 `5500` 提供服务。

```bash
mkdir data flow
docker compose pull
docker compose up -d
docker compose logs -f lumic
```

Compose 使用三个职责不同的持久化挂载：

- `./data:/data`：保存应用基础设置、加密密钥、平台账号凭证、订阅配置、数据库、同步游标和任务状态。这是应用内部状态目录，不保存采集到的正文与媒体。
- `./flow:/flow`：保存平台采集的动态、图文、专栏正文和媒体文件。可通过 `LUMIC_FLOW_ROOT` 修改容器内根路径，Compose 默认固定为 `/flow`。
- `./previews:/previews`：保存动态列表按需生成的压缩预览图，可独立清理或重建。可通过 `LUMIC_PREVIEW_ROOT` 修改容器内缓存路径，Compose 默认固定为 `/previews`。

`/flow` 按“平台 → 作者”组织：

```text
/flow/
├─ bilibili/
│  └─ Comelee/
│     ├─ source.json
│     └─ （后续同步的动态图文、专栏及媒体）
├─ pixiv/
│  └─ 画师名称/
│     └─ （作品元数据与图片）
└─ weibo/
   └─ 博主名称/
      └─ （微博正文与媒体）
```

服务启动时会自动创建 `/flow/bilibili`、`/flow/pixiv` 和 `/flow/weibo`。新增 B 站 UP 主订阅时，会立即创建对应的作者目录，并写入不含账号凭证的 `source.json`。已有 B 站订阅会在升级后的首次启动时自动补建目录。作者名称中的路径分隔符、Windows 非法字符及保留设备名会被安全替换，避免目录穿越和跨平台挂载失败。

浏览动态产生的预览缓存不会写入 `/flow`。服务会在 `/previews` 中按平台和作者镜像原图路径，并在升级后自动清理旧的 `/flow/.previews` 缓存目录。

本地构建镜像时使用：

```bash
docker compose build
docker compose up -d
```

生产环境建议将数据库、平台授权 Cookie / Token 和加密密钥通过 Docker secrets 或环境变量注入，不要写入镜像或提交到仓库。登录账号和密码不再由 Compose 环境变量配置，而是在登录后通过设置页面修改并保存到挂载的 `/data/auth.json`。

### 登录与安全

应用启动后会先显示空白登录表单，页面不会自动填写、展示或提示账号密码。所有 `/api` 业务接口均要求通过服务端会话认证；会话 Cookie 使用 `HttpOnly` 和 `SameSite=Strict`，不会暴露给前端脚本。

首次启动使用内置初始账号后，在左侧「设置」中输入当前密码并设置新账号和新密码。新密码至少 8 位；修改结果保存在 Compose 挂载的 `./data/auth.json` 中，重建或更新容器不会丢失。登录表单不会自动填写或提示初始账号密码。

反向代理启用 HTTPS 后建议设置 `LUMIC_COOKIE_SECURE=true`；该变量不属于当前 Compose 默认配置，可按部署环境单独添加。服务还发送 CSP、`X-Frame-Options: DENY`、`X-Content-Type-Options: nosniff` 和严格 Referrer Policy 等响应头。

平台账号统一在设置页管理。B 站默认使用手机客户端扫码登录：后端先访问 B 站主页建立设备会话，再生成二维码并轮询确认状态；登录成功后自动从响应 Cookie 中提取并加密保存所需凭证。手动 Cookie 导入仅作为高级备用方式。Pixiv 改为浏览器请求头接入：从已登录 Pixiv 的浏览器开发者工具中复制 User-Agent、Baggage、Cookie、用户 ID，以及可选的 Sentry-Trace 和 X-CSRF-TOKEN，在平台凭证卡片中验证并加密保存。微博支持账号密码、移动端扫码和手动 Cookie 三种登录方式。账号密码只用于当次向微博交换会话，不会写入本地配置；若微博要求验证码或二次安全验证，页面会提示改用扫码。成功后服务端仅加密保存会话 Cookie。

保存前服务会验证平台登录状态。原始请求头、token 与 Cookie 不会返回前端，而是随平台配置通过 AES-GCM 加密保存在 `/data/bilibili.enc`，密钥保存在 `/data/secret.key`。应将整个 `data` 目录视为敏感数据，限制宿主机访问权限并定期备份；不要提交到 Git 仓库。Pixiv 和微博接口策略可能随平台调整，使用时应遵守各平台服务条款，并使用自己有权访问的账号。

### 项目代理

「设置 → 网络代理」可配置供所有平台连接器复用的项目代理，支持 `http://`、`https://`、`socks5://` 和 `socks5h://`，也支持在 URL 中携带用户名与密码。设置页只显示脱敏地址，修改代理时需要重新输入完整地址。

Docker 容器中的 `127.0.0.1` 指向容器自身，并不是宿主机。如果代理软件运行在宿主机的 `7890` 端口，应填写：

```text
http://host.docker.internal:7890
```

或：

```text
socks5://host.docker.internal:7890
```

代理软件还必须允许来自 Docker 网络的连接。保存前可以使用「测试连接」检查是否能够访问 pixiv；保存后 B 站凭证验证、UP 主搜索以及后续 Pixiv connector 都会使用该代理。

### 来源详细设置

「设置 → 来源管理」展示所有来源卡片。桌面端可右键卡片打开详细设置，触屏设备可点击「详细设置」。B 站来源可以调整启用状态、执行计划和首次历史拉取策略，但内容范围固定为图文动态与专栏，不能开启视频采集。

### GitHub Actions

工作流 [`docker-publish.yml`](.github/workflows/docker-publish.yml) 会在 `main` 分支推送、`v*` 标签和手动触发时构建并推送镜像；Pull Request 只执行构建校验，不推送镜像。默认构建 `linux/amd64` 与 `linux/arm64`，并使用 `GITHUB_TOKEN` 登录 GHCR。

仓库 Settings → Actions → General 中应允许 Actions 创建和写入 packages；如果发布失败，请确认工作流权限包含 `packages: write`，并确保仓库归属账号与镜像路径中的 `hyaeve` 一致。

## 采集器实现建议

平台采集应优先使用官方开放接口或获得授权的账号会话，并遵守平台服务条款、robots 规则、访问频率限制和隐私要求。建议将每个平台实现为独立 connector：

- 微博：点赞流、指定博主、历史首次拉取、增量同步
- pixiv：关注画师作品流、标签和作品媒体
- 哔哩哔哩：按昵称搜索并订阅指定 UP 主，仅允许图文动态与专栏；视频投稿、视频动态和转发视频卡片必须过滤

调度器保存每个来源的游标和最后同步时间；首次同步通过 `includePast` 选择是否拉取历史全部，后续仅拉取游标之后的新内容。

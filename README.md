# Lumic · 拾光

Lumic 是一个聚合个人内容流的 Docker 项目，中文名为「拾光」。它将微博、pixiv、哔哩哔哩中关注的内容整理为一条按时间排序的时间线，并保留动态作者、头像、配文、标签、媒体和来源信息。

## 当前版本

当前仓库包含可运行的 Vue 前端仪表盘和 Go API 基础服务。后端目前使用演示数据作为数据层占位，平台连接器、登录授权和持久化数据库可在此基础上接入。

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
- `GET /api/health`：健康检查

## Docker

项目已配置使用 GitHub Container Registry 镜像 [`ghcr.io/hyaeve/lumic:latest`](https://ghcr.io/hyaeve/lumic)。Compose 中宿主机端口 `15500` 映射到容器端口 `5500`，因此访问地址为 `http://localhost:15500`；容器内 API 和前端仍由 `5500` 提供服务。

```bash
mkdir data
docker compose pull
docker compose up -d
docker compose logs -f lumic
```

数据目录挂载为 `./data:/data`：左侧是运行 `docker compose` 的目录下的宿主机 `data` 文件夹，右侧是容器内路径。当前演示版本尚未写入持久化数据；后续数据库、Cookie 和任务状态建议统一放在 `/data`。

本地构建镜像时使用：

```bash
docker compose build
docker compose up -d
```

生产环境建议将 `LUMIC_DATABASE_URL`、平台授权 Cookie / Token 和加密密钥通过 Docker secrets 或环境变量注入，不要写入镜像或提交到仓库。

### GitHub Actions

工作流 [`docker-publish.yml`](.github/workflows/docker-publish.yml) 会在 `main` 分支推送、`v*` 标签和手动触发时构建并推送镜像；Pull Request 只执行构建校验，不推送镜像。默认构建 `linux/amd64` 与 `linux/arm64`，并使用 `GITHUB_TOKEN` 登录 GHCR。

仓库 Settings → Actions → General 中应允许 Actions 创建和写入 packages；如果发布失败，请确认工作流权限包含 `packages: write`，并确保仓库归属账号与镜像路径中的 `hyaeve` 一致。

## 采集器实现建议

平台采集应优先使用官方开放接口或获得授权的账号会话，并遵守平台服务条款、robots 规则、访问频率限制和隐私要求。建议将每个平台实现为独立 connector：

- 微博：点赞流、指定博主、历史首次拉取、增量同步
- pixiv：关注画师作品流、标签和作品媒体
- 哔哩哔哩：关注 UP 主动态、视频 / 图文动态和标签

调度器保存每个来源的游标和最后同步时间；首次同步通过 `includePast` 选择是否拉取历史全部，后续仅拉取游标之后的新内容。

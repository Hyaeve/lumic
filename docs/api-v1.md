# Lumic API v1（Lumir 客户端接入）

Lumic 的 `/api/v1` 是面向 Lumir 等原生客户端的稳定接口。Docker Compose 默认将宿主机 `15500` 端口映射到服务端 `5500`，因此局域网中的基础地址通常为：

```text
http://<Lumic 服务端 IP>:15500
```

公网使用时应在 Lumic 前配置 HTTPS 反向代理。Android 9 及以上默认禁止明文 HTTP；仅在局域网调试时，才应通过 Network Security Config 单独允许对应地址。

## 服务发现

以下接口不要求登录：

```http
GET /api/v1
GET /api/v1/health
```

`GET /api/v1` 返回 API 版本、认证方式、会话时长和分页上限。

## Bearer Token 登录

```http
POST /api/v1/auth/login
Content-Type: application/json

{
  "username": "Lumic",
  "password": "你的密码"
}
```

成功响应：

```json
{
  "accessToken": "opaque-token",
  "tokenType": "Bearer",
  "expiresIn": 2592000,
  "expiresAt": "2026-08-16T12:00:00Z"
}
```

后续请求统一携带：

```http
Authorization: Bearer <accessToken>
```

Token 从签发时起有效 30 天，不会随访问或重启续期。保留 Lumic 的 `/data` 数据卷即可在容器重启后继续使用。退出只撤销当前 Token；修改服务端账号凭证会使既有会话失效。旧版本仅存于内存的 Token 无法迁移，升级后需要重新登录一次。安卓端应将 Token 保存到 EncryptedSharedPreferences 或 Android Keystore，不要写入普通日志、URL 查询参数或明文数据库。

登录状态与退出：

```http
GET  /api/v1/auth/session
POST /api/v1/auth/logout
```

## 动态游标分页

```http
GET /api/v1/posts?limit=30&order=newest
Authorization: Bearer <accessToken>
```

响应：

```json
{
  "items": [
    {
      "id": "weibo-123",
      "source": "weibo",
      "author": "作者",
      "avatar": "/flow/weibo/作者/avatar.jpg",
      "caption": "动态正文😀",
      "tags": ["标签"],
      "media": ["/flow/weibo/作者/作者-20260815.jpg"],
      "previewMedia": ["/preview/weibo/作者/作者-20260815.jpg"],
      "videos": [],
      "previewVideos": [],
      "originalUrl": "https://weibo.com/...",
      "published": "2026-08-15T10:30:00+08:00",
      "liked": false
    }
  ],
  "nextCursor": "opaque-cursor",
  "hasMore": true,
  "limit": 30
}
```

下一页只需原样回传服务端给出的游标：

```http
GET /api/v1/posts?limit=30&order=newest&cursor=<nextCursor>
```

不要解析或自行修改游标。游标包含稳定的发布时间与动态 ID 边界，并绑定当前筛选条件；拉取下一页期间即使新增了更晚的动态，也不会造成重复或跳页。

支持的查询参数：

| 参数 | 说明 |
| --- | --- |
| `limit` | 每页数量，默认30，最大100 |
| `cursor` | 上一页返回的 `nextCursor` |
| `order` | `newest` 或 `oldest` |
| `source` | `all`、`weibo`、`bilibili`、`pixiv`、`twitter` |
| `liked` | `true` 或 `false` |
| `author` | 作者名称，精确匹配且忽略大小写 |
| `tag` | 标签名称，可带或不带 `#` |
| `q` | 搜索正文、作者和标签 |

刷新时间线时丢弃旧游标，从第一页重新请求；向下加载时沿用当前筛选参数和 `nextCursor`。

## 动态操作

收藏或取消收藏：

```http
PATCH /api/v1/posts?id=<动态ID>
Content-Type: application/json

{"liked": true}
```

批量修改收藏：

```http
PATCH /api/v1/posts
Content-Type: application/json

{"ids": ["id-1", "id-2"], "liked": false}
```

删除动态及关联文件：

```http
DELETE /api/v1/posts?id=<动态ID>
```

批量删除：

```http
DELETE /api/v1/posts
Content-Type: application/json

{"ids": ["id-1", "id-2"]}
```

重置单条动态（需要先在客户端确认覆盖编辑）：

```http
POST /api/v1/posts/reset
Content-Type: application/json

{"id": "weibo-status-123456"}
```

重新获取该条平台正文与媒体并归档，覆盖手动编辑，保留本地收藏、标签与来源关联。成功返回 `{"post": <更新后的动态>, "message": "动态已重新拉取"}`。该旧入口仍与请求生命周期绑定，批量操作应改用下方后台任务，不应先删除旧动态。并发重置或动态已被其他操作更改时返回 409；上游拒绝、删除或媒体下载失败返回 502，原动态保留。未关联有效平台编号的样例动态返回 400。沿用各平台鉴权和节流，推特仍至少间隔 5 秒。

批量重置（客户端确认后一次提交全部编号）：

```http
POST /api/v1/posts/reset/tasks
Content-Type: application/json

{"ids": ["weibo-status-123456", "weibo-status-234567"]}
```

返回 HTTP 202 和任务对象；最多 1000 个去重编号，按提交顺序串行重新拉取。同一时间只执行一个批次；相同编号集合的重复提交复用活动任务，不同集合返回 409。后台任务不依赖网页连接，切换侧栏、关闭网页或退出登录都不取消已接收的任务；执行每一条前重新读取动态，沿用旧入口的重置互斥、鉴权、平台归档锁、节流和失败保留策略。重置仍为单独的串行工作流，不额外增加并发，不占用订阅同步队列的 5 个名额。

重新打开页面后可读取活动任务与最近 32 个终态任务，无需保存任务编号：

```http
GET /api/v1/posts/reset/tasks
```

返回 `{"tasks": [...]}`，活动任务排在最前，其余按完成时间倒序。任务字段为 `id`、`status`、`total`、`completed`、`failed`、`message`、`postIds`、`updatedIds`；`completed` 是已处理数量（包含失败），`updatedIds` 是已成功归档的编号。`status` 为 `running`、`completed` 或 `failed`，仅全失败时为 `failed`，部分失败仍为 `completed`，请结合 `failed` 判断结果。消息不包含上游响应正文或凭证。所有接口均需登录且返回 `Cache-Control: no-store`，网页兼容路径为 `/api/posts/reset/tasks`。任务状态只保留在服务进程内，不跨容器重启恢复；已完成的动态归档正常持久化。

## 来源与同步

统一读取所有平台来源：

```http
GET /api/v1/feeds
```

该接口不会返回容器内部的 `storagePath`。全量增量同步入口：

```http
POST /api/v1/sync
```

平台账号、搜索、订阅设置以及单来源 `sync` / `resync` 操作均提供 V1 路径：

```text
/api/v1/bilibili/...
/api/v1/weibo/...
/api/v1/pixiv/...
```

路径和请求体与 Lumic 网页端对应接口一致。

### 同步任务与排队

网页及新客户端应使用短请求提交，避免等待中的同步占满浏览器连接：

```http
POST /api/v1/sync/tasks
Content-Type: application/json

{"sourceId":"weibo-12345","full":false}
```

返回202及任务 `{id,sourceId,status,full}`；`full:true` 表示历史拉取。状态为 `queued`、`running`、`completed` 或 `failed`，终态还包含 `message` 和可用的 `source`。同一订阅同模式的活跃任务复用同一ID，模式冲突返回409，不重复下载。来源在实际开始执行前重新读取，已删除的订阅不会继续执行。

```http
GET /api/v1/sync/status?ids=<任务ID1>,<任务ID2>
```

返回 `{"tasks":[...]}`，包含所有排队/运行任务，以及显式查询的已结束任务。省略ids只返回活跃任务；最多保留最近256条终态，队列仅存在当前进程内，重启后旧ID会失效。接口均禁止缓存，也有对应的 `/api/sync/tasks` 和 `/api/sync/status` 网页路径。

订阅同步任务总并发最多5个；微博、Pixiv、哔哩哔哩分别最多2个，推特最多1个，超限按可执行顺序排队，一个平台满额不阻塞其他平台。平台原有请求间隔及访问限制不变；同平台共享目录的媒体归档串行，避免作者订阅和收藏来源相互覆盖。旧同步等待接口、全部同步、定时同步和旧微博点赞接口使用相同队列。旧等待请求断开只结束等待，不取消已提交任务。

微博来源支持可选布尔字段 `includeLivePhotos`，省略时默认开启。使用 `PUT /api/v1/weibo/subscriptions` 传入 `false` 后，后续同步和单条重置只下载静态原图，不请求实况运动片段，也不提示缺失片段；普通视频仍由 `includeVideos` 独立控制。旧客户端更新来源时省略该字段会保留已有设置。关闭不会删除已归档的实况文件，重新开启后可用历史拉取补齐未归档片段。

## 图片加载

列表优先使用 `previewMedia` 和 `previewVideos[].poster`，进入大图或播放模式后再使用 `media`、`videos` 中的原始资源。所有以 `/flow/` 或 `/preview/` 开头的地址都需要拼接 Lumic 基础地址；外部 `https://` 地址保持原样。

推荐 Lumir 使用 OkHttp 拦截器统一添加 `Authorization`，使用 Paging 3 的 `nextCursor` 作为 RemoteMediator 或 PagingSource 的下一页键。

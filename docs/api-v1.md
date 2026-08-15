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
  "expiresIn": 86400,
  "expiresAt": "2026-08-16T12:00:00Z"
}
```

后续请求统一携带：

```http
Authorization: Bearer <accessToken>
```

Token 在24小时后失效；Lumic 容器重启后也会立即失效。安卓端应将 Token 保存到 EncryptedSharedPreferences 或 Android Keystore，不要写入普通日志、URL 查询参数或明文数据库。

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

## 图片加载

列表优先使用 `previewMedia` 和 `previewVideos[].poster`，进入大图或播放模式后再使用 `media`、`videos` 中的原始资源。所有以 `/flow/` 或 `/preview/` 开头的地址都需要拼接 Lumic 基础地址；外部 `https://` 地址保持原样。

推荐 Lumir 使用 OkHttp 拦截器统一添加 `Authorization`，使用 Paging 3 的 `nextCursor` 作为 RemoteMediator 或 PagingSource 的下一页键。

# QZone 数据源适配

`onebot-qzone`（Gu-Heping）作为 QQ 空间的独立数据源桥接器使用。它通过 OneBot v11 提供空间动态、评论、点赞、访客、好友、用户资料、头像和事件推送能力。

## 接入边界

它不替代 NapCat 的 QQ 消息 WS/HTTP 连接，也不直接写入本项目数据库。推荐拓扑：

```text
NapCat WS/HTTP ───────────────┐
                              ├─ Go 采集与归一化 ─ PostgreSQL / 对象存储
onebot-qzone HTTP/WS sidecar ─┘
```

Go 后端通过 OneBot v11 HTTP 调用只读 action，并通过事件 WS 接收动态、评论和点赞事件。所有请求响应先进入 `raw_records`，之后再转换为 `contents`、`relation_events` 和 `media_references`。

## 推荐配置

```text
onebot-qzone bridge: 127.0.0.1:5700
event websocket:     ws://127.0.0.1:5700/event
主平台后端:          127.0.0.1:8000
```

项目已将上游仓库固定在 `integrations/onebot-qzone`。本地启动：

```bash
./scripts/start-qzone.sh
```

启动脚本会先从本机 NapCat HTTP API 获取 `qzone.qq.com` 的临时 Cookie，并只通过进程环境交给桥接器，不写入项目配置。NapCat 无法提供登录态时，才会在 `data/qzone-cache/qrcode.png` 生成登录二维码。

上游的 NapCat 插件只代理 bridge 的 HTTP/WS，不负责传递 Cookie。当前 NapCat 版本还会拒绝非官方白名单插件，所以本项目直接连接 `127.0.0.1:5700`，并由启动脚本复用 NapCat 登录态。

## 第一批只读能力

- `get_login_info`
- `get_stranger_info`
- `get_friend_list`
- `get_emotion_list`
- `get_msg`
- `get_comment_list`
- `get_like_list`
- `get_visitor_list`
- `get_album_list`
- `get_photo_list`

发布、删除、点赞、评论和隐私设置仍然必须走操作中心确认，不进入自动采集任务。

## 可靠性说明

QZone 上游接口会限流、变更或返回 HTML。`onebot-qzone` 已有 feeds3 解析、降级路径、原始响应日志和事件轮询，因此应保留其作为独立 sidecar，而不是复制其逆向实现。采集器必须记录 action、参数摘要、桥接器响应、可靠性标记和原始证据。

项目地址：[Gu-Heping/onebot-qzone](https://github.com/Gu-Heping/onebot-qzone)

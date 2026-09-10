# MiAir Plus · Go

将小爱音箱接入 DLNA / AirPlay 1 音频投送的 Go 原生桥接服务，提供简洁的磨砂风格管理页面和飞牛 fnOS 安装包。

**当前为 `2.0.0-alpha.2` 预览版。已做协议与解码自动测试，尚未完成 fnOS、真实小爱音箱、QQ 音乐／网易云／OPPO／Apple 发送端的实机验收。不要把预览版当作全型号兼容的稳定版。**

## 黑屏修复与通用安装包

`alpha.2` 使用 `go:embed all:dist` 打包以下划线开头的前端辅助模块，修复 `_plugin-vue_export-helper` 返回 404 引起的黑屏。升级后请强制刷新页面（Ctrl+Shift+R）；入口 HTML 不再缓存。通用包内同时携带两个架构的 Go 二进制，由启动脚本按 CPU 自动选择。

## 这次改了什么

- 管理 API、小米账号通信、DLNA、RAOP 接收及服务管理使用 Go；单个原生进程，不运行 Python，不依赖 Docker。
- 前端保留 Vue 3，编译后嵌入 Go 二进制，无需在 NAS 安装 Node.js。
- 名称保存先原子写入本地，再在后台更新广播；改名不等待小米云，也不重建正在播放的 AirPlay 会话。
- 每台音箱的播放命令串行执行，新操作取消旧请求，旧返回不能覆盖新状态。云请求超时不伪装为“停止”。
- DLNA 使用共享 SSDP 发现端口、UPnP 事件订阅和 HTTP Range 流式代理；音频不整首载入内存。
- RAOP 提供 UDP / TCP 音频、RSA / AES、限量乱序重排、丢包请求和固定大小音频缓冲。
- 管理页面移除手动播放控制，播放、暂停、切歌从音乐 App 操作。

## 兼容范围

| 项目 | 当前范围 |
| --- | --- |
| NAS 架构 | 单个通用 FPK 内含 Linux amd64 / arm64，启动时自动选择；ARM32 不支持 |
| DLNA | SSDP、AVTransport、RenderingControl、ConnectionManager、GENA |
| QQ 音乐、网易云 | 实现其可使用的通用 DLNA 音频接口；具体 App 版本需实测 |
| OPPO 系统投送 | 仅当发送端提供 DLNA **音频**输出；不是 Miracast / 屏幕镜像接收器 |
| AirPlay | 传统 AirPlay 1 / RAOP；ALAC 与 L16、未加密或 RSA/AES 音频 |
| 暂不支持 | AirPlay 2、多房间同步、屏幕镜像、FairPlay 加密流、AAC RAOP |
| 小米音箱 | 小米云能列出的设备；提供两种 MiNA 播放接口与兼容模式，不承诺全型号 |
| 解码与拖动进度 | 使用本机 FFmpeg，需有 ALAC 解码和 libmp3lame 编码能力 |

FFmpeg 缺失时 DLNA 直接音频代理仍可使用，AirPlay 不会被错误地广播为可用。某些现代 Apple 发送端只使用 FairPlay，因此可能无法连接此预览版；这项兼容能力尚未迁移完整。

## 安装到飞牛

1. 从 Releases 下载 `miair-plus-2.0.0-alpha.2-all.fpk`，x86_64 与 ARM64 使用同一个安装包。
2. 停止旧版 MiAir / MiAir Plus，避免 8310 / 8311 端口冲突。预览版使用独立应用 ID `miair-plus`，不会覆盖旧 `airisland` 数据。
3. 在飞牛应用中心手动安装，打开管理页面，创建自己的管理员账号。
4. 在“账号”中使用米家扫码，刷新设备，选择音箱并保存。
5. 在“设置”中确认 NAS 局域网 IPv4。手机、NAS 和音箱需在同一局域网，路由器不能隔离组播或客户端。
6. 需要 AirPlay 时，在“设置”确认 FFmpeg 已检测到。程序依次查找 PATH、`/usr/bin/ffmpeg`、`/usr/trim/bin/ffmpeg`、`/var/apps/ffmpeg/target/bin/ffmpeg`，也可填写实际安装路径。**安装包不捆绑 FFmpeg，不自动安装系统软件。** 可通过设备上已有的软件包管理方式安装 FFmpeg，或使用管理员准备的版本。
7. 在音乐 App 的 DLNA / AirPlay 音频输出列表选择音箱。

管理端口默认 TCP 8310，DLNA 与音频代理 TCP 8311，SSDP UDP 1900，mDNS UDP 5353。RAOP 每台启用音箱使用动态 TCP 端口及每个会话的 UDP 音频／控制／时钟端口；防火墙需允许可信局域网访问这些端口。

修改名称后页面会显示后台同步状态，发送端的旧名称缓存可能需要关闭并重新打开投送列表。改变 NAS IP 或音频端口会重建服务并中断当前投送。

## 从源码运行

需要 Go 1.27.1+、Node.js 22+；运行 AirPlay 需要 FFmpeg。

```sh
cd frontend
npm ci
npm run typecheck
npm run build
cd ..
cp -R frontend/dist/. internal/web/dist/
go test ./...
go build -trimpath -o miair-plus ./cmd/miair-plus
./miair-plus --data ./data --listen :8310
```

Windows 开发可用 `Copy-Item frontend/dist/* internal/web/dist -Recurse -Force` 替代 `cp`。`--no-discovery` 关闭组播广播，适合本地页面开发。

原生 FPK：安装官方 [fnpack](https://developer.fnnas.com/docs/cli/fnpack)，然后运行 `scripts/build.sh`，或 Windows 下运行 `scripts/build.ps1 -Fnpack <fnpack.exe路径>`。输出在 `dist/`。构建工具同样使用 Go，包内包含对应源代码 `source.tar.gz`。

## 数据与回退

管理员密码使用 bcrypt；会话具有到期时间，修改密码会使旧会话失效。小米凭据只保存在所选数据目录的 `miair-plus.json` 中，Unix 权限为 0600；请妥善保护该目录。不要上传配置、Cookie、令牌或日志中的私密内容。

旧版数据不会自动导入；迁移时重新扫码并选择音箱。回退时先停止 Go 版，再启动旧版即可。卸载前按需备份 `${TRIM_PKGVAR}/data`。

## 已知边界与验收

见 [兼容与测试说明](docs/COMPATIBILITY.md)。设置仅提供本版实现的功能；旧版的歌词匹配、通知推送、验证码交互登录、被打断后强制续播未包含在本次重写中。

## 来源与许可

GPL-3.0。项目参考 [deerwan/miair-next](https://github.com/deerwan/miair-next) 与 [KiriChen-Wind/MiAir](https://github.com/KiriChen-Wind/MiAir)，不是原项目官方版本。保留了前端基础、部分协议描述和公开协议常量，并重写运行后端；不是“全部从零原创”。详细说明见 [UPSTREAM.md](UPSTREAM.md)。

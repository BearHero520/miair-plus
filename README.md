# MiAir Plus · Go

将小爱音箱接入 DLNA / AirPlay 音频投送的原生桥接服务，提供简洁的磨砂风格管理页面和飞牛 fnOS 安装包。Go 主服务配套 Shairport Sync、NQPTP 与 FFmpeg 原生组件，不依赖 Docker 或 Python。

**当前为 `2.0.0-alpha.4` 预览版。已做协议与解码自动测试，尚未完成 fnOS、真实小爱音箱、QQ 音乐／网易云／OPPO／Apple 发送端的实机验收。不要把预览版当作全型号兼容的稳定版。**

## alpha.4 更新

- 左侧导航、右侧工作区，Element Plus 主题蓝（`#409EFF`）与半透明磨砂表面；手机保留紧凑左侧导航。
- 首页只保留三项状态与简短音箱列表。设置页明确显示内置 FFmpeg 状态，自定义路径折叠到高级设置。
- 日志记录投送操作、音箱执行耗时与失败原因、名称广播及连接恢复。支持筛选、暂停刷新、脱敏诊断报告下载。
- 最近 1000 条事件在重启后恢复，磁盘按 2 MiB 文件轮转并保留一个备份。

## 黑屏修复与通用安装包

`alpha.2` 使用 `go:embed all:dist` 打包以下划线开头的前端辅助模块，修复 `_plugin-vue_export-helper` 返回 404 引起的黑屏。升级后请强制刷新页面（Ctrl+Shift+R）；入口 HTML 不再缓存。通用包内同时携带两个架构的 Go 二进制，由启动脚本按 CPU 自动选择。

## 这次改了什么

- 管理 API、小米账号通信、DLNA、传统 RAOP 接收及服务管理使用 Go；AirPlay 2 协议交给 Shairport Sync 5.5.1，时钟使用 NQPTP 1.2.8，音频处理使用 FFmpeg 8.1。
- 前端保留 Vue 3，编译后嵌入 Go 二进制，无需在 NAS 安装 Node.js。
- 名称保存先原子写入本地，再在后台更新广播，不等待小米云。传统 AirPlay 改名不重建音频会话；AirPlay 2 改名和切换目标会在后台重建接收组件。
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
| AirPlay | 传统 Go RAOP 接收；可选 Shairport Sync AirPlay 2 原生接收，默认关闭以便逐步验收 |
| AirPlay 2 入口 | 同一 NAS IP 使用一个接收实例，设置中选择目标音箱；其余音箱保留传统 AirPlay，全部音箱保留 DLNA |
| 暂不支持 | 屏幕镜像、视频接收；不保证经过小米云和 HTTP 桥接后的多房间精确同步 |
| 小米音箱 | 小米云能列出的设备；提供两种 MiNA 播放接口与兼容模式，不承诺全型号 |
| 解码与拖动进度 | 通用包内置精简音频 FFmpeg；也可手动指定兼容的系统 FFmpeg |

AirPlay 2 需要 NAS 系统已有 D-Bus 与 Avahi 服务，以及空闲的 TCP 7000、UDP 319/320。安装脚本只为 NQPTP 赋予低端口绑定能力，Go 服务与 Shairport Sync 以应用用户运行。组件或依赖不可用会显示错误，DLNA 独立运行；后台默认每 30 秒尝试恢复。安装包不会修改系统 Avahi 配置，也不会停止占用端口的其他应用。

## 安装到飞牛

1. 从 Releases 下载 `miair-plus-2.0.0-alpha.4-all.fpk`，x86_64 与 ARM64 使用同一个安装包。
2. 停止旧版 MiAir / MiAir Plus，避免 8310 / 8311 端口冲突。预览版使用独立应用 ID `miair-plus`，不会覆盖旧 `airisland` 数据。
3. 在飞牛应用中心手动安装，打开管理页面，创建自己的管理员账号。
4. 在“账号”中使用米家扫码，刷新设备，选择音箱并保存。
5. 在“设置”中确认 NAS 局域网 IPv4。手机、NAS 和音箱需在同一局域网，路由器不能隔离组播或客户端。
6. 在“设置”确认 FFmpeg 已检测到。通用包优先使用自带版本；想使用 AirPlay 2 时打开对应开关，选择已启用的目标音箱，保存并等待“原生接收组件已就绪”。这代表服务就绪，真实投送还需在手机上验证。
7. 在音乐 App 的 DLNA / AirPlay 音频输出列表选择音箱。

管理端口默认 TCP 8310，DLNA 与音频代理 TCP 8311，SSDP UDP 1900，mDNS UDP 5353。RAOP 每台启用音箱使用动态 TCP 端口及每个会话的 UDP 音频／控制／时钟端口；防火墙需允许可信局域网访问这些端口。

修改名称后页面会显示后台同步状态，发送端的旧名称缓存可能需要关闭并重新打开投送列表。改变 NAS IP 或音频端口会重建服务并中断当前投送。

## 从源码运行

需要 Go 1.27.1+、Node.js 22+；传统 AirPlay 需要 FFmpeg。AirPlay 2 还需要原生运行库，将 `MIAIR_AUDIO_DIR` 指向对应架构的运行库目录（内含 bin、lib）。

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

原生 FPK：安装官方 [fnpack](https://developer.fnnas.com/docs/cli/fnpack)，运行 GitHub Actions 的 `Build native AirPlay 2 runtime`，取出两个架构的 `airplay2-runtime-*.tar.xz`，分别解压到 `build/native/amd64` 和 `build/native/arm64`。Windows 下运行 `scripts/build.ps1 -Fnpack <fnpack.exe路径> -RuntimeDir <build/native绝对路径>`；Linux 下设置 `MIAIR_RUNTIME_DIR` 后运行 `scripts/build.sh`。打包时校验每个运行库文件的 SHA-256 与 ELF 架构。输出在 `dist/`。

原生组件在 CI 的一次性 Debian 12 构建环境中编译，NAS 运行时不需要容器。运行库以 glibc 2.36 为基线，需要在不同 fnOS 版本上实测。对应第三方源代码作为 Release 的 `airplay2-sources-*.tar.xz` 一起提供，FPK 内保留许可证和本项目 `source.tar.gz`。

## 数据与回退

管理员密码使用 bcrypt；会话具有到期时间，修改密码会使旧会话失效。小米凭据只保存在所选数据目录的 `miair-plus.json` 中，Unix 权限为 0600；请妥善保护该目录。不要上传配置、Cookie、令牌或日志中的私密内容。

旧版数据不会自动导入；迁移时重新扫码并选择音箱。回退时先停止 Go 版，再启动旧版即可。卸载前按需备份 `${TRIM_PKGVAR}/data`。

## 已知边界与验收

见 [兼容与测试说明](docs/COMPATIBILITY.md)。设置仅提供本版实现的功能；旧版的歌词匹配、通知推送、验证码交互登录、被打断后强制续播未包含在本次重写中。

## 来源与许可

GPL-3.0。项目参考 [deerwan/miair-next](https://github.com/deerwan/miair-next) 与 [KiriChen-Wind/MiAir](https://github.com/KiriChen-Wind/MiAir)，不是原项目官方版本。保留了前端基础、部分协议描述和公开协议常量，并重写运行后端；不是“全部从零原创”。详细说明见 [UPSTREAM.md](UPSTREAM.md)。

# MiAir Plus

为小爱音箱扩展 DLNA、AirPlay 音乐投送与自定义音乐闹钟，配备简洁的磨砂风格管理界面，支持飞牛 fnOS。

[下载安装包](https://github.com/BearHero520/miair-plus/releases) · [兼容与测试说明](docs/COMPATIBILITY.md)

## 功能

### 音乐投送

- **DLNA 投送**：通过支持 DLNA 音频输出的音乐 App，将音乐投送到小爱音箱。
- **AirPlay 接收**：支持传统 AirPlay，提供可选的 AirPlay 2 预览功能；可在设置中选择 AirPlay 2 的目标音箱。
- **音箱管理**：米家扫码登录、刷新设备、启用音箱、自定义投送名称，查看服务与同步状态。
- **内置音频处理**：安装包自带 FFmpeg，支持音频解码与格式转换。
- **投送稳定性**：流式音频代理、连接恢复和操作状态追踪；播放、暂停与切歌直接在音乐 App 中操作。

### 音乐闹钟

- **上传音乐**：从电脑或手机上传音频，显示上传进度。
- **从 NAS 选择**：使用飞牛文件选择器，选择并授权已有音乐文件。
- **铃声库**：已导入的音乐保存在 `miair-plus/ringtones`，可重复选用。
- **灵活重复规则**：支持指定星期、法定工作日（含调休补班）、休息日（周末与放假日）以及仅法定节假日。
- **响铃设置**：选择目标音箱、时间、音量与最长播放时长，预览接下来三次响铃日期。
- **停止与延后**：支持试听、停止和延后 5 分钟；页面显示执行结果与失败原因。

支持 MP3、WAV、FLAC、OGG、AAC，单文件最大 100 MB。保存时提前准备铃声，每次播放一次，播完或达到设定时长后结束，最长 10 分钟。

法定日历按中国大陆安排执行，内置 2026 年数据，支持自动与手动更新。断网时使用已有缓存；尚未收录的年份暂停法定日历规则并提示，不影响指定星期的闹钟。

闹钟需要 NAS 与音箱在线。本功能独立于小爱自带闹钟；语音“停止播放”和机身暂停键的效果取决于音箱型号。

### 管理界面与日志

- 左侧导航、右侧页面，主题蓝搭配半透明磨砂界面，支持深浅色外观。
- 精简总览，集中查看音箱与服务状态。
- 日志展示投送操作、执行耗时、失败原因、名称广播与连接恢复。
- 支持日志筛选、暂停刷新和脱敏诊断报告下载；启动失败保留排查日志。

## 安装与使用

1. 从 [Releases](https://github.com/BearHero520/miair-plus/releases) 下载 `all.fpk` 通用安装包，x86_64 与 ARM64 使用同一文件。
2. 在飞牛应用中心选择“手动安装”，安装后打开管理页面并创建管理员账号。
3. 进入“账号”完成米家扫码登录，在“音箱”中刷新并启用需要的设备。
4. 在“设置”中确认 NAS 局域网 IPv4 地址，让手机、NAS 和音箱处于同一局域网。
5. 在音乐 App 的 DLNA / AirPlay 输出列表中选择音箱。使用 AirPlay 2 时，先在设置中开启并选择目标音箱。
6. 设置闹钟时，进入“闹钟 → 新建闹钟”，选择音乐来源、音箱、时间与重复规则，然后保存。

管理页面默认端口为 `8310`，DLNA 与音频代理端口为 `8311`。局域网需要允许设备发现与组播通信，避免客户端隔离或端口冲突。投送名称更改后，音乐 App 可能需要重新打开设备列表以刷新缓存。

## 兼容范围

| 项目 | 说明 |
| --- | --- |
| NAS 架构 | Linux x86_64 / ARM64 通用 FPK；不支持 ARM32 |
| 小米音箱 | 支持小米云可发现的设备，具体型号的播放能力需实测 |
| QQ 音乐、网易云音乐 | 使用 App 提供的 DLNA 音频投送入口，兼容性随版本与设备而异 |
| OPPO 系统投送 | 需要发送端提供 DLNA 音频输出 |
| AirPlay 2 | 预览功能，单个接收实例选择一个目标音箱；需要系统 D-Bus、Avahi 及可用的 TCP 7000、UDP 319/320 端口 |
| 音频投送范围 | 不支持屏幕镜像、视频接收，不保证多房间精确同步 |

当前修复包为 2.0.5，用户已反馈既有功能测试稳定。已在 x86_64 飞牛 NAS 验证启动、NAS 文件授权、音乐导入、保存与转码；实际投送、闹钟响铃和语音停止仍需按音箱型号与发送端验证。详细状态见 [兼容与测试说明](docs/COMPATIBILITY.md)。

## 开发与构建

需要 Go 1.27.1+、Node.js 22+。音频处理需要 FFmpeg；AirPlay 2 使用 Shairport Sync 与 NQPTP，将 `MIAIR_AUDIO_DIR` 指向对应架构的原生运行库目录（含 `bin`、`lib`）。

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

Windows 可用 `Copy-Item frontend/dist/* internal/web/dist -Recurse -Force` 替代 `cp`。`--no-discovery` 可关闭组播广播，用于本地页面开发。

构建 FPK：安装官方 [fnpack](https://developer.fnnas.com/docs/cli/fnpack)，运行 GitHub Actions 的 `Build native AirPlay 2 runtime`，将两个架构的运行库分别解压到 `build/native/amd64` 与 `build/native/arm64`。Windows 使用 `scripts/build.ps1 -Fnpack <fnpack.exe路径> -RuntimeDir <build/native绝对路径>`；Linux 设置 `MIAIR_RUNTIME_DIR` 后运行 `scripts/build.sh`。安装包输出到 `dist/`，原生运行库以 glibc 2.36 为基线。

## 数据与许可

应用配置保存在数据目录中；飞牛卸载前可按需备份 `${TRIM_PKGVAR}/data`。配置包含小米登录凭据，请勿公开上传。

项目采用 GPL-3.0 许可证。致谢 [miair-next](https://github.com/deerwan/miair-next)、[MiAir](https://github.com/KiriChen-Wind/MiAir) 及相关开源组件，来源与许可详见 [UPSTREAM.md](UPSTREAM.md)。调休日历使用 [holiday-cn](https://github.com/NateScarlet/holiday-cn) 数据。原生组件对应源码随 Release 提供，安装包内保留相关许可证。

## 手机飞牛 App 与远程访问

2.0.2 正式版起，应用中心入口通过飞牛统一网关访问 `/app/miair-plus/`，复用系统域名与 HTTPS，局域网仍可直接访问 8310。移动端使用底部导航，账号、日志、关于位于“更多”。NAS 文件授权能力需要 fnOS 1.2.0401+、飞牛 App 1.34.0+。网关接入及 iOS 真机复测说明见 [手机端排查记录](docs/FNOS-MOBILE.md)。

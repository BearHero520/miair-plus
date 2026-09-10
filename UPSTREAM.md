# 来源与许可

MiAir Plus 使用 GPL-3.0，完整文本见 LICENSE。

- MiAir Next: https://github.com/deerwan/miair-next ，本次工作基于之前本地版本，追溯基线 `163afd07f3175c6686b9c0cb0322f4e300c7699e`。
- MiAir: https://github.com/KiriChen-Wind/MiAir ，作为功能与产品参考。
- 继承的 Vue 前端、图标和 UPnP SCPD 描述来自上述 MiAir Next 开发线；已调整页面、品牌、交互与 Go API。
- 小米 MiNA / 账号协议结构参考 MiAir Next 与其使用的 miservice-fork；新实现使用 Go net/http。
- `internal/airplay/airport.go` 中的 AirPort RSA key 是广泛公开的协议互操作常量，来自旧版代码。它不是部署者私钥、用户密钥或账号凭据。
- Go 依赖的许可证由各自 module 保留；版本记录在 go.mod / go.sum。前端依赖记录在 package-lock.json。

AirPlay 2 原生组件来自 [Shairport Sync 5.5.1](https://github.com/mikebrady/shairport-sync/tree/5.5.1)、[NQPTP 1.2.8](https://github.com/mikebrady/nqptp/tree/1.2.8) 与 [FFmpeg n8.1](https://github.com/FFmpeg/FFmpeg/tree/n8.1)。各组件保持各自许可证；FFmpeg 使用 GPLv3 构建选项。构建脚本、精确 Git 提交、Debian 源包版本、许可证随运行库一并保存；完整对应源码在同版 Release 的两个 `airplay2-sources-*.tar.xz` 附件中提供。它们是独立原生组件，不宣称是 Go 重写。

发布的 FPK 内提供 source.tar.gz，包括本项目对应源码、前端源码、依赖锁文件与构建脚本；第三方依赖可从对应锁定版本的公开源下载。

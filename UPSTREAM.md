# 来源与许可

MiAir Plus 使用 GPL-3.0，完整文本见 LICENSE。

- MiAir Next: https://github.com/deerwan/miair-next ，本次工作基于之前本地版本，追溯基线 `163afd07f3175c6686b9c0cb0322f4e300c7699e`。
- MiAir: https://github.com/KiriChen-Wind/MiAir ，作为功能与产品参考。
- 继承的 Vue 前端、图标和 UPnP SCPD 描述来自上述 MiAir Next 开发线；已调整页面、品牌、交互与 Go API。
- 小米 MiNA / 账号协议结构参考 MiAir Next 与其使用的 miservice-fork；新实现使用 Go net/http。
- `internal/airplay/airport.go` 中的 AirPort RSA key 是广泛公开的协议互操作常量，来自旧版代码。它不是部署者私钥、用户密钥或账号凭据。
- Go 依赖的许可证由各自 module 保留；版本记录在 go.mod / go.sum。前端依赖记录在 package-lock.json。

本版不分发 FFmpeg。使用者自行安装的 FFmpeg 受其自身构建选项与许可证约束。

发布的 FPK 内提供 source.tar.gz，包括本项目对应源码、前端源码、依赖锁文件与构建脚本；第三方依赖可从对应锁定版本的公开源下载。

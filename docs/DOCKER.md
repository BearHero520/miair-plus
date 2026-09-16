# Docker 部署

镜像：`bearhero/miair-plus:docker-2.0.9-1`（Docker Hub），支持 Linux amd64 / arm64。
`latest` 指向最新的 Docker 正式发布；固定版本标签方便回退。Docker 版本与 FPK 共用应用源码，当前应用版本为 2.0.9。

## 安装

将仓库根目录的 `compose.yaml` 保存到 NAS 的独立目录，在该目录执行：

```sh
docker compose pull
docker compose up -d
```

打开 `http://NAS局域网IP:8310`，创建管理员账号，扫码登录小米账号，刷新并启用音箱。在设置中选择 NAS 实际局域网 IPv4。

- 必须使用 Linux Docker 的 host 网络，手机、音箱和 NAS 应在可组播通信的同一局域网。普通 bridge 端口映射不能替代组播发现。
- 默认管理端口 8310、DLNA / 音频代理 8311；AirPlay 2 默认 TCP 7000（可在设置中修改），时钟使用 UDP 319/320。需要允许 mDNS UDP 5353、SSDP UDP 1900 和接收器动态端口。不要只开放管理页面端口。
- 先停止同一台 NAS 上的 MiAir Plus FPK 或旧容器，避免端口、设备广播和重复闹钟冲突。其他 AirPlay 接收服务也可能占用时钟端口。
- 镜像包含 FFmpeg、修复自定义端口的 Shairport Sync、NQPTP，以及容器自己的 D-Bus / Avahi。不需要挂载宿主机 D-Bus，不需要 `privileged` 或 host PID / IPC。
- 容器以 root 运行，使用 Docker 默认能力绑定 NQPTP 的低位 UDP 端口；不支持直接加 `user:` 或删除全部 capabilities。数据目录权限默认仅所属用户可访问。
- Windows / macOS Docker Desktop 不作为局域网投送验收环境。

## 数据、音乐与更新

`./data:/data` 保存账号、配置、铃声及闹钟。数据中包含登录凭据，备份时请妥善保管。默认时区为 `Asia/Shanghai`，可通过 Compose 的 `TZ` 调整。

Docker 版从网页上传音乐即可；飞牛文件授权选择器和应用中心统一网关依赖 FPK 集成，Docker 独立入口不提供这些能力。

迁移 FPK 数据时，先停止 FPK，再把原 `${TRIM_PKGVAR}/data` 的全部内容复制到 Compose 旁的 `data/` 目录。保留一份原始备份；若配置中保留 FPK 的 FFmpeg 绝对路径，请在设置中清空自定义路径，使用容器内置组件。铃声若引用旧绝对路径，需要重新上传和选择。检查每个闹钟后再启用。

更新先备份 `data/`，修改 Compose 镜像标签，再执行：

```sh
docker compose pull
docker compose up -d
docker compose logs --tail=100 -f
```

网页更新提示跟随 GitHub 应用版本，不会自动替换容器。回退时选择旧镜像标签，必要时恢复对应的数据备份。

## 本地构建

```sh
docker build -t miair-plus:local .
docker run -d --name miair-plus --network host --restart unless-stopped \
  -e TZ=Asia/Shanghai -v "$PWD/data:/data" miair-plus:local
```

前端和 Go 服务从当前源码构建。原生音频组件从 v2.0.9 FPK 提取，校验固定包 SHA-256 和组件可执行权限，不依赖本地 `build/` 文件。
原生构建脚本为 `scripts/build-airplay2.sh`，许可证随镜像保存在 `/opt/miair-audio/licenses`。对应源码见 [v2.0.9 Release](https://github.com/BearHero520/miair-plus/releases/tag/v2.0.9) 的 `airplay2-sources-amd64.tar.xz` / `airplay2-sources-arm64.tar.xz`。

## 发布与验证

GitHub Actions 的 `Build and publish Docker` 使用原生 amd64 / arm64 runner 分别构建，验证 FFmpeg、AirPlay 2 真实接收器、HTTP 页面、账号数据重启持久化、健康检查与优雅停止。两种架构均通过后才发布合并标签。

仓库 Actions Secret `DOCKERHUB_TOKEN` 需要 Docker Hub 用户 `bearhero` 的 Read & Write 令牌。推送 `docker-*` Git 标签发布同名镜像标签及 `latest`；手动运行发布 `edge`。构建镜像归档保留 7 天，即使发布凭据缺失也能取得测试通过的构建产物。

```sh
git tag docker-2.0.9-1
git push origin docker-2.0.9-1
```

CI 的原生组件测试不代替真实小爱音箱、手机与 NAS 的投送验收。

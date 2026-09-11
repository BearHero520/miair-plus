# 飞牛手机端白屏排查与 2.0.2 正式版 修复

## 现场与结论范围

用户反馈：电脑飞牛网页桌面正常，iOS 飞牛 App 最新版通过远程域名打开白屏。未取得手机 WebView 控制台、NAS 网关日志或远程入口响应，不能把本地测试等同于手机实测。

代码中确认的问题：旧入口强制访问 `http://远程域名:8310/`，不能复用已连通的飞牛系统域名和 HTTPS 入口；前端脚本、API 与 WebSocket 使用根路径；后端 `X-Frame-Options: DENY` 拒绝内嵌展示。此外，SDK 只在闹钟页才初始化，文件授权总是使用独立浏览器弹窗，没有区分手机宿主。

## 官方文档依据

核对日期：2026-09-10。

- [应用入口](https://developer.fnnas.com/docs/core-concepts/app-entry/)：`iframe` 用于内嵌打开，`url` 用于外部 Web 视图或浏览器。
- [统一网关](https://developer.fnnas.com/docs/core-concepts/gateway-registration/)：`gatewayPrefix` 注册公开路径，`gatewaySocket` 指向应用 target 下的 Unix Socket；转发时保留路径前缀，HTTP 和 WebSocket 均应在该前缀下。
- [SDK 调用方式](https://developer.fnnas.com/api/calling/)：使用 SDK 声明 `micro_app=true`；宿主内直接调用 `pickUserFile`，独立浏览器使用授权路由。
- [更新日志](https://developer.fnnas.com/docs/update-log/)：新开放 API 要求 fnOS 1.2.0401+、飞牛 App 1.34.0+。这属于 NAS 文件授权能力版本要求，不作为所有页面白屏的已证实原因。

## 实现

- 默认入口改为同域 `/app/miair-plus/`，保留 8310 局域网直连。页面 HTML 按入口注入 base；路由、懒加载脚本、API、WebSocket、图片和文件授权回调保持同一前缀。
- 额外监听 Unix Socket。`target/app.sock` 链接到 `target/gateway/http.sock`；启动脚本只给应用账号 gateway 目录写权限，保持程序目录和二进制的原有所有权。Socket 允许系统网关连接，业务接口继续检查 MiAir 管理员 token，不使用客户端传入的 NAS 用户头替代应用鉴权。
- 嵌入限制改为 `SAMEORIGIN`，保持跨站来源检查。
- 页面入口创建共享 SDK 实例，不等待宿主就绪才渲染。手机宿主使用原生文件选择器；不支持的宿主调用返回错误并可改用上传。
- 处理存储被 WebView 禁用的情况、旧版媒体查询监听方式，明确 JavaScript 编译目标。启动失败提供重试提示。
- 手机宽度不大于 700px 时使用五项底部导航（总览、音箱、闹钟、设置、更多）；更多包含账号、日志、关于。适配底部安全区域，桌面维持侧栏。

## 验证与真机复测

自动测试覆盖网关转发、入口与深层页面 base、同源策略、管理员鉴权、WebSocket 握手、缺失脚本 404、嵌入资源完整性。浏览器回归检查 320/390/700/701/1440px、更多抽屉、暗色外观、直接访问与深层链接、存储禁用和入口脚本加载失败。

Windows 本地测试不能验证 fnOS 安装后的网关注册、Linux Socket 权限和 iOS WKWebView 实际行为。Linux CI 生命周期脚本已增加通过 `app.sock` 请求网关页面的检查。

安装 2.0.2 正式版 后退出原页面，再从飞牛应用中心重新打开；确认 URL 使用 `/app/miair-plus/`。手机通过原远程域名分别检查登录、总览、切页、闹钟以及 NAS 文件选择。如果仍白屏，先用同一个手机浏览器打开 `https://你的飞牛域名/app/miair-plus/`（使用实际协议和端口），区分网关未连通与 App 容器问题。保留 `startup.log`、`app.log` 中相关错误，反馈 fnOS 的准确版本。

## 2.0.2 现场定位补充

已登录用户 NAS 复现桌面入口失败：入口 HTML 200，携带 Origin 的模块脚本与样式请求 403。系统 nginx 的 `/app/` 配置使用 `proxy_set_header Host $host`，去掉了 Host 端口，而浏览器 Origin 保留 `:5000`；MiAir 原先直接比较二者，误拒绝了同源请求。

修复仅在实际 Unix Socket 监听入口标记可信网关请求；上游 Host 没有端口时，允许同主机名 Origin 携带外部端口。TCP 直连继续比较完整 Host，不接受客户端 Header 或路径前缀伪装成网关。HTTP 与 WebSocket 复用同一校验函数，管理员 token 要求保持不变。增加端口丢失、伪造头、跨域、缺少 token 及 WebSocket 握手回归测试。启动错误现在显示阶段与去除查询参数的资源地址，图片失败不会误报整个页面启动失败。

现场测试中，飞牛 CLI 的 install-fpk 对已安装应用只报告已安装，即使提供更高版本也未执行升级；升级应使用应用中心的手动安装流程。


## 2.0.3 API 适配与加载修复

用户确认 2.0.2 桌面已正常，手机仍报告入口 JS/CSS 下载失败。尚未取得手机失败请求状态，因此不能确认手机问题全部解决。

- 将公开嵌入静态资源与业务 API 来源校验分开；assets 允许跨源读取，不包含用户数据。API 和 WebSocket 继续校验 Origin 和应用 token。
- 资源失败时用同源 GET 补充 HTTP 状态和 Content-Type，不采集响应正文、Cookie 或 URL 查询参数。
- 文件选择使用 pickUserFile 的明确标题、确认文案和文件类型过滤；提供手动读取回调结果、返回页面读取结果和飞牛授权设置入口。取消/关闭后不再把旧请求结果写入新表单。
- 单文件授权只能使用选择器/回调返回结果，不调用只返回目录的 getUserAccessibleFolders。NAS 导入通过 Unix Socket 上的 X-Trim-Userid 识别用户，再用 trim.file.checkUserACL 校验原路径及解析后的路径；声明 trim.file.userAcl。直接端口访问没有可信 NAS 用户身份，保留上传音乐入口并引导从飞牛入口选 NAS 文件。
- 页面渲染后异步读取宿主主题与版本；仅 Web 宿主订阅主题/语言事件。关于页可刷新系统/App 版本并打开应用设置。

版本要求与接口依据：[文件授权](https://developer.fnnas.com/api/authorization/user-access/)、[权限检查](https://developer.fnnas.com/api/authorization/file-acl/)、[平台配置](https://developer.fnnas.com/api/platform-config/)。2.0.3 需要通过应用中心升级以重新登记新增 Scope。


## 2.0.4 网关令牌冲突现场修复

现场浏览器确认：/app/miair-plus/api/v1/settings、me、alarms 返回 HTTP 200、text/plain，正文为 invalid token。NAS 内置 FFmpeg 执行 -version 正常；组件不可用是设置接口失败引起的误报。音箱、闹钟和日志页面把文本当作正常数据后触发 filter/length/map 错误。

通过网关时，HTTP 使用 X-Miair-Authorization 传递应用令牌，避免占用飞牛系统 Authorization；WebSocket 改用 miair_token 查询参数。前端统一拒绝非 JSON API 响应，设置尚未读成功时不显示组件缺失警告。

现场已备份原程序至 /var/apps/miair-plus/var/miair-plus-before-204，通过应用管理器停止、替换程序、启动。当前程序为 2.0.4 热修复，应用中心安装元数据仍为 2.0.3，可用完整 2.0.4 包同步升级。未改动账号、音箱及闹钟数据。

实际飞牛网页桌面复测：总览显示投送服务运行中；设置页 FFmpeg 显示内置组件已就绪；原有音箱和闹钟正常显示；me/settings/alarms/speakers 接口返回 application/json、HTTP 200；WebSocket 握手 101。iOS 远程入口仍需用户重新打开确认。


## 2.0.5 移动端来源校验

用户反馈移动端业务接口仍报跨站请求被拒绝。新增规则仅用于实际 Unix Socket 网关且存在有效 X-Trim-Userid 的请求：有效 MiAir 专用令牌可通过 HTTP/WebSocket 的来源校验；登录和初始化状态请求要求 X-Miair-Client，登录 POST 另要求 application/json。API 不返回跨源许可头。TCP 入口仍使用原来源限制，不信任伪造网关身份。

模拟 null Origin、远程域名/Host 不一致的登录、查询和 WebSocket 测试通过；表单请求、预检、无效令牌和直连伪造头被拒绝。真实手机请求的 Origin 尚未抓取，不将模拟测试作为 iOS 实测。

已备份前版程序到 /var/apps/miair-plus/var/miair-plus-before-205，应用管理器停止/启动后运行程序为 2.0.5。实际 NAS 网关浏览器总览显示账号已连接、投送运行中、音频组件已就绪。应用中心元数据需安装完整 2.0.5 包同步。用户手机需关闭旧页面重新打开确认。

## 2.0.5 用户验收反馈

用户随后确认已实测，各项功能均正常。此前章节保留排查时的现场状态与验证边界；本次反馈确认用户当前设备和环境下的修复结果，不扩展为所有机型的兼容保证。

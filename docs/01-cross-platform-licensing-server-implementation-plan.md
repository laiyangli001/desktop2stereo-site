# Desktop2Stereo 跨平台授权服务器端实施计划

## 1. 范围与状态

本文只负责 `desktop2stereo-site` 服务器端。客户端启动器、设备指纹、安全存储、Runtime
二次门禁和三平台发布由 `desktop2stereo-vulkan/docs/13-cross-platform-licensing-launcher-implementation-plan.md`
负责。

服务端基线为 `QuantumNous/new-api@3a9f41ee85cc369f5b8d7fe6e62ff4e7bf3a9ec8`。
截至 2026-09-04，GitHub 仓库、new-api 基线、授权与商业域第一版代码、SQLite 单元测试和
MySQL/PostgreSQL CI 测试入口已经建立，三种数据库的新建库与重复迁移验证均已通过。并发
压力、支付渠道沙箱、React 页面和生产演练完成前，状态仍为“开发中”，不能标记生产可用。

## 2. 目标与不变规则

服务器端必须提供：

- 邮箱注册验证、登录退出、密码重置、安全会话和管理员权限。
- 每个已验证账号唯一一份 30 天试用，以及一个账号多份正式授权。
- 一份授权同一时间只绑定一台设备；在线、7/14/30 天离线及永久绑定模式。
- 设备码登录、ES256 离线凭证、2 小时在线租约、15 分钟心跳和稳定错误码。
- 免费撤销、每月三次阶梯付费撤销、终身一次人工解绑和离线延长包。
- CN/INTL 首次外部付款区域锁、服务器报价、幂等订单和支付回调。
- CNY/USD 分币种余额、邀请首购奖励、拒付冲正和 CN 支付宝提现审核。
- 公开投诉与服务咨询表单、企业微信客服浮动入口及后台授权工单处理。

客户端提交的价格、区域、付款结果和授权状态永远不可信。不支持用户主动退款、授权转移，
也不使用毒化模型或错误画面作为授权失败处理。

## 3. new-api 复用与新增边界

直接复用 new-api：

- 注册、邮箱验证码、登录、退出、密码找回、OAuth、Passkey 和 2FA。
- Access Token、HttpOnly Refresh Cookie、轮换、重放处置、Turnstile 和限流。
- 用户管理、管理员鉴权、管理员写操作审计、Redis 和三种关系数据库。
- Stripe、Creem、易支付、Waffo/Waffo Pancake 已有收银台及渠道能力。
- `users.aff_code`、`users.inviter_id` 邀请归属字段。

新增 D2S 域：授权状态机、设备绑定、设备码、离线凭证、在线租约、撤销、区域锁、授权订单、
分币种账本、邀请奖励、拒付、提现、人工解绑和管理 API。new-api 的 AI quota 钱包不具备
ISO 货币、提现、负余额和订单快照语义，不作为授权业务账本。

详细差距见 [`new-api-gap-analysis.md`](new-api-gap-analysis.md)。

## 4. 数据模型与并发约束

GORM 维护以下表：

- `d2s_user_profiles`：邮箱资格、区域和锁定时间。
- `d2s_licenses`、`d2s_device_bindings`、`d2s_license_events`：授权、当前设备和历史。
- `d2s_device_codes`：十分钟有效、单次消费的设备码状态机。
- `d2s_offline_entitlements`、`d2s_online_leases`、`d2s_signing_keys`：签发和租约。
- `d2s_free_revoke_cooldowns`、`d2s_paid_revoke_quotas`、`d2s_manual_unbind_requests`。
- `d2s_orders`、`d2s_payment_events`：报价快照和支付幂等。
- `d2s_balance_accounts`、`d2s_balance_transactions`、`d2s_invite_rewards`。
- `d2s_withdrawal_requests`：CNY 提现预留和管理员审核。
- `d2s_support_tickets`：付款、订单/授权、下载、账户与软件使用问题的站内工单，包含联系邮箱、用户描述和处理状态。

数据库必须强制：每用户唯一试用、每设备摘要唯一当前绑定、每授权唯一活动租约、每用户内
幂等键唯一、每授权最多一个待处理付费撤销订单、支付渠道事件联合唯一、每受邀账号最多一次
首购奖励。余额预留、支付完成、区域锁、授权签发和奖励必须在同一数据库事务内完成，并对
竞争行加锁。

所有金额使用整数最小货币单位，时间使用 UTC Unix 秒。只保存设备摘要、令牌哈希和 JWS
摘要，不保存原始设备标识、刷新令牌明文、支付 Secret 或签名私钥。

### 4.1 账户审计与后台治理

后台治理模块复用用户、订单和支付业务实体，不复用前台用户凭据作为后台登录凭据；管理员账号、会话及角色独立存储。新增用户行为日志、注册快照、后台操作审计和账户紧急控制。身份核验模型仅为兼容遗留数据库保留，不纳入新库自动迁移；当前无证件录入页面或已注册 API 路由，不代表收集身份信息。购买留痕扩展现有订单快照，记录邮箱、来源 IP、渠道及脱敏支付标识，不重复建立可分叉的交易账本。

用户行为日志为追加写入，保存完整网络与请求字段并为关键字段计算 SHA-256；来源端口仅在 TCP 对端与解析出的客户端 IP 一致、且 TCP 对端不属于 `TRUSTED_PROXIES` 时记录；经反向代理转发或无法确认时留空，不能把代理连接端口当作客户端端口。查询分页、CSV 导出、单条哈希校验和注册导出仅开放给具备对应后台角色的管理员。后台写操作、敏感财务/授权读取、日志查看/导出、紧急限制和生命周期清理均留审计记录；涉及业务状态变更的成功审计与业务更新在同一事务提交。前台用户或管理员 API 不提供日志更新、删除或清空操作。

邮箱验证码验证为注册和登录/购买的硬性前置条件，验证码有效期不超过 15 分钟，成功核验后一次性失效，并按邮箱和来源 IP 限速；成功注册时，用户激活状态、注册快照和成功 `REGISTER` 行为日志在同一数据库事务中提交。登录成功、失败及临时锁定事件均写入用户行为日志。账户紧急限制在服务端检查，支持禁登录、禁购买、会话撤销及分批处理，批量执行要求预览、二次确认并审计筛选条件与影响数量。

行为日志、注册快照和后台操作审计默认不自动清理；受控清理在服务端将最低期限固定为 366 天并拒绝更低值，用户行为日志仍保留六个月法定基线保护逻辑。成功清理在同一事务写入逐表截止时间、范围和数量审计并删除到期记录；清理事务失败后先回滚，再用新事务追加失败阶段、实际删除数和候选记录统计，数据库故障时由后台操作审计路径再次尝试留痕。应用运行时日志数据库凭据仅具备 INSERT/SELECT。SQLite 的文件级权限不能提供表级授权，部署说明必须清楚区分数据库约束与应用层追加约束。

字段、路由、角色、索引、哈希编码、错误码及导出语义见 [`d2s-admin-management-spec.md`](d2s-admin-management-spec.md)。

当前实施状态：`in_progress`。已落地独立后台账号/会话、一次性首位超级管理员环境变量引导、行为与注册/管理员审计表、日志追加钩子和 SHA-256 校验、查询与 CSV 导出、管理员角色中间件、邮箱验证登录门禁、单账户/批量紧急限制、订单购买快照及 366 天受控清理下限。邮箱验证只证明邮箱控制权。证件核验页面和 API 路由未启用；兼容模型已从新库自动迁移中移除，既有表不会自动删除。实际路由以规格书的 API 表为准。

2026-11-02 新增 `/support` 登录后服务问题表单和 `d2s_support_tickets` 持久化工单，按类型受理付款、订单/授权、下载、软件使用和账户问题；全站右侧浮动联系面板提供站内留言和企业微信入口，企业微信默认链接由运营方提供并允许环境变量覆盖。服务端强制登录后提交并自动关联当前账户，账户可通过“我的工单”仅查看服务端按 user ID 筛选的记录；访客不能创建工单。后台 `account_operator` 及以上角色可分页查看、更新状态和处理记录，成功变更与审计在同一事务提交。生产审计表迁移、角色配置和客服公开配置接口已按 2026-11-03 发布记录核验；生产登录工单、账户隔离及后台处理闭环仍须验收。工单保存期限由运营方按个人信息处理目的单独确定，不套用日志留存期限。

2026-11-02 工单数据库矩阵覆盖 MySQL/PostgreSQL 上的工单创建、用户隔离查询、状态变更和管理员审计提交；控制器回归伪造请求体 user_id，确认记录只关联认证上下文账户。随后在真实隔离 MySQL 8.4.13 与 PostgreSQL 17.11 上执行最新工作树完整 `go test -p 1 ./... -count=1`，全仓通过；verbose 定向复跑 `TestD2SSchemaConfiguredDatabases` 和 `TestD2SConcurrentCriticalPaths`，两种数据库子测试均通过。矩阵在空数据库上连续执行两次 `AutoMigrate`，并检查工单创建/隔离/审计与关键并发路径。另以最近已发布版本 `26f524d11424a2866a8608a4883a63dd1f7fe16d` 分别建立 MySQL/PostgreSQL 代表性 schema，旧版迁移各连续运行两次并写入用户、授权、订单样本；随后当前审计独立迁移和当前 `model.InitDB()` 在两种数据库上各连续运行两次。升级后用户/授权/订单样本仍存在，订单 `purchase_time` 从 `created_at` 正确回填，新增审计/工单表及订单快照列存在，授权身份与订单幂等唯一索引仍在。该夹具来自发布代码而非生产数据备份；生产审计表和权限已于 2026-11-03 单独迁移/核验，本地矩阵仍不替代生产支付、后台闭环和恢复演练。

未完成或未验证：生产已通过固定更新流程运行 `d11d0cc1da5070bc65b57ff72181ed2865043833`；四张审计表已迁移并归属 `d2s_migrator`，`d2s_app` 对其仅有 SELECT/INSERT，清理角色权限已按职责配置。生产客服配置 API 已启用；Playwright 只读验收确认访客访问 `/support` 会转到登录页，390px 视口的投诉浮动面板显示注册/登录提示并提供企业微信链接，无横向溢出或脚本错误。该浏览器检查没有登录或提交工单，因此生产工单提交、账户隔离和后台处理闭环仍未证明。尚未完成：隐私政策实际内容、真实身份核验适用范围书面意见、第二名超级管理员及双人导出审批生产闭环、支付渠道沙箱和日终对账、COS 跨故障域备份、双实例/三平台生产等价演练及真实屏幕阅读器验收。生产 readiness 因隐私政策接口内容为空仍未通过，不得标记生产就绪或提交自评估。已于 2026-11-02 在本地隔离 MySQL 8.4.13/PostgreSQL 17.11 实例运行完整 Go 测试、空库矩阵及从最近已发布代码提交构造的代表性 schema 升级测试，均通过；升级夹具保留了用户、授权和订单样本并检查唯一索引与订单时间回填，但不是生产数据备份。已在登录态首页 GitHub、夸克和百度下载入口接入服务端白名单点击记录；服务端检查登录状态、邮箱验证和登录限制，并在跳转前写入 `DOWNLOAD/SUCCESS` 日志。首页访客点击也经白名单服务端接口记录为无账户关联的 DOWNLOAD 事件，登录用户仍须通过邮箱验证和账户限制检查；日志只证明链接点击已记录，不证明外部文件传输完成。2026-11-03 先前工作树检查点的全套 Go 测试、Go 静态检查和构建、前端 Vitest 76 文件/476 项、SQLite 迁移回归、前端类型检查/生产构建及 readiness 自检均曾通过；这不代表当前最近提交已完成全量复验，也不等于支付渠道沙箱或生产端到端验收。

- 2026-10-30 修正两类 CSV 导出的审计时序：发送前追加 `STARTED`，成功 flush 后追加 `SUCCESS`，流错误追加 `FAILURE`；若开始审计失败则不发送 CSV。新增成功、流失败和审计表不可用三项回归；`go test -p 1 ./...` 通过。
- 2026-10-30 登录和通用用户行为审计中间件改为缓冲响应，在行为记录追加成功后才返回原响应；追加失败时返回 `503` 且不释放成功响应或登录 Cookie。登录审计失败会撤销新建会话，待完成 2FA 的请求记为 `BLOCKED`；注册成功快照、账号和 `REGISTER` 记录保持同事务提交。新增审计成功/失败及大于 64 KiB 请求体完整透传回归，`go test ./middleware -run 'TestD2S(LoginActionLog|UserActionLog)' -count=1` 与 `go test ./controller ./middleware` 均通过。初次完整 `go test -p 1 ./...` 除 `relay/channel/ollama` 外其余包通过；该包运行临时 `ollama.test.exe` 连续返回 Windows `Access is denied`，单独重跑仍复现。2026-10-30 将 GOCACHE 与 GOTMPDIR 固定到工作区后，重新运行完整 `go test -p 1 ./...` 通过（退出码 0）。订单创建、余额预留和即时履约现在在同一个数据库事务中写入 PURCHASE/SUCCESS 行为审计，失败即回滚；SQLite 审计插入故障触发器测试验证回滚、成功提交和幂等重试，model/controller/router/middleware 测试通过。Stripe Checkout 和 Waffo Pancake Checkout 均发送订单作用域固定 Idempotency-Key；Waffo Pancake SDK 升级至 v0.14.0，并新增回归验证该键只发送到 create-session 请求。Creem request_id 与 Waffo paymentRequestId 均使用本地订单 ID，供应商文档将其定义为去重标识；易支付目前仅传递稳定 ServiceTradeNo，尚未核实适用于当前商户接口的重复下单契约，仍需沙箱确认。第三方 Checkout 外部会话不能由本地数据库事务回滚；真实渠道沙箱异常矩阵和日终对账仍待完成。供应商契约：[Creem](https://docs.creem.io/skills/creem-api/REFERENCE)、[Waffo](https://waffo.com/en/checkout)、[Waffo Pancake SDK](https://pkg.go.dev/github.com/waffo-com/waffo-pancake-sdk-go)。

## 5. 授权状态机

- 授权状态：`active`、`suspended`、`revoked`。
- 授权模式：`unbound`、`online`、`offline`、`permanent`。
- 已验证邮箱账号首次访问时原子创建未启动的试用授权；客户端首次成功绑定设备时才开始计算 30 天试用期，重试和并发请求不能重复创建或重复起算。
- 离线档位为 7/14/30 天，对应免费撤销冷却；模式切换不能清除冷却。
- +30 天延长包叠加当前离线截止时间；当前首发价为 CN ¥19.90（1990 分）、INTL $2.99（299 美分），由环境变量配置，价格调整需同步更新部署配置。
- 在线运行租约 TTL 为 2 小时（不低于 1 小时，推荐 2～4 小时），服务端当前返回 15 分钟默认心跳；
  部署调优时心跳推荐保持在 15～30 分钟，客户端应加入 ±10% 随机抖动，同授权第二个运行实例被拒绝。
  该 TTL 只表示断网后的在线运行宽限，
  不等同于 7/14/30 天离线授权有效期。
- D2S 桌面客户端无浏览器 Cookie 时，刷新接口接受安全存储传入的 JSON `refresh_token`；账号登录和刷新响应提供 `server_time`。
- 用户主动退出释放该账号的在线租约；过期租约允许被下一次事务覆盖并定期清理。
- 永久绑定需明确确认，之后禁止普通降级和撤销；人工解绑每账号终身最多批准一次。
- 付费撤销每授权每自然月最多三次，CN 为 1990/3490/6990 分，INTL 为
  299/499/999 美分。

## 6. 商业规则

- 首份正式授权：CN 9900 分，INTL 2990 美分。
- 支付宝、微信、易支付、支付 FM、Waffo 属 CN；Stripe、Creem、Waffo Pancake 属 INTL。
  PayPal/Paddle 在完成官方适配器和沙箱验收前不属于首发渠道，也不得出现在首发页面。
- 第一次成功的外部支付原子锁定区域；余额全额支付不能建立区域来源。
- 订单预览和新订单创建均由服务器重新报价；创建请求必须回传用户确认的金额、币种和区域快照，服务器报价变化时返回 `409 quote_changed`，不创建订单也不扣款。已有幂等键仅在完整请求和确认快照与原订单一致时返回原订单，否则返回 `order_invalid`；余额先预留，成功后消耗，失败/取消后释放。
- 支付事件按 `(provider,event_id)` 幂等，渠道、订单、金额、币种或负载摘要冲突时拒绝。
- 邀请人必须同区域且有有效正式授权；受邀账号首次正式授权成功后仅奖励一次：CN 1000 分、
  INTL 140 美分。
- 拒付暂停目标授权并冲正奖励，余额可以为负；负余额账号不能提现。
- CN 可在 CNY 余额达到 5000 分时申请支付宝提现；INTL 不提供提现。

## 7. API 契约

新增 D2S 业务接口统一使用 `/api/v1`，响应包含 `version`、`success`、`request_id` 和稳定
错误码；直接复用的 new-api 账号兼容入口保持其原生响应格式。

- 设备码：`POST /device/authorize|approve|token|cancel`。
- 授权：`GET /license/list|status|keys`，以及激活、切换、模式、续期、离线签发/延长、
  免费/付费撤销、在线心跳/退出、永久确认和人工解绑接口。
- 商业：订单预览/创建/查询、邀请、余额、流水、提现。
- 支付：`POST /webhooks/:provider`，只接收渠道验签后的内部规范事件。
- 下载：`POST /downloads/:target/track` 仅接受白名单目标；访客点击记录来源网络信息但不关联账户，已登录用户须通过邮箱验证和账户登录限制检查；成功追加 `DOWNLOAD` 事件后才由客户端打开外部下载链接。
- 管理：授权列表、人工解绑、提现、用户区域调整。
- 账号：`/api/v1/auth/*` 兼容入口及 new-api 原生账号接口。

完整请求、响应和错误码见 [`desktop2stereo-api.md`](desktop2stereo-api.md)。

## 8. 离线签名与密钥

离线凭证使用 P-256 ES256 紧凑 JWS，固定包含版本、`key_id`、凭证 ID、授权 ID、产品、
设备摘要、模式、功能、签发/生效/过期时间、试用标记和离线档位。私钥只从 Secret 读取，
公钥通过 `/api/v1/license/keys` 发布。

轮换顺序必须是：先发布包含新公钥的客户端，再切换服务端签名键，最后等待旧凭证过期后
退役旧公钥。缺少私钥或签名失败时安全拒绝签发。

## 9. 实施阶段

### S1：基线与授权核心

状态：`verified`。2026-11-03 已满足本阶段退出条件：D2S 测试、全部 39 个含测试 Go 包、根模块及 relaykit 静态检查/构建、敏感信息扫描均通过；全量 GitHub Actions 也通过。三数据库矩阵单独记在 S2；生产演练与外部业务核验继续由 S3–S5 跟踪。

- 2026-11-03 复测最近后台审计改动：固定路径 `controller`、`model`、`service`、`router` 全包测试通过；`go vet -p 1 ./...` 与 `go build -p 1 ./...` 通过。尝试对全部含测试包执行固定路径测试时，360 拒绝启动 `e2e.test.exe`；排除该包重跑后，360 在 `middleware.test.exe` 启动期间移除了二进制，脚本按设计停止。其它包未完成全套复跑；当前全量 Go 测试仍需在 360 暂时退出后执行，不得把本轮部分结果记为全量通过。
- 2026-11-03 继续复跑固定路径全仓 Go 测试：`common`、`constant`、`controller`、`dto` 测试包通过；`e2e.test.exe` 编译完成并计算 SHA-256 后，在启动时被 360 移除，既有脚本停止后续包。未更改 360 设置、隔离区或安全策略；本轮不能记为全仓测试通过。随后当前工作树 `go vet -p 1 ./...` 与 `go build -p 1 ./...` 均通过。
- 2026-11-03 在隔离本机 PostgreSQL 15.14/MySQL 8.4.9 上，用固定路径 runner 执行完整 `model`、`controller`、`service`、`router` 包测试，均通过；`model` 测试进程配置了两种数据库 DSN，并实际运行配置数据库迁移和并发矩阵。全仓 Go 测试仍因 `e2e.test.exe` 被 360 移除而未完成；数据库服务已停止。
- 2026-11-03 再次使用固定路径 runner 执行当前工作树完整 `controller` 包测试，通过（退出码 0）；测试覆盖批量账户筛选、邮箱域名及注册 IP 条件、软删除账户排除等现有控制器回归。此结果不等于全仓 Go 测试通过；批量操作失败审计类型的待修复项仍以其独立授权状态为准。
- 2026-11-03 当前工作树全仓 `go vet -p 1 ./...`、Windows 原生 `go build -p 1 ./...`、Linux/amd64 与 macOS/arm64 交叉构建全部通过（同一固定 GOCACHE/GOTMPDIR）。全仓 Go 测试仍未完成，因此 S1 不据此改为 `verified`。
- 2026-11-03 当前工作树重新执行 `go list ./...` 枚举的全部 39 个含测试包，使用 `scripts/d2s-fixed-go-test.ps1` 固定路径编译并逐包运行，全部通过（含此前受 Windows 临时 EXE 启动限制的 `e2e`、`middleware`、`relay/channel/ollama` 和 `pkg/billingexpr`）；`go vet -p 1 ./...`、`go build -p 1 ./...` 及 `scripts/d2s-sensitive-scan.ps1` 均通过，敏感扫描覆盖 7,843 个跟踪/未跟踪工作树文件。360 托盘进程仍在，但本轮未发现 `360Safe` 进程，测试 EXE 均保留并执行完成。此项更新取代本节较早的“全仓测试未完成”检查点记录；Windows 原生、Linux/amd64 和 macOS/arm64 构建结果仍以此前记录为准。
- 2026-11-03 经用户授权修复批量账户解锁失败审计类型错误：原实现把所有执行失败固定记为 `EMERGENCY_LOCK`，即使预览中的操作是解锁。新增 selection-changed 失败回归，先确认其失败并显示实际记录为错误类型；现根据已验证的预览动作确定类型，并让失败审计沿用该类型。回归转绿后，固定路径 `controller` 全包测试、39 个含测试包的固定路径全量测试、根模块 `go vet -p 1 ./...`、`go build -p 1 ./...`、敏感信息扫描（7,843 个工作树文件）及 `git diff --check` 均通过。PR #1 的 `Backend vet, build, and test` 工作流又在 Ubuntu 上通过根模块与 relaykit 检查、构建和测试。
- 2026-11-03 在提交 `9737a9c` 上复跑 `go list ./...` 枚举的全部 39 个含测试 Go 包，固定路径 runner 全部通过；`go vet -p 1 ./...` 与 `go build -p 1 ./...` 也通过。此次测试过程中 360 未移除或阻止任何固定路径测试程序。对应 PR #1 五项 GitHub Actions 检查均通过。该复验覆盖当前服务器代码，不替代支付沙箱、生产闭环或外部法律意见。

- 导入 new-api、保留上游和 AGPL-3.0 信息。
- 增加 D2S 模型、迁移、路由、授权状态机、设备码、ES256 和在线租约。
- 增加订单、区域、余额、邀请、拒付、提现和管理员 API。
- 2026-10-30 首次完整 Go 测试发现登录会话、订单和离线签名测试夹具未跟上邮箱验证/账户安全控制；仅更新测试数据库表和邮箱验证字段后，完整 Go 套件通过。
- 2026-10-30 默认并行度的 `go test ./...` 曾因 Windows 分页文件不足导致编译进程内存分配失败；以 `go test -p 1 ./...` 重跑全部包通过。`go vet ./...`、`go build ./...`、固定路径扫描（254 个 Go 测试文件）均通过。
- 2026-10-30 `d2s-sensitive-scan.ps1` 已扩展为扫描 Git 跟踪及未忽略的未跟踪工作树文件，2,326 个文件（2,298 跟踪、28 未跟踪）扫描通过。
- 2026-10-31 当前工作树重新运行 `scripts/d2s-sensitive-scan.ps1` 通过：3,633 个文件（2,298 跟踪、1,335 未跟踪）。
- 2026-10-31 修正工作树扫描：`.ci-fixed/` 与 `.tmp/` 本地构建缓存和浏览器临时数据加入 `.gitignore`；敏感扫描器改为先检查二进制标记并按 UTF-8 流式逐行读取，避免加载 Go 缓存导致高内存占用；固定路径扫描现覆盖已跟踪与未跟踪测试。复跑敏感扫描 2,340 个文件（2,298 跟踪、42 未跟踪）、固定路径扫描 266 个 Go 测试文件通过；两个 PowerShell 脚本 AST 解析与 `git diff --check` 通过。
- 2026-10-31 邮箱验证共享存储与一次性消费修复后，Redis 双客户端并发消费用例连续 50 次通过；完整 `go test -p 1 ./... -count=1`、`go vet -p 1 ./...`、`go build -p 1 ./...` 均退出码 0。此前某轮全量测试中 relay HTTP/2 GOAWAY 重试用例曾遇到 socket abort；定向连续 3 次及其后的完整套件均通过，未复现。
- 2026-10-31 路由核对发现设备码批准与设备令牌换取两个登录入口未追加 `LOGIN` 日志；已将两路挂载统一登录日志中间件，记录可解析的 user_id/设备哈希，并在已批准设备码失败重试时只凭存储哈希关联账户。新增成功与失败登录关联回归，`go test ./middleware -run TestD2SLoginActionLogAssociatesDeviceTokenEvents -count=1` 通过；`gofmt`、`go vet -p 1 ./...` 与 `go build -p 1 ./...` 通过。
- 2026-11-02 新增注册失败审计闭环回归：实际调用 `controller.Register` 提交缺少点击验证码的请求，确认追加规范化邮箱、来源 IP、User-Agent、路由和哈希，事件为 `REGISTER/FAILURE` 且未错误关联用户 ID；完整 `go test -p 1 ./middleware -count=1`、全量 `go test -p 1 ./... -count=1`、`go vet -p 1 ./...` 和 `go build -p 1 ./...` 通过。
- 2026-11-02 360 暂停期间复跑全量 `go test -p 1 ./... -count=1`，包含 controller 测试且退出码 0；本地 Vitest 直接运行通过 75 个文件、462 项测试。`pnpm test` 因 `es5-ext@0.10.64` 构建脚本未获批准而停止，未批准该脚本；改用 `node node_modules/vitest/vitest.mjs run --reporter=dot` 完成测试。敏感扫描（2,345 个工作树文件）和固定路径扫描（271 个 Go 测试文件）通过。
- 2026-11-02 修复 relay/channel 的原始 HTTP/2 测试服务器过早关闭连接导致的 Windows Winsock reset：服务器通过 PING/ACK 同步控制帧及响应关闭，GOAWAY 旧连接在客户端断开或 1 秒确认窗口后释放。涉及的 GOAWAY、REFUSED_STREAM 用例连续 10 次通过，`go test -p 1 ./relay/channel` 通过。全量 `go test -p 1 ./... -count=1` 中 relay/channel 已通过，但 Windows 对临时目录里的 `router.test.exe`、`config.test.exe` 返回 `Access is denied`；将这两个包分别编译到固定工作区路径后运行均通过。全量命令仍未整体退出 0，故不记录为全量通过；`git diff --check` 通过。
- 2026-11-02 将含测试的 39 个 Go 包逐包编译到工作区固定路径并运行，全部通过（含 controller、model、relay/channel、router、service 和 setting/config）。该验证绕过了 Windows 对临时目录测试可执行文件的启动限制；常规 `go test -p 1 ./... -count=1` 仍因临时 `router.test.exe`、`config.test.exe` 被系统拒绝而未整体通过，因此分别记录命令结果与固定路径全仓结果。随后 `go vet -p 1 ./...`、`go build -p 1 ./...` 均通过，敏感信息扫描检查 2,345 个工作树文件并通过。
- 2026-11-02 固定路径测试脚本在传入全部 83 个 Go 包时，因根包无测试文件且 `go test -c` 不生成可执行文件而错误退出；脚本现先用一次 `go list` 筛选含内部或外部测试文件的包。PowerShell AST 解析与 `git diff --check` 通过；按该脚本对全部 39 个含测试包逐一编译并运行，全部通过。
- 2026-11-02 修复 `scripts/d2s-fixed-go-test.ps1` 对仅含无测试文件包的处理：过滤 `go list` 输出的空行，筛选结果为空时明确跳过。随后 `go list ./...` 枚举 83 个包并传入脚本，全部含测试包固定路径编译和运行通过（退出码 0）；单独传入 `./cmd/d2s-audit-migrate` 验证无测试包分支正常退出。
- 2026-11-02 在最新工作树重新运行敏感信息扫描：检查 2,346 个文件（2,345 个 Git 跟踪文件及 1 个未跟踪 Vitest 缓存文件），扫描通过。
- 2026-11-02 再次对当前工作树运行敏感信息扫描：检查 2,358 个文件（2,346 个 Git 跟踪文件、12 个未跟踪文件），扫描通过。

退出条件：D2S 单元测试、完整 Go 测试、`go vet`、根模块构建和敏感信息扫描通过。

### S2：三数据库与并发

状态：`verified`；较早的空库/升级矩阵与权限隔离测试通过；当前工作树的 MySQL/PostgreSQL 配置迁移和并发矩阵已复验。

- SQLite、MySQL、PostgreSQL 新建库连续执行两次迁移：`verified`。
- 已在 SQLite、MySQL、PostgreSQL 验证并发试用、绑定、租约、订单、区域锁、余额和支付幂等；人工解绑的跨申请并发审批回归现已验证每账号最多一笔成功。
- 2026-10-31 修复邮箱验证码仅保存在实例内存导致双实例请求被负载均衡到不同实例时无法验证的问题，并将成功核验改为一次性消费：启用 Redis 后，将收件标识与验证码分别使用共享 `SESSION_SECRET` 做 HMAC-SHA-256 摘要并按有效期设置 Redis TTL；Redis 以 Lua 原子比较并删除，避免多实例并发重复消费。Redis 写入失败时发码失败关闭，不回退到节点本地缓存；未启用 Redis 的单实例继续使用内存。发送邮件失败时清除验证码，普通邮件与腾讯 SES 模板显示值均与服务端最长 15 分钟有效期一致。miniredis 双客户端切换、并发原子消费、过期、无 Redis 回退、Redis 故障失败关闭及腾讯 SES 模板边界回归通过。
- 2026-10-31 在 360 暂退后，将 SQLite 并发关键路径及永久解绑并发审批回归各重复 20 次，均通过。当前本机 `CGO_ENABLED=0` 且无 GCC，故未运行 Go race detector；本机未配置 MySQL/PostgreSQL DSN 且没有 Docker，未据此声称本轮完成三数据库验证。
- 2026-10-30 在隔离本地 MySQL/PostgreSQL 数据库执行审计权限脚本，并以应用账号对四张审计表分别尝试 UPDATE/DELETE，共 16 次均被拒绝；此结果只适用于隔离验证库，生产账号仍待部署验收。
- 2026-11-02 加强并实测 PostgreSQL 权限脚本：除表级 UPDATE/DELETE 外，脚本撤销并断言无列级 UPDATE、TRUNCATE、REFERENCES 和 TRIGGER；清理身份在三张留存表仅有 SELECT/DELETE，在留存审计表仅有 SELECT/INSERT。隔离 PostgreSQL 15.14 中临时授予应用账号单列 UPDATE 后，脚本撤销额外权限并通过有效权限断言。MySQL 8.4.9 与 PostgreSQL 15.14 上分别执行权限脚本；运行账号对四表 SELECT 成功，四表 INSERT 均在回滚事务中成功，24 次 UPDATE/DELETE/TRUNCATE 尝试全部被拒绝，未留下测试行。MySQL SHOW GRANTS 显示运行及清理账号仅有预期表级授权。仅证明隔离库配置，不替代生产权限与继承角色验收。
- CI 中保留真实 MySQL/PostgreSQL 服务测试。
- 2026-11-02 扩展隔离数据库矩阵，实测 MySQL 8.4.13 与 PostgreSQL 17.11 上单账户紧急锁定/解除、服务端登录/购买限制读取及对应 `EMERGENCY_LOCK`/`EMERGENCY_UNLOCK` 审计落库；矩阵测试通过，`go test -p 1 ./model -count=1`、`go vet -p 1 ./model`、`go build -p 1 ./model` 通过。两套数据库测试实例随后停止；此证据不替代生产账号权限与生产环境验收。
- 2026-11-03 当前分支数据库复验：以隔离 `d2s_test` 账号连接本机 PostgreSQL 15.14（5433）与 MySQL 8.4.9（3307），固定路径 `model` 全包测试通过，实际执行 `TestD2SSchemaConfiguredDatabases` 和 `TestD2SConcurrentCriticalPaths` 两种数据库子项。覆盖迁移重复执行、用户紧急锁定/解除与审计、授权绑定、工单创建/处理与审计、区域/提现/解绑审核、支付对账及并发路径。`controller`、`service`、`router` 固定路径全包测试随后也通过。数据库 DSN 只在本次子进程环境中注入并清除；本地 PostgreSQL/MySQL 实例已停止。此次使用的是隔离持久测试库，不据此声称本轮测试数据库为空库。
- 2026-11-03 当前工作树再次运行 `scripts/d2s-fixed-go-test.ps1 -Package @('./model','./controller','./service','./router')`，四个包全部通过（退出码 0）。`model` 测试以隔离 `d2s_test` 账号连接 PostgreSQL 15.14（127.0.0.1:5433）与 MySQL 8.4.9（127.0.0.1:3307），配置数据库迁移/并发矩阵子测试实际运行并通过；其余三个包随后通过。测试账号 DSN 只注入本次 PowerShell 子进程，结束后清除；本机 PostgreSQL/MySQL 服务已确认停止。测试使用既有隔离测试库，不代表空库或发布版本升级矩阵在本轮重跑。
- 2026-11-03 GitHub Actions PR run `41559258003` 的 `MySQL and PostgreSQL migrations` 工作通过：真实数据库迁移/并发矩阵与 PostgreSQL 生产角色隔离检查均成功；该次运行绑定提交 `a2b84c431cb70f746ef690265e342a0d84d0b049`。
- 2026-09-09 已将本地与 GitHub Actions 的 Go、Vitest 缓存及临时目录固定到工作区路径；扫描确认 D2S 测试不再调用 `t.TempDir`、`os.MkdirTemp` 或 `os.CreateTemp`，Actions `34278684231` 已通过固定目录回归。
- 2026-09-09 已将登录/注册验证码从拖动拼图切换为自托管点击验证码：后端使用 `go-captcha/v2/click` 生成主图和提示图，服务端保存目标区域并一次性校验坐标数组；登录、注册、前端组件测试和 `controller` 回归均已覆盖。

### S3：支付渠道

状态：`in_progress`。

- 已将 Stripe、Creem、易支付（含支付宝、微信和支付 FM）、Waffo、Waffo Pancake 的渠道回调在各自验签成功后接入 D2S 规范结算处理；代码路径已实现，真实渠道沙箱异常矩阵和日终对账仍待执行。
- PayPal/Paddle 在实现适配器前从首发页面和文档中移除。
- 2026-10-30 复核首发渠道范围：PayPal/Paddle 在 `web/src`、controller/service/model/router 运行时代码中无入口，仅在拒绝它们的回归和文档中出现；购买页回归 11 项与 `TestD2SProviderRegionExcludesUnsupportedLaunchProviders` 均通过。该证据验证当前工作树实现，不代表生产配置已同步。
- 已覆盖开放渠道的统一订单事务、幂等、金额/币种校验，以及 Stripe、Creem、易支付（含 `TRADE_REFUND`）、Waffo、Waffo Pancake 的冲正回归；真实渠道沙箱矩阵和日终对账仍待执行。
- 2026-11-03 新增 controller 回归：相同用户以幂等键重试时，若请求报价与已存订单快照一致，即使当前商品报价已变化，也返回原订单及其金额快照；`go test -p 1 ./controller -run '^TestD2SOrderCreate(ReplaysHistoricalQuoteSnapshot|RejectsChangedServerQuoteBeforeCreatingOrder)$' -count=1` 通过。不同请求快照仍由幂等校验拒绝。
- 2026-11-02 已从 Stripe、Creem 和 Waffo Pancake 的已验签支付成功回调提取可用付款方标识：Stripe/Creem 优先取客户 ID、缺失时取邮箱，Waffo Pancake 取 `buyerEmail`；邮箱规范化为小写。订单只保存带 `d2s-payment-account-v1` 域分离密钥的 HMAC-SHA-256 摘要，缺失时为 NULL。模型只接受这些 provider 的 32 字节摘要；controller 的摘要、签名结算、冲正后留存和无明文回归及 model 拒绝不支持 provider 的回归均通过。Creem 与 Waffo Pancake 回调字段依据 [Creem Webhook 文档](https://docs.creem.io/skills/creem-api/WEBHOOKS) 和 [Waffo Pancake Go SDK](https://pkg.go.dev/github.com/waffo-com/waffo-pancake-sdk-go)。易支付和 Waffo 的独立付款账户字段此前未确认，继续留空。
- 2026-11-03 对照固定版本 Waffo Go SDK webhook 示例，补充从 `paymentInfo.payMethodResponse.maskCardData` 生成 HMAC 摘要；只处理掩码值，不读取其 `payMethodProperties` 令牌，不将本站传入的 `userInfo.userEmail` 误作付款标识。若字段缺失或格式不符则留空。新增掩码字段提取、缺失/无效字段和成功结算留存回归；模型允许 Waffo 摘要。全仓 `go vet -p 1 ./...` 和 `go build -p 1 ./...` 通过；Go 测试 exe 仍受本机安全软件影响，新增回归尚待运行。字段依据 [Waffo Go SDK v1.3.2 webhook 示例](https://github.com/waffo-com/waffo-go/blob/v1.3.2/README.md)。
- 2026-11-02 固定支付重试使用订单创建时的邮箱快照：Stripe、Creem 和 Waffo Pancake Checkout 使用订单邮箱快照；Creem 元数据中的用户标签基于 user ID 稳定生成。Stripe 回归验证邮箱快照和订单作用域幂等键在重试时保持一致；`go test -p 1 ./controller -run '^(TestStripeCheckoutUsesOrderScopedIdempotencyKey|TestD2SOrderCheckoutEmailUsesOrderSnapshot)$' -count=1`、`go vet -p 1 ./controller`、`go build -p 1 ./controller` 通过。
- 2026-11-02 收敛 Creem Checkout 诊断日志：不再记录用户邮箱明文或完整支付链接，只记录邮箱/链接是否存在及订单、响应标识。隔离 HTTP transport 回归确认返回链接功能不变且日志不含邮箱、token 或完整链接；`TestCreemCheckoutLogsOmitEmailAndCheckoutURL` 通过。
- 2026-11-02 支付渠道 Webhook 日志统一只记 Gin 路由模板或 URL path，不再记录完整 RequestURI 查询串；新增回归验证 `sign`、`param` 查询值不会进入日志路径。
- 2026-11-02 Waffo `PaymentRequestID` 改为从内部订单 ID 稳定生成 32 位十六进制幂等键；标准 UUID 去除连字符，非 UUID 使用域分离 SHA-256 截断，完整内部订单 ID 继续作为 `MerchantOrderID` 关联回调。新增长度、稳定性和非 UUID 回退回归通过（[Waffo Go SDK](https://pkg.go.dev/github.com/waffo-com/waffo-go) 标注幂等 ID 最长 32 字符）。
- 2026-10-30 核实易支付重试边界：当前 `go-epay` 的 `Purchase` 在本地生成签名表单，稳定使用 D2S `order.ID` 作为 `out_trade_no`；新增回归验证同一订单多次生成的 URL、签名参数、订单号和金额完全一致。易支付公开文档将 `out_trade_no` 定义为商户订单号并支持按该字段查单，但没有规定重复提交是否返回既有订单，因此不宣称网关幂等，仍需用实际商户沙箱验证重复表单提交、重试和查单行为（[SDK](https://github.com/Calcium-Ion/go-epay)、[易支付接口文档](https://pay.yzfpay.com/doc/v1_legacy_api.html)）。
- 已新增统一 `/webhooks/:provider` HTTP 桥的 8 渠道回归：每个开放渠道均验证专用密钥、订单金额/币种/区域、`paid` 状态变更、重复事件幂等和错误签名拒绝；Stripe、Creem、易支付及 Waffo 原生 HTTP 签名回调与重复事件隔离 SQLite 回归已通过（Waffo 使用临时 RSA 密钥覆盖 SDK 验签、拒绝无效签名和重复结算）；`go test ./controller -count=1` 通过；真实渠道沙箱矩阵和日终对账仍待执行。
- 2026-10-31 新增 Waffo Pancake 原生 HTTP 无效签名拒绝用例；首次执行时 Windows 对 Temp 和工作区固定临时目录中的 `controller.test.exe` 返回 `Access is denied`。360 暂时退出后重跑完整 `go test -p 1 ./... -count=1`，所有 Go 包通过；360 误报样本仍需由用户提交给厂商复核。
- 2026-10-31 Go 验证：设备码登录新增回归 `go test ./middleware -run TestD2SLoginActionLogAssociatesDeviceTokenEvents -count=1` 通过；一次全仓串行测试中 controller 测试程序被 Windows 拒绝启动（`controller.test.exe: Access is denied`），另有两项 HTTP/2 测试出现 socket abort。360 暂时退出后，`go test -p 1 ./... -count=1` 完整重跑通过，因此前述问题未在本次复测中复现。
- 对所有开放渠道执行完整沙箱异常矩阵和日终对账。
- 2026-11-02 当前环境复核：Stripe、Creem、易支付、Waffo、Waffo Pancake、Payment FM 沙箱凭据变量均未设置，仓库根目录 `.env` 不存在；本次仅检查变量是否存在，未读取或输出密钥值，因此无法发起真实沙箱交易。后续须从各支付渠道的隔离沙箱环境注入凭据，不得以生产凭据替代。

### S4：用户与管理员网页

状态：`in_progress`。

- 2026-11-03 按 Web Interface Guidelines 复核管理员导出审批队列，发现审批状态刷新、获批 CSV 下载、刷新待审批、批准和拒绝五个按钮缺少可见键盘焦点样式。已统一加入 `focus-visible` 焦点环，并扩展后台回归验证五个按钮；后台控制台测试 12/12、全量 Vitest 77 文件/478 项、`tsgo -b`、Rsbuild 生产构建、目标文件 oxlint/oxfmt 检查及 `git diff --check` 通过。真实屏幕阅读器验收仍待完成。
- 2026-11-03 依据代码数据清单编制[隐私政策内部草稿](privacy-policy-draft.md)，明确标为不可发布，并列出主体、生产供应商/处理地区、提现启用与保护、业务数据期限、用户请求渠道及实名适用意见等未决事实。生产 `legal.privacy_policy` 未修改，草稿不能替代运营负责人和法律顾问审核；隐私政策 readiness 仍未通过。

- 2026-11-03 隐私政策事实盘点发现提现路径会收集 `AlipayAccount` 和 `RealName`，并以明文字段保存在 `d2s_withdrawal_requests`；这不等同证件核验，但说明“网站只有邮箱”不准确。新增内部数据清单，区分 366 天安全审计、授权/交易/工单留存和未知的生产服务商配置；SEC-001 与管理规格同步区分订单付款标识的 HMAC/脱敏和提现资料明文。生产提现是否启用、必要性、访问权限、安全方式与期限仍待核，未据此自动改变业务或声称政策已完成。
- 2026-11-03 补齐超级管理员财务/授权后台审计：订单、支付事件、余额、提现、授权、解绑申请和签名公钥查看写入带筛选范围及数量的 `VIEW_*` 审计；提现审核、人工解绑审核、账户区域修改和签名密钥退役的成功审计与业务变更同事务提交，失败尝试单独审计。controller 回归覆盖四类业务变更在审计表不可用时回滚及各成功路径；`scripts/d2s-fixed-go-test.ps1` 将测试二进制写入固定工作区路径后，controller、model、service、router 四个包测试全部通过；`go vet -p 1 ./controller ./model ./service` 和 `go build -p 1 ./controller ./model ./service` 通过。常规临时路径运行 service 测试曾被 Windows 拒绝启动，固定路径运行已通过。MySQL/PostgreSQL 测试 DSN 和本地服务均不可用，因此跨数据库矩阵未验证。改动尚未部署生产。
- 2026-11-03 后续 fail-closed 复核发现提现和人工解绑列表在审计表不可用时虽拒绝返回结果，错误体仍经过通用错误处理；现统一返回 `AUDIT_WRITE_FAILED`。新增回归覆盖提现、解绑、订单、支付事件、余额、授权和签名公钥七类敏感列表，审计表不可用时验证不返回相应私密数据；测试代码已格式化且 `go vet -p 1 ./controller` 通过。该扩展回归尚未执行，360 相关测试二进制启动阻断见 [Windows 误报与签名处理说明](windows-360-false-positive-and-signing.md)。
- 2026-11-03 补齐首页匿名下载点击留痕：下载路由改为可选身份认证，仍只接受三个外链白名单目标；有效登录账号须邮箱已验证且未被禁登录，访客事件写空 user_id/email 并留存可用来源网络字段，认证失败/封禁/审计失败均不跳转。首页各下载入口现在先完成日志请求再打开外链。新增控制器、路由和前端回归；前端 Vitest 2/2、`tsgo -b`、Rsbuild 生产构建、`go vet -p 1 ./controller ./router`、`go build -p 1 ./...` 与 `git diff --check` 通过。Go 控制器/路由回归尚待运行；外部文件传输是否完成仍无法从点击事件证明。SEC-001 已同步访客首页点击留痕和绕过本站外链时不可观测的边界。
- 2026-11-03 先前检查点：`go vet -p 1 ./...`、`go build -p 1 ./...`、前端 `tsgo -b`、Rsbuild 生产构建、Vitest 77 个文件/478 项、格式检查、生产 readiness selftest 和 `git diff --check` 通过；敏感信息扫描检查 7,842 个跟踪及未跟踪文件并通过。该检查点之后增加的管理审计和批量筛选回归未全部复验。selftest 验证 HTTP-only 版本及签名密钥核对、拒绝空/占位隐私政策、非法客服链接和不兼容参数组合。新增 MySQL/PostgreSQL 配置数据库矩阵覆盖提现审核、人工解绑审核、账户区域修改与审计同事务提交，但该扩展矩阵尚未运行。此前固定路径 Go 测试曾因 360 移除 `pkg/billingexpr` 测试二进制而中断；详情见 `windows-360-false-positive-and-signing.md`。
- 2026-11-03 当前前端工作树复验：Vitest 77 个文件/478 项、`tsgo -b` 和 Rsbuild 生产构建通过。全仓 Oxlint 未通过，包含多个非 D2S 模块的既有错误；D2S 定向 lint 指出 `src/features/d2s/admin.tsx:541` 一处规则错误。全仓格式检查也未通过，其中 D2S 范围列出 `withdrawal-request-form.tsx`、`admin.test.tsx`、`license-card.test.tsx`。依仓库 AGENTS.md，已先请求授权修复这 4 项；修复及独立屏幕阅读器验收未完成，S4 保持 `in_progress`。
- 2026-11-03 经用户授权修复上述 D2S 相关项：将许可区域编辑行改为随服务端区域变化重新挂载，避免同步状态的 effect，并新增服务端区域更新时丢弃未保存草稿的回归测试；格式化另外两个 D2S 测试/组件文件。D2S 测试 8 个文件/36 项及全量 Vitest 77 个文件/479 项通过；`tsgo -b`、Rsbuild 生产构建、4 个 D2S 文件 Oxlint/Oxfmt 定向检查和 `git diff --check` 通过。全仓 Oxlint 的非 D2S 既有错误未处理，真实屏幕阅读器验收仍待完成，S4 保持 `in_progress`。
- 2026-11-03 加强固定路径 Go 测试 runner 诊断：启动测试 EXE 前输出 SHA-256，并在构建后、启动异常和运行后检查样本是否存在；文件消失时记录包名、路径和哈希后停止，不自动重试或改变安全软件设置。PowerShell AST 语法检查和 `git diff --check` 通过；未运行 Go 测试程序，避免触发当前 360 拦截。
- 2026-11-03 继续审查购买记录视图审计，补记 user_id 筛选并将 email/status 规范化后写入审计详情；规格同步，新增成功筛选和审计存储失败关闭回归用例。`go vet -p 1 ./...`、`go build -p 1 ./...`、生产就绪自检脚本及 `git diff --check` 通过。固定路径诊断确认 `pkg/billingexpr` 测试 exe 可编译并生成 SHA-256 `BD8F25A8C9E6A04467F38279287A7E625BCA6BC4FB0A0B2989D49D5A2ED0630C`；启动调用报 Windows“系统找不到指定的文件”，随后该文件从路径消失。检查时 `360Safe`/`360tray` 正在运行，Windows Defender 日志无匹配记录；强烈疑似 360 隔离，但未获得其检测名称或隔离记录，故不能认定厂商已确认。全仓测试及新增回归未通过运行验证，详情见 `windows-360-false-positive-and-signing.md`。
- 2026-11-03 补齐批量账户处置对存量账号的筛选：邮箱域名和注册时间窗口改以当前用户表邮箱/创建时间查询，不再因旧账户没有新注册快照而漏选；IP CIDR 仍仅按不可变注册 IP 快照匹配，缺失 IP 不推断，并从匹配结果排除已软删除账户。管理规格已同步。新增无注册快照历史账户的预览/执行及 IP 段命中/软删除排除回归；`gofmt`、`go vet -p 1 ./controller` 和 `go build -p 1 ./...` 通过，回归尚待在 360 退出后执行。
- 2026-11-03 在提交 `994f9ff` 后复核并更新上述测试状态：固定路径 runner 的 controller、model、service、router 全套测试通过，覆盖高权限财务/授权审计、fail-closed 敏感列表、存量账户批量筛选、匿名下载点击和 Waffo 掩码卡摘要；`go vet -p 1 ./...` 与 `go build -p 1 ./...` 通过。360 进程当时仍在运行，但固定路径测试程序完整保留并通过。当前机器无 MySQL/PostgreSQL 服务、命令或测试 DSN，故新增数据库矩阵仍未执行；该分支未开 PR，因此 GitHub CI 尚无运行结果。生产未部署此分支。
- 2026-11-03 最新本地前端复验：从 `web/` 目录直接运行现有 Vitest 可执行入口，77 个测试文件、478 项全部通过；`tsgo -b` 与 Rsbuild 生产构建通过。该项复验覆盖当前前端工作树，不替代真实屏幕阅读器或生产用户/管理员闭环验收。
- 2026-11-03 先前自测检查点：Go 全量 `go test -p 1 ./... -count=1`、`go vet -p 1 ./...`、`go build -p 1 ./...` 均通过（83 个包）；SQLite 回归确认新库核心迁移不会创建证件核验表。前端 Vitest 全量 76 个文件、476 项通过；`tsgo -b`、Rsbuild 生产构建、生产就绪自检有效配置及 9 类拒绝场景通过。日志、注册快照和管理员审计保留下限为 366 天。此前已用 Playwright/Edge 对独立后台、钱包报价/建单/Checkout 及支持工单模拟流程完成本地浏览器验收，API 均由本地模拟；真实屏幕阅读器、生产后台及第三方支付沙箱验收仍未完成。后续加入的财务/授权审计、购买记录审计和 Waffo 掩码支付标识改动，以紧随其后的测试记录为准；S4 与自评估门槛保持未完成。
- 2026-11-02 补充支持工单 Playwright 浏览器闭环：用本机 Edge 和隔离 API mock 实际完成邮箱/密码登录、行为验证码、工单提交、受理编号展示和“我的工单”读取客服回复；确认提交与列表请求携带 Bearer 会话。验收暴露 `zhCN` 被直接传入 `Intl.DateTimeFormat` 导致用户工单页进入错误页；现改为使用 `toIntlLocale` 转换，并新增简体中文日期回归。工单 Vitest 6/6、前端全套 76 文件/474 项、`tsgo -b`、Rsbuild 生产构建均通过。此浏览器流程由本地 API mock 驱动，不代表生产后台、真实屏幕阅读器或支付沙箱验收。
- 2026-11-03 工单后台状态筛选的非法值原先被模型层当作查询故障而返回 500；现由 controller 白名单校验四种合法状态，非法输入返回 400 `INVALID_STATUS` 并追加 `VIEW_SUPPORT_TICKETS/FAILURE` 审计。回归先复现 500，再验证修复；工单定向测试、完整 controller 测试、`go vet -p 1 ./controller`、`go build -p 1 ./controller` 和 `git diff --check` 通过。管理规格已同步。
- 2026-11-03 复核投诉入口文案时发现旧版多语言资源错误表示访客可提交但不会关联账户，与真实的登录强制路由/API 不一致。已在右侧联系面板明确提示“注册并登录后提交”，更新英语、法语、日语、俄语、越南语、简体和繁体中文文案，并新增未登录路由守卫及简体中文文案回归。支持页面定向测试 8/8、全量前端测试 76 文件/476 项、`tsgo -b`、Rsbuild 生产构建及目标文件格式检查通过。另将证件核验兼容模型移出新库自动迁移，保留既有数据表且不自动删除；新增 SQLite 迁移回归确认新库不创建该表，无证件核验路由。该项不替代生产验收。

- 2026-10-31 Edge 156 本地真实服务后台验收：用隔离 SQLite 与临时测试管理员登录独立 `super_admin` 后，后台会话、行为日志、注册记录、购买记录及两类 CSV 导出接口均返回 200；CSV 为 UTF-8 且包含要求的列和命名格式。注册、购买、紧急限制、身份核验、日志留存模块可打开，该次验收时留存下限为 180 天（已于 2026-11-02 调整为 366 天）；当时身份核验模块可打开属于历史实现状态，当前页面和 API 已停用。未执行账户限制或日志清理；本地服务与浏览器已停止，测试数据仅留在系统临时目录。此项不替代真实用户支付闭环、支付渠道或独立屏幕阅读器验收。
- 2026-10-31 补充 Edge 156 本地浏览器验收：在新建临时 SQLite 和测试管理员上完成 new-api 初始化、独立后台登录、行为日志/注册/购买/紧急限制/身份核验/留存五个模块切换，并从无障碍树核对紧急限制表单控件名称；后台会话 Cookie 实际标记 `Secure`、`HttpOnly`、`SameSite=Strict`，退出后回到独立登录页。未提交账户限制、身份核验、CSV 导出或日志清理操作；该次验收不证明生产部署、支付闭环或独立屏幕阅读器验收。服务与浏览器已停止，监听端口已关闭；由于自动审批拒绝递归删除本轮临时测试目录和自签证书，它们仍保留在本机 `.tmp` 和当前用户证书库，后续清理待处理。
- 2026-10-31 补齐账户购买紧急限制的控制器回归：启用 `purchase_blocked` 后，订单预览、建单、Checkout、付费解绑和离线续期五个付费入口均返回 `423 ACCOUNT_PURCHASE_BLOCKED`，并确认未创建订单；定向测试与完整 `go test -p 1 ./... -count=1` 通过。
- 2026-10-31 修复设备码登录绕过账户禁登录限制：令牌兑换在签发会话前检查账号限制，被锁账号返回 `423 account_login_blocked` 并消费已批准设备码；隔离 SQLite 回归确认无会话生成、设备码不可重用；登录审计回归确认该请求关联账号与设备摘要并记为 `BLOCKED`。定向测试、完整 `go test -p 1 ./... -count=1`、`go vet -p 1 ./...` 和 `go build -p 1 ./...` 通过。
- 2026-10-31 修复 refresh token 绕过账户禁登录限制：刷新前检查账号锁定状态，锁定账号返回 HTTP 423 `ACCOUNT_LOGIN_BLOCKED`，不轮换刷新令牌并清除浏览器 refresh cookie；隔离 SQLite 控制器回归确认令牌摘要不变、会话仍有效（强制下线仍为独立操作）。service/controller 定向测试和全量 `go test -p 1 ./... -count=1` 通过；首轮全量测试遇到两个非相关偶发失败，各自定向复测及第二轮全量测试均通过。随后 `go vet -p 1 ./...`、`go build -p 1 ./...`、`git diff --check`、敏感信息扫描（2344 个工作树文件）和固定路径扫描（270 个 Go 测试文件）通过。API 行为已同步写入 `desktop2stereo-api.md`。
- 用户页：授权、设备、模式、订单、邀请、余额和提现。
- 管理页：授权、解绑、提现、拒付、负余额、区域和签名键。
- 当前已提供用户工作台、管理员工作台和上一 UTC 日内部对账摘要；管理员审核写操作继续复用管理员审计中间件。
- 已补充购买页报价确认：显示服务器返回的商品、授权、渠道、区域和金额；确认后创建订单并提交金额/币种/区域快照，服务器返回 `409 quote_changed` 时要求用户重新核对报价。余额支付、付费解绑目标、开放渠道、幂等重试和预览失败控件恢复均已有组件回归；管理员区域保存失败、提现批准和人工解绑拒绝也已回归验证。支付页真实屏幕阅读器、生产端到端浏览器和第三方支付闭环仍待完成。
- 报价请求期间新增中英双语的屏幕阅读器状态提示；完整键盘/屏幕阅读器验收仍未完成。
- 2026-11-02 按 Web Interface Guidelines 对购买页和独立后台页完成静态可访问性审查并修复：金额/日期改用 `Intl` 本地化格式；表单控件补充名称、自动填充与拼写检查语义；加载/空结果状态支持播报；后台各页面增加跳转到主内容入口；通用按钮改为限定属性过渡并尊重减少动态效果设置。购买页和后台页 18 项 Vitest 通过，`tsgo -b`、目标文件格式检查及 `git diff --check` 通过。该项属于代码层静态审查，真实屏幕阅读器和完整键盘端到端验收仍待完成。
- 2026-11-02 后续代码复核发现独立后台原生操作按钮缺少明确悬停反馈，日志/注册/购买结果表的横向滚动容器未提供键盘焦点入口；现已为操作按钮补充悬停与可见焦点状态，并为三类表格添加命名、可聚焦的滚动区域。新增回归检查区域键盘焦点语义和按钮交互样式；后台路由 11 项测试通过，`tsgo -b`、目标文件 `oxlint`/`oxfmt --check` 与 `git diff --check` 均通过。未完成真实屏幕阅读器实测。
- 2026-11-02 补齐后台用户行为日志、注册记录和购买记录的完整前端分页：统一按服务端 `total` 翻页，注册/购买查询带 `page` 与 `page_size=50`，切换模块和修改筛选后清除旧分页结果并回到第一页。新增 51 条边界的三类列表分页组件回归；后台路由 8 项测试、`tsgo -b`、目标文件 oxlint/oxfmt 检查通过。
- 2026-11-02 完成当前工作树前端全量回归，75 个测试文件、465 项通过；`tsgo -b` 通过，Rsbuild 生产构建成功（10.5 秒）。
- 2026-11-02 后续变更后从 `web` 目录重跑全量 Vitest，75 个测试文件、467 项通过；后台路由单文件 11 项通过，`tsgo -b` 和 Rsbuild 生产构建通过（11.6 秒）。目标文件 `oxlint`/`oxfmt --check` 与 `git diff --check` 通过。
- 2026-11-02 使用 Playwright 启动本机 Edge，对独立后台和隔离模拟 API 完成浏览器交互验收：登录页按 Tab 可到达跳过链接、必填邮箱、密码和登录按钮，并可用 Enter 提交模拟登录；主内容跳转链接可聚焦并将焦点移至唯一 `main`；日志筛选返回 50 条、服务端分页可加载第 2 页余下 1 条，完整展示 14 个审计字段且 CSV 文件名符合约定；注册、购买、紧急限制模块切换正常，紧急处置输入控件可访问名称完整；390px 视口文档无横向溢出且没有页面脚本异常。API 全部由本地路由模拟，未触发限制、清理或生产请求。该验收不等同真实屏幕阅读器或生产后台验收。
- 2026-11-02 Playwright 对钱包页及模拟 API 的购买报价流程进行补充验收时，发现 `Intl.NumberFormat('zhCN')` 在报价弹窗渲染中抛出 `RangeError` 并触发 500。修复为在购买金额与日期格式化前调用现有 `toIntlLocale`，将内部语言码转换为 BCP 47 标签；复测报价弹窗展示 USD 29.90、商品/支付/区域信息，Escape 关闭弹窗，唯一 `main` 保留，390px 视口无横向溢出且无脚本异常。补充 Playwright 模拟完整的报价→建单→Checkout 前端链路：建单请求含产品、渠道、2990 美分、USD、INTL 和幂等键，Checkout 返回仅本地使用的模拟地址；所有 API 均由浏览器路由桩响应，未连接应用数据库或真实支付渠道。新增 `zhCN` 报价格式回归后，完整 Vitest 75 个文件/465 项通过，`tsgo -b`、相关 oxlint/oxfmt 检查及 Rsbuild 生产构建（10.5 秒）通过；真实屏幕阅读器、生产购买与支付回调仍待验收。
- 管理员路由、侧栏入口和后端管理 API 已覆盖角色权限回归；独立后台按角色打开首个授权模块，账户操作员紧急限制界面有确认取消与请求载荷组件回归。批量处置在筛选条件或操作参数变化、以及刷新预览失败时立即废弃旧预览，并在请求中禁用相关输入；组件回归覆盖旧预览失效与新预览请求失败。永久解绑申请创建与管理员批准现按账号授权资料行锁串行化，避免并发审批突破每账号终身一次限制；SQLite、MySQL、PostgreSQL 并发回归均通过。沿用 new-api React/Semi UI/i18next，完整可访问性、组件和端到端支付页面测试仍待完成。
- 管理员行为日志 CSV 增加控制器回归：校验 14 列完整字段、时间/账号/邮箱/IP/行为筛选、公式注入防护，以及导出 `STARTED`/`SUCCESS` 审计记录；审计表不可用时拒绝导出且不泄露日志数据；501 条记录的导出跨过 500 条分页边界后无遗漏、无重复。对应定向 Go 测试通过。后台页面导出组件回归验证筛选参数、会话凭据、服务端文件名和下载调用；该路由新增 `audit_reader` 全部 14 个日志字段展示回归后共 7 项 Vitest，TypeScript 与格式检查通过。完整网页套件复跑为 75 个测试文件、462 项全部通过。
- 2026-11-03 D2S 页面修复和批量解锁审计修复合并提交 `a2b84c4` 后，Draft PR #1 的 D2S 前端 CI（类型检查、测试、生产构建）、D2S 后端专项测试、完整 Go 测试、MySQL/PostgreSQL 矩阵和部署配置检查均通过；通用 CI 的全量前端测试也通过。全仓 Oxlint 既有非 D2S 错误仍未解决，真实屏幕阅读器验收仍待完成。
- 已补充系统维护更新组件回归：更新期间按钮禁用、状态轮询、成功后的版本刷新，以及失败提示和按钮恢复；完整浏览器端到端验收仍待完成。
- 2026-10-30 使用本地 Vitest 执行完整网页回归：73 个测试文件、454 项测试通过；Node 输出 `--localstorage-file` 路径警告。已完成部分本地 Edge 键盘与可访问性树验收；真实屏幕阅读器、完整浏览器端到端和支付渠道流程仍待完成。
- 2026-10-30 已为后台批量筛选、身份核验和留存控件补充可见标签，为模块选择按钮暴露选择状态，并为分页与批量预览状态增加屏幕阅读器播报，为结果表增加可访问名称与列标题语义。`tsgo -b`、Rsbuild 生产构建和该路由 `oxfmt --check` 均通过；这些静态检查不替代浏览器键盘与屏幕阅读器验收。
- 2026-10-30 修正管理员 CSV 下载的 blob URL 生命周期：将下载锚点挂入文档后触发下载，再延迟回收 URL。新增 Vitest 回归验证点击时锚点已挂载且 URL 延迟 1 秒回收；针对该用例的 Vitest、`tsgo -b` 和相关文件 `oxfmt --check` 均通过。Rsbuild 生产构建此前已通过；跨浏览器下载时序仍待真实浏览器验收。
- 2026-10-30 修正管理员强制下线审计时序：操作开始记为 `STARTED`，只有会话撤销、认证版本提升和在线租约释放全部完成后追加 `SUCCESS`，失败则记录阶段和已撤销会话数量并追加 `FAILURE`。成功/失败控制器回归均通过。
- 2026-10-30 将身份核验状态与 `REVIEW_IDENTITY/SUCCESS` 审计记录纳入同一事务，并新增证件号仅存 SHA-256 哈希及审计插入失败时事务回滚的控制器回归。
- 2026-10-30 修复后台管理员失败登录锁定计数：加入 15 分钟失败窗口，窗口过期、锁定到期及登录成功时重置计数，并补窗口边界和锁定到期回归。
- 2026-10-31 后台登录控制器对畸形 JSON、无效邮箱和缺失/超长密码请求也先追加 `LOGIN_ADMIN/FAILURE` 审计；密码不写入审计详情，若审计失败则返回 500。新增对应 controller 回归；定向回归与全量 `go test -p 1 ./... -count=1` 通过。
- 2026-10-31 修复已登录但角色不足时权限中间件直接拒绝而不留审计的问题：追加 `ADMIN_PERMISSION_DENIED/FAILURE`，记录操作者、角色、所需角色、请求方法、来源 IP 和路由模板；审计写入失败或审计详情序列化失败时返回 500 且不进入受限处理器。审计详情使用项目 `common.Marshal` 封装。新增正常审计与审计库不可用两项中间件回归；全量 `go test -p 1 ./... -count=1`、`go vet -p 1 ./...` 和 `go build -p 1 ./...` 均通过。本轮测试回归使用 SQLite；环境未配置 MySQL/PostgreSQL 测试 DSN 且无 Docker，故未对该中间件路径声称完成三数据库复测。
- 2026-10-31 修复日志、注册、购买查询/导出及受控留存操作的参数拒绝或查询失败分支忽略审计写入错误的问题：追加失败审计不可用时统一返回 `AUDIT_WRITE_FAILED`；注册/购买/用户日志相关结果在失败审计写入前不返回。新增 5 类读取/导出缺失审计表回归、注册查询失败审计及低于 180 天留存拒绝审计回归。定向和完整 controller、全量 `go test -p 1 ./... -count=1`、`go vet -p 1 ./...`、`go build -p 1 ./...` 通过；本轮使用 SQLite，当前环境无 MySQL/PostgreSQL DSN 或 Docker，未对这些控制器路径做三数据库复测。
- 2026-10-31 加固批量紧急处置、证件审核与单账户紧急控制的审计失败路径：失败响应在失败审计成功写入前不返回；批量预览与成功审计改为同事务，审计写入失败即回滚预览；批量参数 JSON 使用项目 `common.Marshal/common.Unmarshal` 封装。CSV 开始发送后无法更改 HTTP 状态，因此保留先行 `STARTED` 审计，并在终态审计失败时写入服务端错误日志。新增缺失审计表时的 fail-closed 与批量预览事务回滚回归；D2S 管理器定向测试、全量 `go test -p 1 ./... -count=1`、`go vet -p 1 ./...`、`go build -p 1 ./...` 均通过。该轮数据库回归为 SQLite，未进行 MySQL/PostgreSQL 控制器复测。
- 2026-10-30 批量紧急处置在执行事务内重算筛选用户集合；集合相对预览发生变化时拒绝操作并保留失败审计，成功和变化拒绝控制器回归均通过。
- 2026-11-03 扩展批量紧急处置失败审计：过期或已消费预览若属于当前管理员且动作快照可读，失败审计仍准确区分 `EMERGENCY_LOCK` 与 `EMERGENCY_UNLOCK`；不存在预览或无法确认动作时记录 `BATCH_EMERGENCY_APPLY`，不再误记为锁定。新增过期解锁回归，先确认旧实现失败，再验证修复通过；另覆盖已消费解锁和无预览通用失败类型。解锁选择变化回归、四项定向测试、完整 controller 测试、`go vet -p 1 ./controller` 与 `go build -p 1 ./controller` 通过。
- 2026-10-30 本地 Edge 156 浏览器验收：后台独立登录页渲染正常，邮箱/密码字段在可访问性树中有名称，Tab 可到登录按钮；未登录访问 `/d2s` 正确跳转登录页。使用仅运行于本机的假 API 会话打开 `/wallet`，确认 D2S 购买页可渲染、报价弹窗显示金额/币种/区域并提供取消/确认按钮，Escape 能关闭弹窗。验收发现布局容器嵌套两个 `main` 且 Skip to Main 无目标；已将内层 `Main` 改为 `div`，并将唯一 `main` 设为 `id=content`、`tabIndex=-1`；浏览器验证 Tab 后 Enter 将焦点移至该主地标，页面仅保留一个 `main`。后台登录、用户授权页和钱包页的可访问性树与基础键盘路径已检查。该验收使用假 API，不验证真实后台操作/支付渠道；未运行独立屏幕阅读器，完整屏幕阅读器与端到端验收仍待完成。
- 2026-10-30 新增 `Main` 布局无障碍回归，断言其嵌入认证壳层时不会创建嵌套 `main` 地标；针对测试通过 Vitest，`oxfmt --check` 与 `git diff --check` 通过。
- 2026-10-30 使用本地 Edge 156.0.4314.50 CDP 补充验收：后台独立登录页的邮箱和密码输入在关联标签与可访问性树中均有名称；未登录访问 `/d2s` 跳转到 `/sign-in`。通过仅在本机生效的假 API 会话，`/d2s` 与 `/wallet` 页面均正常渲染且各只有一个 `main` 地标；钱包页展示购买类型、余额与 Stripe 控件，服务器报价弹窗显示 USD 29.90 和 INTL 区域，按 Escape 后关闭。该模拟不触达真实用户、后台或支付渠道；未运行独立屏幕阅读器。盲测技能文件不在当前技能目录，故按已授权范围改用本地浏览器 CDP。

- 2026-10-30 修复独立后台登录表单的 Enter 提交：邮箱/密码字段处于原生必填表单中，按 Enter 提交登录；新增 user-event 键盘回归。该测试通过，完整网页测试为 75 个文件、456 项通过；`tsgo -b`、Rsbuild 生产构建、修改文件 `oxfmt --check` 均通过。仍未进行真实屏幕阅读器或生产端到端支付验收。
- 2026-10-30 将后台会话初始检查提示标记为 `role=status` 与 `aria-live=polite`，保留 `main` 地标；新增异步状态播报回归。完整网页测试 75 个文件、457 项通过，`tsgo -b` 和 Rsbuild 生产构建通过；真实屏幕阅读器和生产端到端支付验收仍待完成。
- 2026-10-31 当前工作树直接运行 Vitest 全套：75 个测试文件、462 项通过，退出码 0；Node worker 发出 `--localstorage-file` 未提供有效路径的告警，`vitest.config.ts`/`test-setup.ts` 未设置该参数，Node v25.2.1 且当前 `NODE_OPTIONS` 未设置；来源仍待定位。该结果不覆盖独立屏幕阅读器和生产端到端支付验收。
- 2026-10-31 修复管理员页面测试 `fetch` mock 参数类型不完整导致的 TypeScript 错误；`tsgo -b` 通过，后台测试文件 7/7 通过，相关文件 oxlint 与 oxfmt 检查通过。随后完整网页套件再次通过：75 个文件、462 项测试；Node worker 的 `--localstorage-file` 无效路径告警仍出现。
- 2026-10-31 360 暂退后复跑前端全套：直接调用 `web/node_modules/.bin/vitest.CMD`，75 个测试文件、462 项通过；`tsgo.CMD -b` 通过；`rsbuild.CMD build` 通过（81.4 秒）。pnpm 命令仍被 `es5-ext@0.10.64` 未批准构建脚本拦截，因此此次使用现有本地可执行文件；Vitest 的 `--localstorage-file` 无效路径告警仍存在。
- 2026-10-31 定位并消除 Vitest 的 `--localstorage-file` 警告：Node 25 的全局持久化存储 getter 被 `src/test-setup.ts` 的存储探测触发。测试启动时改为直接注入独立内存 `localStorage`/`sessionStorage`，避免磁盘状态污染。带 `NODE_OPTIONS=--trace-warnings` 重跑全套后 75 个文件、462 项全部通过且无该警告；`tsgo -b`、修改文件 oxlint 与 oxfmt 检查通过。
- 2026-10-31 当前工作树再次运行前端全套：Vitest 75 个文件、462 项通过，`tsgo -b` 和 Rsbuild 生产构建通过；受影响 4 个组件/路由测试共 20 项复测通过。本次涉及的 14 个应用文件及格式检查器共 15 个文件 oxlint 与 oxfmt 检查通过。全仓 oxlint 仍因其它页面/脚本问题退出 1；全仓格式检查列出 33 个其它未格式化文件，未批量改写无关文件。修复 Windows 格式检查器对 `oxfmt` 的启动方式，现可正确调用本地 `oxfmt.cmd` 并报告真实格式差异，而非静默因命令解析失败退出。
- 2026-10-31 本地 Edge Headless 加载 `/d2s-admin-console`，确认后台登录页呈现一个 `main` 地标、邮箱/密码输入均为必填且在可访问性树中以关联标签命名。前端 API 代理目标 `localhost:3000` 当前无服务，日志显示 `ECONNREFUSED`，因此本轮未形成真实后台登录或操作的浏览器端到端证据；独立屏幕阅读器验收仍待完成。
- 2026-11-02 修复受控审计清理保留天数上界未校验导致的秒数乘法溢出风险：服务端限制在 180 至 2,147,483,647 天，超界请求在任何数据库访问前拒绝；管理页同步设置整数步进和最大值，并禁用超界提交。模型回归确认低于下限和高于上限均拒绝且未删除记录；固定路径 `model` 全包测试、`go vet -p 1 ./model` 通过，后台页面测试 10/10、`tsgo -b`、Rsbuild 生产构建以及目标文件 oxlint/oxfmt 检查通过。

- 2026-11-02 修复受控审计清理失败留痕：清理事务回滚后用新事务写入带 `FAILURE`、失败阶段、时间范围及统计数量的清理审计；新增触发器故障注入回归，确认审计删除回滚且失败审计保留。新增 `result` 字段须随独立审计迁移升级，启动时检查该字段避免旧 schema 执行清理。定向模型测试、全量 `go test -p 1 ./... -count=1`、`go vet -p 1 ./...` 和 `go build -p 1 ./...` 通过；生产清理仍待迁移与数据库权限验收。


- 2026-11-02 确认受控清理代码将用户行为日志、注册快照和管理员操作审计的硬下限设为 366 天（超过一年），365 天及以下请求被拒绝；UTC 用户行为日志截止时间仍执行较长保护逻辑。对应回归包含 366 天边界、低于下限拒绝、清理审计与回滚验证。
- 2026-11-03 根据负责人补充的运营事实复核真实身份核验风险。邮箱验证码仅证明邮箱控制权，不等于实名；本站没有公开发布、评论、群组或站内即时通讯功能，但实际提供在线下单、付款和自有软件交付，《反电信网络诈骗法》第二十一条第四项单列的“网络交易”实名要求构成实质风险，不能因商品为自有软件或安装包托管在外部平台就推断购买账号当然豁免。第二十三条应用程序封装/分发义务是否涵盖外链导流仍须结合链接控制和实际交付流程判断；《互联网用户账号信息管理规定》第二十三条将应用软件下载列入互联网服务定义，而第九条具体实名义务针对信息发布/即时通讯等服务，两者适用边界需分别判断。现行《网络安全法》第五十条第二款对应用软件下载服务提供者另有安全管理义务。2026 年《互联网信息服务管理办法》修订草案不作为现行义务依据。运营主体已说明注册地为烟台、工商经营范围含“软件开发”、ICP备案类别未知、仅销售自有软件且安装包由外部托管；这些事实尚未独立核验。取得主管部门或合格法律顾问基于真实主体、备案及交易/交付流程出具的书面意见前，保持证件采集入口关闭，不对外宣称已实名或确定豁免，并不得提交自评估。
- 2026-11-02 补齐站内工单闭环：后台填写的 `admin_reply` 现通过当前登录用户自己的工单列表返回并展示；用户接口仍排除联系邮箱、来源 IP、User-Agent 和后台经办人。数据库投影、审计事务、跨数据库矩阵和前端页面均加入回复可见性断言。当前本地 `go test ./...`、`go vet ./...`、`go build ./...` 均通过；随后在隔离 MySQL 8.4.13 与 PostgreSQL 17.11 实例上运行 `TestD2SSchemaConfiguredDatabases` 和 `TestD2SConcurrentCriticalPaths`，两库均通过，测试后已停止实例。前端全套 76 文件/471 项通过，`tsgo -b` 与 Rsbuild 生产构建通过；生产部署、生产浏览器验收、屏幕阅读器和支付沙箱仍未通过。

- 2026-11-02 扩展工单入口前端回归：验证右侧浮动菜单展开/关闭、站内 `/support` 链接、企业微信 URL 与 `target=_blank`/`noopener noreferrer` 属性，以及独立后台中隐藏入口。目标文件 5 项测试通过，完整 Vitest 76 个文件、473 项通过，`tsgo -b` 和目标文件 `oxfmt --check` 通过；未替代真实浏览器/屏幕阅读器验收。

### S5：部署与生产演练

状态：`in_progress`。

- 2026-11-03 以隐私政策内部草稿中的真实方括号占位内容检查 readiness 门禁，发现它只识别部分固定词组，可能放过 `[填写...]`、`[待确定]`、`[负责人核实...]` 等草稿标记。已扩展检查和 mock 回归覆盖这些形式，直接加载仓库的内部草稿并确认拒绝；规则按具体待填标记匹配，`[待付款]`状态文字和正常 Markdown 法规链接可通过。`scripts/d2s-production-readiness-selftest.ps1`、PowerShell AST 检查通过。此门禁仅拦截明显占位符，不判断政策事实准确性或法律充分性，生产政策仍需负责人核实并审核。
- 2026-11-03 重新运行 `scripts/d2s-production-readiness-selftest.ps1`，有效模拟环境通过，空政策、仓库内部草稿、错误客服主机/路径/端口/查询/凭据/片段、缺失客服接口、错误运行 SHA 和无效 P-256 公钥等拒绝用例全部通过。随后对生产执行无凭据只读 GET：`/api/status` 为 200、运行 SHA `d11d0cc1da5070bc65b57ff72181ed2865043833`；`/api/privacy-policy` 为 200 但内容为空，`privacy_policy_enabled=false`。mock 自测不证明生产就绪；生产政策门禁仍失败。
- 2026-11-03 在已授权的只读生产核验中再次 GET：`/api/status` 为 200，运行 SHA 仍为 `d11d0cc1da5070bc65b57ff72181ed2865043833`；`/api/privacy-policy` 返回 `{"success":true,"message":"","data":""}`，政策内容为空；`/api/v1/support/config` 返回 200。Draft PR #1 的两个 GitHub Actions 工作流（`41559259029`、`41559258003`）最终均通过，涵盖通用前后端 CI、根模块/relaykit vet/build/test、D2S 前后端、MySQL/PostgreSQL 矩阵及部署检查。PR 保持 Draft，本轮没有部署生产或提交自评估。

- 2026-11-02 无凭据只读 GET 复核：`/api/status` 返回 200，运行版本为 `26f524d11424a2866a8608a4883a63dd1f7fe16d`；新增投诉配置 API `/api/v1/support/config` 返回 404；`/support` 返回 200 HTML，但属于 SPA 回退响应，不能证明工单页面/API 已部署。由此确认本地投诉与企业微信入口代码尚未上线。本次未执行生产写操作。

- 2026-11-02 当前工作树全仓交叉构建 `GOOS=linux GOARCH=amd64 go build -p 1 ./...` 与 `GOOS=darwin GOARCH=arm64 go build -p 1 ./...` 均通过。Docker 更新器增加运行版本 SHA 门禁：Docker healthy 后要求 `/api/status` 的成功响应 `data.version` 与目标 SHA 完全相同；状态读取失败、JSON 无效或 SHA 不符会回滚并记录失败，且已有 `.update-complete` 标记必须与当前运行 SHA 复核。Git Bash 隔离回归覆盖精确 SHA 成功、匹配标记快速返回、旧标记错配、运行版本错配回滚及状态接口不可用回滚，全部通过；Shell 语法检查通过。因当前 Windows 环境没有可用 Python 3，隔离回归用 Node 替身模拟更新器现有 Python JSON I/O，尚需在生产等价 Linux/Python 环境复跑；未部署生产。

- 2026-10-31 经 PowerShell 无凭据只读 GET 复核：`https://d2s.site/api/status`、`https://d2s.site/api/v1/license/keys`、`https://d2s.site/device` 均返回 200，公开公钥 `kid` 为 `d2s-es256-2026-09`；旧域名 `https://100393.com/api/status` 返回 404。该检查只确认公网路由响应，不验证容器内部健康、完整设备码闭环或支付回调。

- 2026-10-31 补充复核：同一授权范围内的无凭据 GET 再次确认 status、公钥、设备页为 200，旧域名 API 为 404；当前工作树 `go vet -p 1 ./...` 与 `go build -p 1 ./...` 均以退出码 0 完成。公网检查不验证生产内部健康，构建与静态分析不替代部署演练。
- 2026-11-02 通过无凭据只读 GET 再核验生产公开端点：`/api/status` 返回 200，版本 `26f524d11424a2866a8608a4883a63dd1f7fe16d`；`/api/v1/license/keys` 返回 200 并公布 1 个 ES256/P-256 公钥（`kid=d2s-es256-2026-09`）；`/device` 返回 200。该结果仅说明三个公网路由可响应，不验证生产容器/数据库状态、密钥轮换、支付回调或交易闭环。
- 2026-11-02 再次无凭据只读核验生产：`/api/status`、`/api/v1/license/keys`、`/device` 均返回 200，公开密钥列表有 1 个条目。状态版本 `26f524d11424a2866a8608a4883a63dd1f7fe16d` 与 `origin/main` 完全一致；本地 `HEAD=13805885f2b4e68d5ad751b7af4f6274c76a4a5e` 在生产基线之后有 5 个提交，工作区干净。此次只证明公开端点可响应及生产仍运行 origin 基线，不证明本地新提交已部署或内部服务、数据库、支付和端到端流程正常；未执行生产写操作。
- 2026-11-02 通过 SSH 对生产主机执行只读核查：`/opt/desktop2stereo-site` Git HEAD 为 `b9c068bebdea1670a0242e67b449fc9e2621d3f0`，工作区有 12 个已跟踪改动和 131 个未跟踪文件；运行容器 `new-api` 健康且镜像 revision 为 `26f524d11424a2866a8608a4883a63dd1f7fe16d`，PostgreSQL 与 Redis 容器运行。`desktop2stereo-reconciliation.timer` 为 disabled，`/etc/desktop2stereo/reconciliation.env` 不存在；只读查询 PostgreSQL `information_schema.columns` 未找到用户行为日志、注册记录、管理员审计和留存审计四张表。当前本地提交 `50899e5` 尚未部署。不得从该脏工作区直接发布；生产迁移、权限配置和定时器仍未执行。
- 2026-10-31 交叉目标编译：当前工作树 `GOOS=linux GOARCH=amd64 go build -p 1 ./...` 与 `GOOS=darwin GOARCH=arm64 go build -p 1 ./...` 均退出码 0；仅证明对应平台编译通过，未在 Linux/macOS 主机运行或验收。
- 2026-11-02 在当前工作树复跑 `GOOS=linux GOARCH=amd64 go build -p 1 ./...` 与 `GOOS=darwin GOARCH=arm64 go build -p 1 ./...`，均退出码 0；这仅验证交叉编译，不替代 Linux/macOS 实机运行、签名或平台端到端验收。
- 2026-10-31 处理 360 报毒反馈：核对发布工作流后确认 `controller.test.exe` 不在任何用户发布清单中；服务端和 Electron Windows 发布目前都没有 Authenticode 签名步骤。新增 [Windows 误报与签名处理说明](windows-360-false-positive-and-signing.md)，记录 360 官方样本申诉步骤、同 SHA-256 复核和正式签名要求。具体误报文件、检测名、样本哈希及正式签名证书/托管签名服务尚未提供，因此正式发布签名与该具体样本的厂商复核仍待完成。
- 2026-11-02 再核对 Windows 发布：`release.yml` 仅上传 `new-api-*.exe`，Electron 工作流仅从 `electron/dist/*.exe` 生成发布资产，Go 测试程序不会进入 Release。当前仓库级 `gh secret list` 未列出代码签名 Secrets，本机也没有带私钥的 Authenticode 代码签名证书；无法安全启用正式签名步骤。需要明确被拦截的是测试临时文件还是实际发布 EXE，并提供该文件的 SHA-256、360 检测名和截图；正式发布签名须先配置受控代码签名服务或 HSM。
- 2026-10-31 修正订单及付费撤销/离线续期路由的中间件顺序，使未认证购买请求先写入 `PURCHASE/FAILURE` 行为日志，再返回认证错误；路由回归覆盖订单预览、创建、Checkout、付费解绑和离线续期五个入口。定向回归与完整 `go test -p 1 ./... -count=1` 均通过；`go build -p 1 ./router`、`go vet -p 1 ./router` 和 `git diff --check` 通过。
- 按 [`d2s.site.md`](d2s.site.md) 部署腾讯云、Cloudflare、PostgreSQL、Redis 和回调 Worker。
- 完成双实例压力、密钥轮换、备份恢复、灰度、回滚、监控和告警演练。
- 与 Windows/Linux/macOS 客户端执行生产等价端到端验收。
- 已建立 [`d2s-production-drill.md`](d2s-production-drill.md) 和无破坏性的生产就绪检查脚本；runbook §5 已列出 Windows/Linux/macOS 的账号登录、设备码、绑定、模式、租约、离线周期和证据要求，真实基础设施、渠道沙箱和三平台端到端证据仍待执行。
- 2026-11-02 生产就绪脚本新增 `-ExpectedVersion`，校验 `/api/status` 的 `data.version` 与指定的 40 位发布 Git SHA 精确一致，并拒绝格式错误 SHA、版本不匹配、无 HTTP 检查时指定版本。PowerShell AST 和本地 HTTP mock 的匹配/不匹配/非法 SHA/`-SkipHttp` 四种回归通过；runbook 已要求部署验收传入目标 SHA。随后使用已授权的无凭据只读 GET 对真实生产 API 验证：当前运行版本 `26f524d11424a2866a8608a4883a63dd1f7fe16d` 匹配通过，错误 SHA 被拒绝；此结果仅验证公开 API 契约，不代表新提交已部署。
- 2026-10-30 加强生产就绪检查：`-RequireSecrets` 现在实际解析配置私钥并校验 P-256 曲线；启用 HTTP 检查并传入 `-RequireSecrets` 时逐字比较 `/api/v1/license/keys` 中配置 `kid` 的 `x/y` 坐标，防止同 ID 错配密钥。隔离回归覆盖 PKCS#8 DER、Base64 PEM、SEC1 DER、损坏密钥、非 P-256/仅公钥密钥及公钥不匹配；本地 mock HTTP 验证通过，CI 冒烟步骤已同步生成隔离 P-256 密钥并覆盖错配、错误曲线、仅公钥材料及损坏密钥拒绝。此结果不证明生产 Secret 当前有效，也未替代生产密钥轮换演练。
- 已部署固定路径的日终对账脚本和 systemd 单元；生产服务器 timer 当前为 disabled，root-only 对账环境文件不存在，令牌与定时器演练仍待完成。
- 2026-10-30 修复对账认证契约：原 runner 使用 Bearer 令牌，但生产接口只接受 12 小时后台会话。代码现为对账接口单独支持 32 字节随机十六进制令牌（恒定时间比较），仍保留后台会话访问；其它后台 API 不接受该机器令牌。每次成功或失败的对账请求写入管理员审计，审计范围包含 UTC 时间窗和结果/差异数量。runner、Compose 环境变量、PowerShell 手动 runner 与部署说明已统一为 `D2S_RECONCILIATION_TOKEN`。middleware/controller 定向测试通过。此修复尚未部署生产，因此生产 timer 保持禁用。只读核验发现生产部署仓库 HEAD 与本地基线不同，且含有既有未提交及未跟踪内容；为避免覆盖或混入生产变更，本轮未部署代码、未写入生产令牌。待通过受控发布交付代码并核对审计表权限后，再配置令牌、验证端点并启用 timer。
- 2026-10-30 修复对账审计存储失败时重复写 HTTP 响应的问题；新增 controller 回归验证响应仍为单一有效 JSON，`go test ./controller -run '^TestD2SAdminReconciliation' -count=1` 通过。
- 2026-11-02 修复日终对账漏查旧订单及并发读取不一致：对账现在按 `status=pending`、有效到期时间早于报告截止时间扫描全体未结订单，并持续列为 `pending_expired`，直到订单进入终态；订单与支付事件从同一只读事务快照读取，MySQL/PostgreSQL 显式请求 `REPEATABLE READ`。定向对账测试、`go vet -p 1 ./...` 和 `go build -p 1 ./...` 通过；全量 Go 测试被 `relay/channel` 两项 Windows HTTP/2 socket reset 用例间歇性阻断。MySQL/PostgreSQL DSN 未配置，数据库矩阵本地跳过。该报告仅提示核查，不会自动更改订单状态。
- 2026-10-30 加固日终对账报告落盘：Linux runner 设置 `umask 077`，systemd service 同样设置 `UMask=0077`，避免报告、响应暂存和临时文件被其他本机账号读取；PowerShell runner 将相对路径解析到 PowerShell 当前目录，并以 `FileMode.CreateNew` 排除并发覆盖，UTF-8 输出无 BOM。PowerShell AST 解析、Git Bash `bash -n`、本地模拟 API 导出验证（相对路径、有效 JSON、无 BOM、重复文件拒绝覆盖且内容不变）与 `git diff --check` 均通过；尚未在 Linux 生产主机验证 systemd unit、定时器或实际 ACL。
- 2026-09-09 已通过固定 `/opt/desktop2stereo-site/update-requests/request` 更新流程将生产 Docker 实例更新到 `e57086b041fb9158cf42e0a71c68a5d66018ce3e`；状态文件为 `succeeded`，`/api/status` 返回同一版本且容器健康。该记录只证明一次更新链路成功，不替代完整灰度、回滚和告警演练。
- 2026-09-09 已将 `d55293350ec204d35ce268764481a81d900e7d2a` 部署到生产；状态为 `succeeded`，应用容器 healthy，且宿主机更新器和数据库备份脚本均已通过 SHA 与该发布目录文件逐字一致校验。
- 2026-11-02 身份核验号码从无密钥 SHA-256 改为域分离 HMAC-SHA-256；新增 `id_document_number_hash_version` 字段，读取到旧版 VERIFIED 摘要时，在同一事务审计后降为 PENDING 并清除旧摘要；密钥缺失/格式异常时返回 503 且不写入记录。生产就绪脚本校验 `D2S_ID_DOCUMENT_HMAC_KEY` 为 64 位十六进制；更新规格书、部署说明和 SEC-001 V1.3。`go test -p 1 ./controller -run '^TestD2SAdminIdentity' -count=1`、`go test -p 1 ./model -count=1`、`go vet -p 1 ./controller ./model`、`go build -p 1 ./controller ./model` 均通过；PowerShell AST 解析及密钥格式正反例通过。生产 Secret 注入与运行配置尚未验证。SEC-001 后续版本已更新订单邮箱快照/重试规则和 180 天内部留存基线的个人信息保存期限审查说明。
- 2026-09-09 已使用该版本更新前生成的 PostgreSQL 备份，在固定恢复容器中完成隔离恢复验证，恢复出 17 张 D2S 表；关键表行数和 19 项关键关联完整性检查均通过（孤儿记录为 0），恢复容器已由脚本清理。该证据不替代 COS 跨故障域复制和业务语义完整性核对。
- 2026-09-09 已将带 COS 上传能力的备份脚本同步到固定宿主机路径 `/usr/local/sbin/desktop2stereo-db-backup`，并现场生成带 SHA 校验的备份；服务器尚未配置 COSCLI 与 COS 上传 URI，因此 COS 跨故障域复制仍未宣称完成。
- 2026-09-09 已将官方 COSCLI v1.0.9（Linux amd64，发布 SHA256 已核对）安装到固定 `/usr/local/bin/coscli`；COS 桶 URI 和最小权限认证仍待配置，配置前不执行真实上传。
- 生产就绪脚本现在校验 `SESSION_COOKIE_TRUSTED_URL` 与 `D2S_DEVICE_VERIFICATION_URI` 必须使用 BaseUrl 主机或其子域名、匹配端口，并拒绝含凭据的 URL；这项静态门禁不能替代生产环境变量核验。生产就绪检查已在隔离临时配置下实际运行通过；2026-09-09 已对真实生产地址执行 HTTPS `/api/status` 检查并返回 200，生产容器保持 healthy。2026-10-30 从公网读取 `/api/status` 返回 200，接口报告版本 `26f524d11424a2866a8608a4883a63dd1f7fe16d`；本次复核 `https://d2s.site/api/status` 返回 200，`https://d2s.site/api/v1/license/keys` 返回 200 并公布 1 个 ES256/P-256 公钥（kid `d2s-es256-2026-09`），而 `https://100393.com/api/status` 返回 404。上述公网请求只证明 API 当前可响应，不证明数据库、支付回调、容器内部健康或灰度/回滚状态。2026-10-30 经 SSH 将生产 `.env` 的 `SESSION_COOKIE_TRUSTED_URL` 更新为 `https://d2s.site`，将 `D2S_DEVICE_VERIFICATION_URI` 更新为 `https://d2s.site/device`；原子替换只改这两项并保留 `600 ubuntu:ubuntu` 权限，Compose 配置校验通过，仅重建 `new-api`，容器恢复 healthy。重启后公网 `/api/status`、`/api/v1/license/keys` 和 `/device` 均返回 200，公钥列表含 `d2s-es256-2026-09`；旧域名 `https://100393.com/api/status` 仍返回 404。该验证不等同于实测浏览器登录 Cookie 或完整设备码确认闭环。真实 Secret 轮换、COS 跨故障域备份、灰度/回滚、监控告警和渠道沙箱证据仍待执行。

只有 S1-S5 全部达到 `verified` 才允许正式上线。

## 10. 当前待决策与风险

- 2026-11-03 为取得真实身份核验适用范围的书面意见，新增[事实核对与书面意见请求清单](SEC-002-真实身份核验适用范围书面意见请求.md)。清单区分负责人提供的业务事实、待核验材料，以及《反电信网络诈骗法》第二十一条/第二十三条和账号管理规定第九条/第二十三条涉及的具体问题；它不构成法律结论，不改变自评估和证件采集门槛。
- 2026-11-03 再次无凭据只读复核生产：`/api/status` 返回 200，运行 SHA 仍为 `d11d0cc1da5070bc65b57ff72181ed2865043833`；`/api/v1/support/config` 返回 200，已配置 `work.weixin.qq.com` 客服入口；`/api/privacy-policy` 返回 200，但 `data` 为空。结合本次发布记录中的审计角色/表权限验收，修正本计划和 SEC-001 当前状态表，删除已过期的“生产旧版本、客服 API 404、角色未配置”现状描述。未访问生产数据库或执行生产写操作；双人审批、支付沙箱、COS、生产工单闭环等仍未验收。
- 2026-11-03 对生产 `/support` 执行 Playwright 访客态只读验收：HTTP 导航后落到 `/sign-in?redirect=%2Fsupport`；390px 视口点击“投诉与咨询”后显示登录要求、站内问题链接和配置好的企业微信链接，无水平溢出或页面脚本错误。未输入凭据、提交工单或执行其它生产写操作。此证据不替代登录用户工单提交/隔离、管理员处理流程及真实屏幕阅读器验收。
- 2026-11-03 真实身份核验适用性复核发现此前的初步判断遗漏现行《反电信网络诈骗法》第二十一条第（四）项关于信息/软件发布服务及网络交易服务的列举。负责人报告运营主体所在地为山东烟台、只销售自有 Desktop2Stereo 软件、无第三方或用户上架；“软件开发”是公司工商经营范围，ICP 备案服务类别不清楚；网站不托管安装包，只跳转到 GitHub、夸克网盘或百度网盘等外部下载地址。上述事实尚未独立核验，尤其备案材料和线上配置待核。外部托管下载和自营模式均不能单独排除本条对在线交易的适用性，须由主管部门或合格法律顾问结合运营主体及具体交易/分发模式书面确认。无 UGC 或仅邮箱验证不足以据此断言豁免；在书面确认前不擅自收集证件，也不提交自评估或声称无需实名。SEC-001 与后台规格已更新官方依据、负责人提供的事实和未决门槛。
- 2026-11-02 工单后台列表遇到数据库查询失败时原先返回 400；现调整为 500，并追加 `VIEW_SUPPORT_TICKETS/FAILURE` 审计。新增故障回归验证响应码和审计落库；工单定向测试、全量 `go test -p 1 ./... -count=1`、`go vet -p 1 ./...`、`go build -p 1 ./...` 均通过。该修复仅在本地工作树，生产 API 仍返回 404，未部署。生产就绪脚本同步增强：HTTP 检查现要求 `/api/v1/support/config` 返回 HTTPS `work.weixin.qq.com/ca/` 联系链接；新增 `scripts/d2s-production-readiness-selftest.ps1`，无网络回归验证合规链接通过、其他主机和缺失端点被拒绝。
- 2026-11-02 收紧客服链接配置 API：仅返回 HTTPS、精确主机 `work.weixin.qq.com`、无端口/凭据/查询/片段且路径位于 `/ca/` 的链接；默认企业微信链接通过，HTTP、伪装主机、错误路径及 URL 附加内容共 8 种无效配置均被拒绝。客服配置定向控制器测试、全量 `go test -p 1 ./... -count=1`、`go vet -p 1 ./...`、`go build -p 1 ./...` 及 `git diff --check` 通过。该修复仍仅在本地工作树，生产 API 未部署；浏览器端到端验收尚未完成。
- 2026-11-02 生产 readiness 新增公开隐私政策配置门禁：要求 `/api/privacy-policy` 返回非空内容；无网络自测覆盖有效政策通过及空政策拒绝，并保留客服 URL 错误主机和客服 API 缺失拒绝用例；PowerShell AST 解析、自测全套和 `git diff --check` 通过。此门禁仅检查内容存在，不判断法律充分性；生产政策需运营负责人审核工单数据类别、用途、访问和留存说明。
- 2026-11-02 收紧生产 readiness 的企业微信 URL 检查，与服务端限制保持一致；新增错误路径、任意显式端口（含 `:443`）、查询参数、用户信息和片段拒绝回归。更新后的 readiness 自测全套通过，runbook 已同步列出覆盖项；本地 mock 不替代生产公开端点核验。
- 2026-11-02 复跑本地工单验收：`go test -p 1 ./router -run D2SRoutesExposeAuthenticatedSupportAndNoIdentityEntryAPI -count=1`、`go test -p 1 ./controller ./model -run D2SSupport -count=1` 均通过；支持页面 Vitest 5/5 通过。覆盖匿名请求拒绝、登录账户工单隔离和用户数据裁剪、工单审计事务、表单校验、企业微信浮动入口和用户工单查看。该结果不替代真实浏览器、生产 API 和支付链路验收。
- 2026-11-03 补齐 CSV 导出制度中的事前审批缺口：后端新增双人审批模型和接口，审批绑定精确导出类型/筛选范围、30 分钟过期并单次使用；申请人不能自批，审批申请、决定、消费和导出审计可按审批 ID 关联。后台界面支持申请、查询状态、另一超级管理员批准/拒绝及获批下载；新增经现有超级管理员凭据验证的离线管理员开通命令，以便受控配置第二名审批人。自测通过：`go test -p 1 ./... -count=1`、审批/导出及管理员开通定向 Go 回归、`go vet -p 1 ./...`、`go build -p 1 ./...`、前端 Vitest 76 文件/476 项、`tsgo -b`、Rsbuild 生产构建、格式检查和 `git diff --check`。全量 Go 测试期间后续增加的空待审批列表与自批拒绝用例亦已单独通过。MySQL/PostgreSQL DSN 未配置，未在本轮复测；生产账号创建、数据库授权、生产验收和自评估均未完成，不得标记生产已落实。
- 2026-11-03 无凭据、只读生产验收：`/api/status` 返回 200，运行 SHA 为 `26f524d11424a2866a8608a4883a63dd1f7fe16d`；`/api/v1/license/keys` 返回有效 ES256 公钥。`/api/v1/support/config` 返回 404；`/api/privacy-policy` 返回 200 但内容为空；`/support` 返回 New API 通用 SPA HTML 回退，不证明支持页部署。生产 readiness 脚本以已运行 SHA 验证时在客服配置 API 处失败。未执行生产写操作。支持工单、双人导出审批与自评估功能仍未部署，须先通过受控发布、配置真实隐私政策、至少两名超级管理员和生产数据库权限，再重跑生产验收；禁止从生产脏工作树发布。
- 2026-11-03 经授权提交并推送 `2e4d1fa` 后，CI 首轮暴露 readiness secret smoke test 未模拟客服配置和隐私政策响应；修正 CI mock 并推送 `6d223bc35efb199aeef0d06ba1ad6cbe8ce35461`，完整 CI 全绿（Go 全量测试/静态检查/构建、MySQL/PostgreSQL 迁移矩阵、前端类型/测试/构建和 secret smoke test）。随后生产首次更新因四张审计表缺失而自动回滚。只读检查确认应用此前使用 PostgreSQL 超级用户且未配置独立迁移/清理 DSN；更新启动期间 GORM 已将工单及导出审批等普通业务表迁入生产 schema。负责人授权后，新增 `d2s_app`、`d2s_migrator`、`d2s_audit_cleanup` 三个独立非超级用户角色，分别设置随机密码并将 DSN 写入权限为 0600 的生产 `.env`；更新前应用数据库角色和权限测试通过 CI。首次门禁更新被服务器旧版固定更新器绕过，随后因发布目录已有残留而将源码嵌套，应用未启动。已通过容器内 PostgreSQL 工具验证备份 checksum 和归档目录；将 CI 验证的新更新器安装到宿主机，执行角色/表归属准备、四张审计表迁移和权限校验，应用恢复健康。再修复源码目录安装使用 `mv -T` 并核验 Compose 文件位于发布根目录，提交 `d11d0cc1da5070bc65b57ff72181ed2865043833` 的全量 CI 通过，固定更新流程最终成功；`/api/status` 返回完全相同 SHA，容器 healthy，发布标记存在，宿主机固定更新器与发布版本一致。生产核验显示四张审计表归属 `d2s_migrator`，应用角色对审计表有 SELECT/INSERT 而无 UPDATE/DELETE，清理角色对三张留存表有 SELECT/DELETE、对清理审计表有 SELECT/INSERT。`/api/v1/support/config` 返回 200 且企业微信入口已配置。数据库备份保存在服务器本地并通过 SHA-256 和容器内 `pg_restore` 列表检查；远端 COS 备份未配置。`/api/privacy-policy` 虽返回 200，但内容为空，因此生产 readiness 未通过，不得提交自评估；还需完成获批隐私政策、至少两名超级管理员审批闭环及真实支付沙箱验收。

- 2026-11-03 为生产 readiness 新增 `-HttpOnly` 只读核验模式，要求明确提供预期运行 SHA 与签名 key ID，跳过本机数据库和 Secret 检查，同时保留公开 HTTPS、版本、公钥、客服链接和非空隐私政策校验。无网络自测覆盖无本地配置成功、SHA 不匹配及参数冲突拒绝；PowerShell AST、全部 readiness selftest 与 `git diff --check` 通过。使用该模式对生产执行 GET：运行 SHA 与 `d11d0cc1da5070bc65b57ff72181ed2865043833` 一致、公钥 ID `d2s-es256-2026-09` 存在、企业微信链接通过；隐私政策为空，脚本按设计拒绝 readiness。未登录、未执行写接口。该模式只验证公开端点，不证明本机 Secret、私钥匹配、数据库角色或交易闭环。
- 2026-11-03 加强 HTTP-only readiness 的签名公钥检查：现要求预期 key ID 唯一匹配，并将 Base64URL x/y 导入 P-256 曲线验证公钥点；错误长度或无效曲线坐标被拒绝。无网络自测、PowerShell AST 和 `git diff --check` 通过；生产只读复跑成功验证实际公钥点，后续仍按空隐私政策拒绝 readiness。
- 2026-11-03 加强生产隐私政策 readiness 门禁：除非非空外，拒绝常见未完成占位文本（例如“待填写”、`TODO` 和 `{{...}}`）；自测覆盖中文和英文占位项，PowerShell AST 及 `scripts/d2s-production-readiness-selftest.ps1` 全部通过。该检测仅排除明显草稿标记，不验证政策事实准确性或法律充分性；生产政策仍为空，readiness 继续失败。
- 2026-11-03 审查暂存改动时发现 Waffo 掩码卡付款标识调用了不接受 `payment_method` 的 HMAC helper，修正允许的标识类型并补足购买查询审计成功用例缺失的审计表测试夹具。`scripts/d2s-fixed-go-test.ps1 -Package ./controller` 固定路径全套 controller 测试通过，包含 Waffo Webhook 掩码标识及审计过滤器回归；360 进程当时仍运行，但测试二进制未被隔离。MySQL/PostgreSQL 数据库矩阵和其余包测试仍未在本轮重跑。
- 首发 +30 天离线延长包价格已确定：CN ¥19.90、INTL $2.99；生产环境变量仍需部署并验证。
- PayPal、Paddle 没有当前基线的原生适配器，已移出首发范围；后续如实现官方适配器再单独评估接入。
- 统一支付桥不是渠道官方 Webhook 验签的替代品；各开放渠道适配器已有统一入口回归，真实第三方沙箱异常矩阵和日终对账仍待完成。
- 2026-10-31 检查当前本地进程环境中的 Stripe、Creem、易支付、Waffo、Waffo Pancake 与 Payment FM 沙箱凭据变量，均未配置；本机不能执行真实渠道沙箱矩阵。取得各开放渠道的隔离沙箱凭据后再进行真实发起、重试、退款和结算文件对账，不使用生产支付密钥代替沙箱验证。
- 用户和管理员网页已有第一版，管理员权限及组件回归已覆盖；独立后台现在按角色打开首个授权模块，未知角色不展示受限模块，并有账户操作员权限边界回归。完整可访问性、真实屏幕阅读器和端到端支付流程仍待完成。
- 如需保留旧 `desktop2stereo-server` 数据，必须另做迁移、校验和对账，不能直接复制。
- 对外部署 AGPL-3.0 修改版服务时必须持续满足对应源代码提供义务。

## 11. 文档同步规则

- 服务端代码、API、迁移、商业规则和本计划只在 `desktop2stereo-site` 维护。
- 部署、域名、Cloudflare、腾讯云、备份和运维只在 [`d2s.site.md`](d2s.site.md) 维护。
- 客户端契约、公钥清单、启动器、Runtime 门禁和三平台发布只在客户端计划维护。
- 变更 API 时同步更新 `desktop2stereo-api.md`、客户端 requirements matrix 和两边变更日志。

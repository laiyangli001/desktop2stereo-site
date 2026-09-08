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
- 设备码登录、ES256 离线凭证、15 分钟在线租约和稳定错误码。
- 免费撤销、每月三次阶梯付费撤销、终身一次人工解绑和离线延长包。
- CN/INTL 首次外部付款区域锁、服务器报价、幂等订单和支付回调。
- CNY/USD 分币种余额、邀请首购奖励、拒付冲正和 CN 支付宝提现审核。

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

数据库必须强制：每用户唯一试用、每设备摘要唯一当前绑定、每授权唯一活动租约、每用户内
幂等键唯一、每授权最多一个待处理付费撤销订单、支付渠道事件联合唯一、每受邀账号最多一次
首购奖励。余额预留、支付完成、区域锁、授权签发和奖励必须在同一数据库事务内完成，并对
竞争行加锁。

所有金额使用整数最小货币单位，时间使用 UTC Unix 秒。只保存设备摘要、令牌哈希和 JWS
摘要，不保存原始设备标识、刷新令牌明文、支付 Secret 或签名私钥。

## 5. 授权状态机

- 授权状态：`active`、`suspended`、`revoked`。
- 授权模式：`unbound`、`online`、`offline`、`permanent`。
- 已验证邮箱账号首次访问时原子创建 30 天试用；重试和并发请求不能重复创建。
- 离线档位为 7/14/30 天，对应免费撤销冷却；模式切换不能清除冷却。
- +30 天延长包叠加当前离线截止时间；价格由 CN/INTL 环境变量配置。
- 在线租约有效 15 分钟，建议客户端每 5 分钟续期；同授权第二个运行实例被拒绝。
- 用户主动退出释放该账号的在线租约；过期租约允许被下一次事务覆盖并定期清理。
- 永久绑定需明确确认，之后禁止普通降级和撤销；人工解绑每账号终身最多批准一次。
- 付费撤销每授权每自然月最多三次，CN 为 1990/3490/6990 分，INTL 为
  299/499/999 美分。

## 6. 商业规则

- 首份正式授权：CN 9900 分，INTL 2990 美分。
- 支付宝、微信、易支付、支付 FM、Waffo 属 CN；Stripe、Creem、Waffo Pancake 属 INTL。
  PayPal/Paddle 在完成官方适配器和沙箱验收前不属于首发渠道，也不得出现在首发页面。
- 第一次成功的外部支付原子锁定区域；余额全额支付不能建立区域来源。
- 订单预览和创建均由服务器重新报价；余额先预留，成功后消耗，失败/取消后释放。
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

状态：`implemented`，SQLite 测试已通过。

- 导入 new-api、保留上游和 AGPL-3.0 信息。
- 增加 D2S 模型、迁移、路由、授权状态机、设备码、ES256 和在线租约。
- 增加订单、区域、余额、邀请、拒付、提现和管理员 API。

退出条件：D2S 单元测试、完整 Go 测试、`go vet`、根模块构建和敏感信息扫描通过。

### S2：三数据库与并发

状态：`verified`。

- SQLite、MySQL、PostgreSQL 新建库连续执行两次迁移：`verified`。
- 已在 SQLite、MySQL、PostgreSQL 验证并发试用、绑定、租约、订单、区域锁、余额和支付幂等。
- CI 中保留真实 MySQL/PostgreSQL 服务测试。

### S3：支付渠道

状态：`in_progress`。

- 把 Stripe、Creem、易支付、Waffo 的官方验签结果接入 D2S 规范事件桥。
- PayPal/Paddle 在实现适配器前从首发页面和文档中移除。
- 已覆盖开放渠道的统一订单事务、幂等、金额/币种校验，以及 Stripe、Creem、易支付（含 `TRADE_REFUND`）、Waffo、Waffo Pancake 的冲正回归；真实渠道沙箱矩阵和日终对账仍待执行。
- 对所有开放渠道执行完整沙箱异常矩阵和日终对账。

### S4：用户与管理员网页

状态：`in_progress`。

- 用户页：授权、设备、模式、订单、邀请、余额和提现。
- 管理页：授权、解绑、提现、拒付、负余额、区域和签名键。
- 当前已提供用户工作台、管理员工作台和上一 UTC 日内部对账摘要；管理员审核写操作继续复用管理员审计中间件。
- 管理员路由、侧栏入口和后端管理 API 已覆盖角色权限回归；沿用 new-api React/Semi UI/i18next，完整可访问性、组件和端到端支付页面测试仍待完成。

### S5：部署与生产演练

状态：`in_progress`。

- 按 [`d2s.site.md`](d2s.site.md) 部署腾讯云、Cloudflare、PostgreSQL、Redis 和回调 Worker。
- 完成双实例压力、密钥轮换、备份恢复、灰度、回滚、监控和告警演练。
- 与 Windows/Linux/macOS 客户端执行生产等价端到端验收。
- 已建立 [`d2s-production-drill.md`](d2s-production-drill.md) 和无破坏性的生产就绪检查脚本；真实基础设施、渠道沙箱和三平台端到端证据仍待执行。
- 已部署固定路径的日终对账脚本和 systemd 单元；服务器尚未配置 root-only 对账令牌，因此定时器和真实日终告警演练仍待执行。
- 2026-09-09 已通过固定 `/opt/desktop2stereo-site/update-requests/request` 更新流程将生产 Docker 实例更新到 `e57086b041fb9158cf42e0a71c68a5d66018ce3e`；状态文件为 `succeeded`，`/api/status` 返回同一版本且容器健康。该记录只证明一次更新链路成功，不替代完整灰度、回滚和告警演练。
- 2026-09-09 已使用该版本更新前生成的 PostgreSQL 备份，在固定恢复容器中完成隔离恢复验证，恢复出 17 张 D2S 表；关键表行数和 19 项关键关联完整性检查均通过（孤儿记录为 0），恢复容器已由脚本清理。该证据不替代 COS 跨故障域复制和业务语义完整性核对。
- 2026-09-09 已将带 COS 上传能力的备份脚本同步到固定宿主机路径 `/usr/local/sbin/desktop2stereo-db-backup`，并现场生成带 SHA 校验的备份；服务器尚未配置 COSCLI 与 COS 上传 URI，因此 COS 跨故障域复制仍未宣称完成。
- 生产就绪检查已在隔离临时配置下实际运行通过；2026-09-09 已对真实生产地址执行 HTTPS `/api/status` 检查并返回 200，生产容器保持 healthy。真实 Secret 轮换、COS 跨故障域备份、灰度/回滚、监控告警和渠道沙箱证据仍待执行。

只有 S1-S5 全部达到 `verified` 才允许正式上线。

## 10. 当前待决策与风险

- 离线 +30 天延长包的 CN/INTL 价格尚未确定。
- PayPal、Paddle 没有当前基线的原生适配器，已移出首发范围；后续如实现官方适配器再单独评估接入。
- 当前统一支付桥不是渠道官方 Webhook 验签的替代品，各渠道仍需完成适配。
- 用户和管理员网页已有第一版，管理员权限回归已覆盖；完整可访问性、组件回归和端到端支付页面仍待完成。
- 如需保留旧 `desktop2stereo-server` 数据，必须另做迁移、校验和对账，不能直接复制。
- 对外部署 AGPL-3.0 修改版服务时必须持续满足对应源代码提供义务。

## 11. 文档同步规则

- 服务端代码、API、迁移、商业规则和本计划只在 `desktop2stereo-site` 维护。
- 部署、域名、Cloudflare、腾讯云、备份和运维只在 [`d2s.site.md`](d2s.site.md) 维护。
- 客户端契约、公钥清单、启动器、Runtime 门禁和三平台发布只在客户端计划维护。
- 变更 API 时同步更新 `desktop2stereo-api.md`、客户端 requirements matrix 和两边变更日志。

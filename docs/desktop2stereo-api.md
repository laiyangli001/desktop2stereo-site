# Desktop2Stereo API v1

D2S 设备、授权、订单、账务和管理响应包含 `version`、`success` 和 `request_id`。失败响应
的 `error.code` 是客户端判断依据，客户端不得解析自然语言消息。账号兼容入口直接复用
new-api 原生响应格式。除设备码申请、兑换、公钥和支付回调外，接口需要 new-api Bearer
access token。

## 账号兼容入口

- `GET /api/captcha`：生成自托管点击验证码。响应中的 `id`、`master_image`、`thumb_image`、
  图片尺寸和 `required_clicks` 仅用于当前验证；验证码默认 5 分钟有效且只能成功消费一次。
- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/logout`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/password-reset/confirm`
- `GET /api/user/login/encryption-key`：返回 `data.enabled`；启用时同时返回 `kid` 和 RSA SPKI `public_key`，
  客户端使用 RSA-OAEP-SHA256 后将 `password_encrypted` 与 `encryption_key_id` 提交给登录接口。
- `PUT /api/user/self`：已登录用户修改密码，请求使用 `original_password` 和 `password`；成功后返回
  新的 Access Token，当前会话之外的旧会话按服务端认证版本策略失效。

登录和注册请求必须携带 `captcha_id`、`captcha_clicks`，其中 `captcha_clicks` 是包含
`x`、`y` 坐标的数组，数量必须等于 `required_clicks`；缺失、过期、重复消费、点击目标不匹配
或坐标误差超过服务端容差时返回
`BEHAVIOR_CAPTCHA_REQUIRED`，不会创建会话或账号。发送邮箱验证码和请求密码重置继续复用
`/api/verification`、`/api/reset_password`，其现有 Turnstile 防护保持不变。
邮箱验证码和密码重置码有效期最多 15 分钟，成功核验后立即失效。多实例部署必须让所有节点共享 Redis 和相同的
`SESSION_SECRET`；验证码收件标识与验证码均以 HMAC-SHA-256 摘要存储，Redis 故障时发送操作失败关闭。
未配置 Redis 时仅支持单实例内存存储，不应在多实例模式下关闭 Redis。
这些账号接口保持 new-api 契约，不承诺 D2S `version/request_id/error` 外层结构。桌面启动器
没有浏览器 Cookie 时，刷新和登出接口也接受 JSON `refresh_token`；登录成功的 `server_time` 位于
响应 `data` 中，刷新令牌仍优先通过 HttpOnly Cookie 返回。
账号被管理员禁止登录时，`POST /api/v1/auth/refresh` 返回 HTTP 423 和 `ACCOUNT_LOGIN_BLOCKED`，不会轮换刷新令牌，并清除浏览器 refresh cookie；JSON token 客户端收到同一错误。该限制阻止续期，但不撤销已有会话，强制下线由独立管理操作执行。

## 设备码

- `POST /api/v1/device/authorize`：提交 `device_hash`、`fingerprint_version`、`client_name`、
  `platform`，返回设备码、用户码、验证地址、600 秒有效期和 5 秒轮询间隔。
- `POST /api/v1/device/approve`：网页登录后提交 `user_code`。
- `POST /api/v1/device/token`：启动器提交 `device_code`；待批准返回 HTTP 202 和
  `authorization_pending`，批准后返回 access/refresh token，且只能兑换一次。若账号被管理员禁止登录，返回 HTTP 423 和
  `account_login_blocked`，并消费本次已批准设备码，不能在之后解锁时复用旧码。
- `POST /api/v1/device/cancel`：取消尚未批准的设备码。

## 投诉、建议与服务咨询

- `GET /api/v1/support/config`：公开返回企业微信客服入口链接。默认链接为 `https://work.weixin.qq.com/ca/cawcde40faa1d08998`，可用 `D2S_ENTERPRISE_WECHAT_URL` 覆盖；服务端只返回 HTTPS `work.weixin.qq.com/ca/` 链接，并拒绝非指定主机/路径、端口、URL 凭据、查询参数和片段。
- `POST /api/v1/support/tickets`：要求已注册并登录的前台账户，按部署的关键请求限流。字段为 `category`（`PAYMENT`、`ORDER_LICENSE`、`DOWNLOAD`、`SOFTWARE`、`ACCOUNT`、`OTHER`）、`contact_email`、可选 `order_id`、`subject`、`message`。成功返回 `ticket_id`；服务端从认证上下文绑定账户 ID，不接受客户端指定账户。不得提交密码、验证码或完整支付卡号。
- `GET /api/v1/support/my-tickets`：要求前台登录，只返回当前服务端账户 ID 绑定的工单及用户可见字段，包含后台回复，不包含联系邮箱、来源 IP、User-Agent 或后台经办人信息；不能通过邮箱参数查询其他账户工单。
- 后台 `GET /api/v1/admin/support/tickets` 与 `PUT /api/v1/admin/support/tickets/:id` 仅对独立 D2S 后台的 `account_operator` 及更高角色开放，支持分页查看、更新状态及登记回复。读取、修改均写入管理员操作审计；修改和成功审计在同一事务提交。

网站右侧浮动“投诉与咨询”按钮打开联系面板，包含企业微信客服链接和站内留言入口。访客可直接使用企业微信入口；选择站内留言时必须先登录。企业微信消息由客服系统接收，站内表单创建的留言关联登录账户并可在“我的工单”和网站后台查看。

## 授权

- `GET /api/v1/license/list`
- `GET /api/v1/license/status`
- `POST /api/v1/license/activate` 和 `/switch`
- `POST /api/v1/license/change-mode`
- `POST /api/v1/license/renew` 和 `/offline/issue`
- `POST /api/v1/license/revoke/free`
- `POST /api/v1/license/revoke/paid`
- `POST /api/v1/license/offline/extend`
- `POST /api/v1/license/online/heartbeat`
- `POST /api/v1/license/online/logout`
- `POST /api/v1/license/permanent/confirm`
- `POST|GET /api/v1/license/manual-unbind`
- `GET /api/v1/license/keys`

公钥接口返回当前键以及数据库中所有尚未标记为 `retired` 的历史公钥；客户端应按 `kid`
选择验证键。轮换完成且旧凭证全部过期后，管理员才可将旧键标记为 `retired`。

设备指纹必须是客户端按平台规则计算的 64 位小写 SHA-256 摘要，并携带正整数指纹版本。
服务器不会接收 MachineGuid、machine-id、IOPlatformUUID 等原始标识。

切换永久模式必须提交 `confirmation: "PERMANENT"`。切换为 `offline` 时必须提交严格的
`offline_period_days`：仅允许 7、14 或 30；非法周期返回 `invalid_offline_period`，不会静默改为
其他周期。切换为 `online` 时保留此前的离线周期，切换为 `permanent` 时周期固定为 0。离线签发前必须先绑定当前设备并把
模式切换为 `offline` 或 `permanent`。在线运行租约 TTL 为 2 小时（1 小时为下限，推荐 2～4 小时），
服务端当前默认心跳间隔为 15 分钟，部署调优时推荐保持在 15～30 分钟；客户端应对心跳加入
±10% 随机抖动。该 TTL 用于断网后的在线运行宽限，不等同于 7/14/30 天离线授权
有效期。在线心跳首次返回随机租约令牌，后续心跳必须回传，并返回
`server_time` 供客户端在当前进程内锚定租约倒计时，避免本机墙上时钟调整造成误判。

## 订单、邀请和余额

- `POST /api/v1/orders/preview`：服务器报价。
- `POST /api/v1/orders/create`：再次报价并使用 `idempotency_key` 创建订单。必须回传预览响应中的 `amount_minor`、`currency`、`region`，字段名分别为 `expected_amount_minor`、`expected_currency`、`expected_region`；任一值与创建时服务器报价不一致时返回 `409 quote_changed`，不会创建或扣款。若该账号已有同一幂等键的订单，只有商品、授权、支付渠道、余额金额和确认报价快照均一致时才返回原订单；请求不一致返回 `400 order_invalid`。
- `GET /api/v1/orders`：返回当前账号最近 100 笔订单。
- `GET /api/v1/orders/providers`：返回当前已配置并满足合规开关的 Checkout 渠道；前端不得展示未返回的渠道。
- `GET /api/v1/orders/:id`
- `POST /api/v1/orders/:id/checkout`：为已创建的 Stripe、Creem、Waffo、Waffo Pancake、支付宝电脑网站支付或易支付订单生成 Checkout 信息；服务端再次校验订单归属、状态、过期时间、渠道和网关金额。`alipay_pc` 使用支付宝 `alipay.trade.page.pay` RSA2 签名生成并提交收银台 `POST` 表单，模式由普通系统后台的支付网关设置并嵌入本站 iframe；支付宝商户订单号为 `D2S` 加本地 UUID 的 32 位十六进制字符，`time_expire` 与本地订单截止时间一致。易支付 provider 包括 `alipay`、`wechat` 和可配置的 `paymentfm`，其中 `wechat` 映射到网关的 `wxpay` 参数；Creem 还要求已配置与服务器报价完全匹配的 USD 商品，Waffo/Waffo Pancake 使用服务器价格快照；易支付返回服务端签名的 `POST` 地址和参数，客户端不得自行改写。
- `POST /api/v1/alipay/d2s/notify`：支付宝电脑网站支付异步通知；服务端验 RSA2、公钥签名和应用 ID，并校验商户订单号、订单金额/币种后通过现有 D2S 支付事务履约。关闭新购买入口不会阻断已有订单的已验签通知。同步 return URL 仅用于用户跳转，不能作为支付成功依据。
- `GET/POST /api/v1/alipay/app-gateway`：支付宝开放平台应用网关的公网检测与通知入口。GET 返回检测响应；POST 仅接受验签通过的 `trade_status_sync` 通知并复用电脑网站支付结算逻辑。其他通知类型不启用。
- 普通系统后台支付宝管理接口（与其他支付配置相同，要求系统 root 登录）：`GET/PUT /api/option/alipay/settings` 读取/设置二维码模式，`PUT /api/option/alipay/config` 保存加密参数；`POST /api/option/alipay/orders/:id/refunds` 发起退款，`GET /api/option/alipay/orders/:id/refunds` 查询退款记录，`POST /api/option/alipay/refunds/:out_request_no/reconcile` 查询待确认退款；`POST /api/option/alipay/orders/:id/close` 关单；`GET /api/option/alipay/bill?date=YYYY-MM-DD` 下载交易账单。敏感写操作和账单下载写入后台审计日志。 独立安全审计后台的 Cookie 不具备这些接口权限，审计记录带 `operator_realm=system_admin` 以区分账号体系。
- `GET /api/v1/invite/info`、`GET /api/v1/invite/records`
- `GET /api/v1/balance/info`、`GET /api/v1/balance/transactions`
- `POST /api/v1/withdrawal/request`、`GET /api/v1/withdrawal/status`：用户响应不回传收款支付宝账号或姓名。

`product` 支持 `license`、`paid_revoke` 和 `offline_extension`。金额全部使用最小货币单位。
正式授权固定为 CN/CNY 9900、INTL/USD 2990；付费撤销按 1/2/3 次分别为
CN 1990/3490/6990、INTL 299/499/999。离线延长价格必须由部署配置给出。
`provider: "balance"` 仅适用于已锁定区域的余额全额支付；服务器重新报价并在同一事务中
扣除余额、签发授权，不允许用余额建立区域来源或创建部分余额支付订单。

## 支付事件桥

`POST /api/v1/webhooks/:provider` 只接受已完成渠道原生验签的内部适配器请求。请求体：

```json
{
  "event_id": "provider-event-id",
  "order_id": "d2s-order-id",
  "event_type": "paid",
  "amount_minor": 2990,
  "currency": "USD"
}
```

将请求体原始字节以 `D2S_PAYMENT_BRIDGE_SECRET[_PROVIDER]` 计算 HMAC-SHA256，小写十六进制
结果放入 `X-D2S-Signature`。支持 `paid`、`canceled`、`failed`、`chargeback`、`reversed`。
事件以 `(provider,event_id)` 幂等，重复事件的订单、金额、币种、类型或摘要不一致时拒绝。
该入口同时受请求体大小限制和高危接口限流保护。

当前服务端已在完成渠道官方验签后，将 Stripe、Creem、易支付（包括 `paymentfm`）、Waffo 和 Waffo Pancake
的已规范化订单事件直接送入同一 D2S 事务处理器；普通 new-api 充值仍沿用原有回调逻辑。
PayPal/Paddle 尚未接入，不能作为订单渠道使用。

## 系统支付购买开关

- `GET /api/option/d2s-purchases`：返回 `{"success":true,"data":{"enabled":false}}`。
- `PUT /api/option/d2s-purchases`：请求 `{"enabled":true|false}`，布尔字段必填；与其他支付设置一样，仅允许普通系统 root 管理员，独立安全后台 Cookie 不可授权。
- 修改和 `UPDATE_PURCHASE_SETTINGS` 审计在同一事务保存，包含操作者、来源 IP、时间、启停状态及 `operator_realm=system_admin`。通用 option 写接口禁止修改该键。
- 保存的 `D2SPurchasesEnabled` 优先于旧环境变量 `D2S_PURCHASES_ENABLED`；未保存时沿用环境变量（默认关闭）。每次请求读取数据库，跨节点生效。数据库读取错误拒绝购买，返回 HTTP 503。
- 关闭会拒绝新报价、下单和结账；支付通知、已付款履约、退款和对账继续处理。

## 管理 API

- `GET /api/v1/admin/licenses`（每项包含当前用户区域 `region`；空字符串表示尚未锁定）
- `GET /api/v1/admin/orders[?status=&user_id=]`（包含拒付/冲正后的订单状态）
- `status` 仅支持 `pending`、`paid`、`canceled`、`chargeback`；`user_id` 必须是正整数，非法筛选返回 HTTP 400。
- `GET /api/v1/admin/payment-events[?provider=&order_id=&event_type=]`（逐笔支付事件与渠道事件号）
- `GET /api/v1/admin/balances[?negative=true&user_id=]`（默认查看负余额）
- 余额查询的 `user_id` 同样必须是正整数，非法筛选返回 HTTP 400。
- `GET /api/v1/admin/signing-keys`（仅公开 JWK，不返回私钥）
- `PUT /api/v1/admin/signing-keys/:id`（请求 `{"status":"retired"}`；当前活动键不可退休）
- `GET /api/v1/admin/reconciliation[?start_at=&end_at=]`（默认上一 UTC 日）
- `GET /api/v1/admin/withdrawals`：列表不返回收款账号和姓名；`GET /api/v1/admin/withdrawals/:id/payout` 按单读取待处理申请的收款资料，并在响应前写入 `VIEW_WITHDRAWAL_PAYOUT` 审计；`PUT /api/v1/admin/withdrawals/:id` 审核申请。
- `GET|PUT /api/v1/admin/unbind-requests[/:id]`
- `PUT /api/v1/admin/users/:id/region`

管理员写操作经过 new-api 的管理员审计中间件。区域只允许在余额为零、无待处理订单和提现时调整。
对账报告按订单创建时间和窗口内支付事件的订单引用共同确定范围，因此旧订单在窗口内发生
退款/拒付时也会被纳入。报告会标记 `paid_order_without_payment_event`、
`paid_order_not_settled`、`cancel_event_not_settled`、`reversal_not_settled`、
`orphan_payment_event`、`payment_order_mismatch` 和 `pending_expired`，仅报告异常，不自动改账。

后台独立认证、用户行为日志、注册快照、CSV 导出、哈希校验、购买留痕、账户紧急限制、批量处置、独立管理员审计与受控留存的 API 和字段契约，统一见 [`d2s-admin-management-spec.md`](d2s-admin-management-spec.md)。本 API 文档中列出的管理端点均须由该规格定义的独立后台身份和角色授权保护。

日志和注册信息 CSV 导出须先通过独立管理员双人审批：申请人提交用途和筛选范围，另一名超级管理员批准后，申请人在 30 分钟内使用一次性审批编号下载。审批编号绑定导出类型及精确筛选条件；下载时任何范围变化、审批人自批、过期或重复使用均会被服务端拒绝。审批申请、决定、消费及最终导出结果均写入管理员审计。

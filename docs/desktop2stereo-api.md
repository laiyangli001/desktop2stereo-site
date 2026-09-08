# Desktop2Stereo API v1

D2S 设备、授权、订单、账务和管理响应包含 `version`、`success` 和 `request_id`。失败响应
的 `error.code` 是客户端判断依据，客户端不得解析自然语言消息。账号兼容入口直接复用
new-api 原生响应格式。除设备码申请、兑换、公钥和支付回调外，接口需要 new-api Bearer
access token。

## 账号兼容入口

- `GET /api/captcha`：生成自托管拖动拼图验证码。响应中的 `id`、`master_image`、
  `tile_image`、尺寸和起始坐标仅用于当前验证；验证码默认 5 分钟有效且只能成功消费一次。
- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/logout`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/password-reset/confirm`

登录和注册请求必须携带 `captcha_id`、`captcha_x`、`captcha_y`，分别对应验证码 ID、
拼图提交的横坐标和纵坐标；缺失、过期、重复消费或坐标误差超过服务端容差时返回
`BEHAVIOR_CAPTCHA_REQUIRED`，不会创建会话或账号。发送邮箱验证码和请求密码重置继续复用
`/api/verification`、`/api/reset_password`，其现有 Turnstile 防护保持不变。
这些账号接口保持 new-api 契约，不承诺 D2S `version/request_id/error` 外层结构。

## 设备码

- `POST /api/v1/device/authorize`：提交 `device_hash`、`fingerprint_version`、`client_name`、
  `platform`，返回设备码、用户码、验证地址、600 秒有效期和 5 秒轮询间隔。
- `POST /api/v1/device/approve`：网页登录后提交 `user_code`。
- `POST /api/v1/device/token`：启动器提交 `device_code`；待批准返回 HTTP 202 和
  `authorization_pending`，批准后返回 access/refresh token，且只能兑换一次。
- `POST /api/v1/device/cancel`：取消尚未批准的设备码。

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

切换永久模式必须提交 `confirmation: "PERMANENT"`。离线签发前必须先绑定当前设备并把
模式切换为 `offline` 或 `permanent`。在线心跳首次返回随机租约令牌，后续心跳必须回传。

## 订单、邀请和余额

- `POST /api/v1/orders/preview`：服务器报价。
- `POST /api/v1/orders/create`：再次报价并使用 `idempotency_key` 创建订单。
- `GET /api/v1/orders`：返回当前账号最近 100 笔订单。
- `GET /api/v1/orders/providers`：返回当前已配置并满足合规开关的 Checkout 渠道；前端不得展示未返回的渠道。
- `GET /api/v1/orders/:id`
- `POST /api/v1/orders/:id/checkout`：为已创建的 Stripe、Creem、Waffo、Waffo Pancake 或易支付订单生成 Checkout 信息；服务端再次校验订单归属、状态、过期时间、渠道和网关金额。易支付 provider 包括 `alipay`、`wechat` 和可配置的 `paymentfm`，其中 `wechat` 映射到网关的 `wxpay` 参数；Creem 还要求已配置与服务器报价完全匹配的 USD 商品，Waffo/Waffo Pancake 使用服务器价格快照；易支付返回服务端签名的 `POST` 地址和参数，客户端不得自行改写。
- `GET /api/v1/invite/info`、`GET /api/v1/invite/records`
- `GET /api/v1/balance/info`、`GET /api/v1/balance/transactions`
- `POST /api/v1/withdrawal/request`、`GET /api/v1/withdrawal/status`

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
- `GET|PUT /api/v1/admin/withdrawals[/:id]`
- `GET|PUT /api/v1/admin/unbind-requests[/:id]`
- `PUT /api/v1/admin/users/:id/region`

管理员写操作经过 new-api 的管理员审计中间件。区域只允许在余额为零、无待处理订单和提现时调整。
对账报告按订单创建时间和窗口内支付事件的订单引用共同确定范围，因此旧订单在窗口内发生
退款/拒付时也会被纳入。报告会标记 `paid_order_without_payment_event`、
`paid_order_not_settled`、`cancel_event_not_settled`、`reversal_not_settled`、
`orphan_payment_event`、`payment_order_mismatch` 和 `pending_expired`，仅报告异常，不自动改账。

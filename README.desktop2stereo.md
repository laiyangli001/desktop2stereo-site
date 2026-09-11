# Desktop2Stereo 服务端扩展

本仓库以 [QuantumNous/new-api](https://github.com/QuantumNous/new-api) 提交
`3a9f41ee85cc369f5b8d7fe6e62ff4e7bf3a9ec8` 为基线，保留其 AGPL-3.0
许可证、项目标识和上游归属，并新增 Desktop2Stereo 授权、设备、订单和账务域。

## 当前能力

- 直接复用 new-api 的注册、邮箱验证码、登录、JWT + 刷新令牌轮换、Turnstile、限流、
  管理员鉴权、审计、Redis、SQLite/MySQL/PostgreSQL 和现有支付适配器。
- 新增 30 天试用、多授权、单授权单设备、设备码登录、7/14/30 天离线模式、在线租约、
  永久绑定、免费/付费撤销、离线延长包、ES256 JWS、区域锁定、专属余额、邀请奖励、
  拒付暂停、CN 支付宝提现和管理员审核 API。
- Desktop2Stereo API 使用 `/api/v1` 前缀，返回稳定错误码、版本号和请求 ID。

## 本地启动

1. 生成 P-256 私钥，并只将私钥放入 Secret：

   ```bash
   openssl ecparam -name prime256v1 -genkey -noout -out d2s-license-key.pem
   openssl pkcs8 -topk8 -nocrypt -in d2s-license-key.pem -out d2s-license-key-pkcs8.pem
   ```

2. 复制 `.env.example` 中的 Desktop2Stereo 配置到本地 `.env`，至少设置：

   - `D2S_LICENSE_KEY_ID`（生产默认使用 `d2s-es256-2026-09`）
   - `D2S_LICENSE_PRIVATE_KEY_B64`（仅保存于服务器 Secret 或 `.env`，不要提交到 Git）
   - `D2S_PAYMENT_BRIDGE_SECRET`
   - `D2S_DEVICE_VERIFICATION_URI`
   - 两个离线延长包价格

3. 构建并启动当前源码：

   ```bash
   docker compose up -d --build
   docker compose ps
   ```

   签名密钥配置后用 `curl -fsS http://localhost:3000/api/v1/license/keys` 验证公钥接口；
   返回 `signing_key_unavailable` 表示私钥尚未加载。

4. 打开 `http://localhost:3000` 完成 new-api 初始化，并在系统设置中启用邮箱验证、
   Turnstile 和需要的支付渠道。

服务器端实施计划见 `docs/01-cross-platform-licensing-server-implementation-plan.md`，
API 契约见 `docs/desktop2stereo-api.md`，部署与密钥操作见 `docs/d2s.site.md`，复用/新增边界
见 `docs/new-api-gap-analysis.md`。

## 安全边界

- 客户端不得上传价格或自行确认支付成功；订单创建会重新计算价格。
- 外部支付回调不能直接调用业务结算。支付适配器完成渠道原生验签后，使用
  `X-D2S-Signature` 调用统一事件桥接接口。
- 数据库只保存设备摘要、令牌哈希和 JWS 摘要；不保存原始设备信息、刷新令牌或私钥。
- 缺少签名私钥或离线延长包价格时，对应接口安全失败。
- 生产发布前必须完成支付渠道沙箱、真实三数据库、密钥轮换和灾难恢复演练。

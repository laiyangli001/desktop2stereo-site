# Desktop2Stereo 生产演练 Runbook

本 runbook 用于上线前和季度恢复演练。它不把“脚本执行成功”当作生产验收；每项都必须保存
命令输出、时间、操作者、版本 SHA 和监控截图。真实渠道账号、腾讯云资源和客户端设备必须
使用隔离的生产等价环境。

## 0. 前置条件

- PostgreSQL、Redis、Cloudflare、Nginx/CLB、两个应用实例和支付回调 Worker 已准备。
- 所有 Secret 从腾讯云 Secret 管理或编排系统注入，执行过程不打印值。
- 准备一份 PostgreSQL 快照、上一版本镜像和本次 Git SHA。
- 至少准备一个 CN 渠道和一个 INTL 渠道的沙箱账号；PayPal/Paddle 不得出现在测试矩阵。

先在应用节点执行：

```powershell
.\scripts\d2s-production-readiness.ps1 -BaseUrl https://d2s.site -RequireSecrets
```

`-RequireSecrets` additionally enforces `SESSION_COOKIE_SECURE=true`, HTTPS
`SESSION_COOKIE_TRUSTED_URL` and `D2S_DEVICE_VERIFICATION_URI`, and a non-empty
`TRUSTED_PROXIES` configuration before the HTTP checks are considered sufficient.

## 1. 数据库和双实例

1. 在影子库执行两次迁移，并确认第二次没有破坏性变化：

   ```bash
   go test ./model -run '^TestD2SSchemaConfiguredDatabases$' -count=1 -v
   ```

2. 启动两个应用实例，使用同一 PostgreSQL、Redis、会话 Secret、签名键和桥接 Secret。
3. 用两个并发客户端执行设备绑定、在线心跳、相同订单幂等键和相同支付事件。
4. 保存数据库结果：试用数为 1、在线租约最多 1、幂等订单 1、支付事件 1、账本无重复流水。

## 2. 支付沙箱矩阵

对每个开放渠道分别记录事件 ID、订单 ID、金额、币种、签名验证结果和最终订单状态：

| 场景 | 期望 |
| --- | --- |
| 成功支付 | 订单 `paid`，授权/余额只结算一次 |
| 重复回调 | HTTP 成功或幂等确认，不产生第二次结算 |
| 乱序/延迟 | 状态机拒绝非法跃迁，不破坏已结算订单 |
| 取消/失败 | 订单取消，余额预留释放 |
| 伪造签名 | 拒绝，不能写入支付事件 |
| 金额或币种不符 | 拒绝并告警 |
| 全额退款/拒付 | 订单 `chargeback`，授权暂停，奖励冲正 |
| 部分退款 | 按当前全额金额契约拒绝并进入人工对账 |

日终调用管理员对账接口，并与渠道结算文件逐笔比对：

```bash
curl -H 'Authorization: Bearer <admin-token>' \
  'https://d2s.site/api/v1/admin/reconciliation'
```

## 3. 密钥轮换

1. 发布包含新公钥的客户端版本，记录客户端版本和新 `key_id`。
2. 切换服务端 `D2S_LICENSE_KEY_ID` 与私钥，滚动重启两个实例。
3. 验证新签发凭证、旧凭证、篡改签名和过期凭证。
4. 等待旧离线凭证全部过期后才将旧公钥标记 `retired`。
5. 确认私钥未进入日志、镜像、数据库或客户端包。

## 4. 备份恢复与回滚

1. 对 PostgreSQL 执行加密备份并复制到不同故障域 COS。
2. 在隔离恢复库执行恢复，核对用户、订单、支付事件、余额流水、授权事件和签名记录的
   数量及关联完整性。
3. 将恢复库切换为只读验证环境，运行 D2S 定向测试和客户端授权验收。
4. 灰度上一版本镜像，确认回调、租约和错误率恢复正常；不删除已接收支付事件。
5. 记录切换耗时、RTO、RPO、回滚触发条件和最终操作者。

## 5. 监控和告警

演练中人为触发一次签名失败、金额不符、租约冲突、拒付和负余额，确认告警包含订单/事件
标识但不包含 Secret、令牌、原始设备标识或完整支付资料。没有保存完整证据前，S5 不能标记
为 `verified`。

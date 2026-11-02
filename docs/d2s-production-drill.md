# Desktop2Stereo 生产演练 Runbook

本 runbook 用于上线前和季度恢复演练。它不把“脚本执行成功”当作生产验收；每项都必须保存
命令输出、时间、操作者、版本 SHA 和监控截图。真实渠道账号、腾讯云资源和客户端设备必须
使用隔离的生产等价环境。

## 0. 前置条件

- PostgreSQL、Redis、Cloudflare、Nginx/CLB、两个应用实例和支付回调 Worker 已准备；两个实例
  使用相同的渠道专用桥接 Secret（`D2S_PAYMENT_BRIDGE_SECRET_{PROVIDER}`）。
- 所有 Secret 从腾讯云 Secret 管理或编排系统注入，执行过程不打印值。
- 准备一份 PostgreSQL 快照、上一版本镜像和本次 Git SHA。
- 至少准备一个 CN 渠道和一个 INTL 渠道的沙箱账号；PayPal/Paddle 不得出现在测试矩阵。

先在应用节点输入本次发布的完整 40 位 Git SHA 并执行：

```powershell
$releaseSha = Read-Host "请输入待验收的 40 位 Git 提交 SHA"
.\scripts\d2s-production-readiness.ps1 -BaseUrl https://d2s.site -ExpectedVersion $releaseSha -RequireSecrets -RequiredPaymentProviders stripe,waffo_pancake
```

`-ExpectedVersion` 会将 `/api/status` 返回的运行版本与本次发布 SHA 做区分大小写的精确比较；
版本不匹配、响应 JSON 无效或同时指定 `-SkipHttp` 时检查失败。部署后必须传入目标 SHA，避免旧实例健康但新版本尚未生效时被判定为通过。

`-RequireSecrets` additionally enforces `SESSION_COOKIE_SECURE=true`, HTTPS
`SESSION_COOKIE_TRUSTED_URL` and `D2S_DEVICE_VERIFICATION_URI`, and a non-empty
`TRUSTED_PROXIES` configuration before the HTTP checks are considered sufficient.
It also validates positive integer offline-extension prices and Base64 private-key
material, requires 32-character session and payment-bridge secrets, rejects
wildcard or whole-address-space trusted-proxy entries, and requires the device and
cookie trust URLs to use the BaseUrl host or a subdomain on the same port.
Pass the channels enabled in the deployment to `-RequiredPaymentProviders`; each
listed channel must have its own 32-character `D2S_PAYMENT_BRIDGE_SECRET_{PROVIDER}`
value. The parameter does not require secrets for disabled channels.
When HTTP checks are enabled, the script also rejects an `http://` `-BaseUrl`.

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

易支付 V1 的 `TRADE_REFUND` 回调可能表示部分或全部退款；服务端将其送入统一金额校验，
只有金额等于原支付金额时才会完成冲正，部分退款保留订单状态并进入人工对账。

易支付还需独立验证发起侧重试契约：对同一 `out_trade_no` 重复提交同一签名表单，检查网关是否返回同一订单、超时后的重试行为，以及按商户订单号查单；保存沙箱事件/订单标识和网关响应。当前实现稳定复用 D2S 订单 ID，但网关是否幂等未知，不能将本地确定性表单视为网关幂等。

日终调用管理员对账接口，并与渠道结算文件逐笔比对：

```bash
curl -H 'Authorization: Bearer <D2S_RECONCILIATION_TOKEN>' \
  'https://d2s.site/api/v1/admin/reconciliation'
```

也可以用仓库脚本保存不可变的 JSON 报告；脚本不会打印或写入管理员令牌，也不会修改订单或
账本：

```powershell
.\scripts\d2s-reconciliation.ps1 -ReconciliationToken $env:D2S_RECONCILIATION_TOKEN
```

如需重跑指定 UTC 窗口，同时传入 `-StartAt`、`-EndAt` 和唯一的 `-OutputPath`；脚本拒绝覆盖
已有报告。接入计划任务或告警时增加 `-FailOnMismatch`，脚本会先保存报告，再以失败状态退出，
避免把存在差异的日期误判为已结算；默认不加该参数时保持人工复核流程。

生产日终任务使用固定的 `/usr/local/sbin/desktop2stereo-reconciliation` 和
`desktop2stereo-reconciliation.timer`。管理员令牌只放在 `/etc/desktop2stereo/reconciliation.env`
（`0600`），报告写入 `/opt/desktop2stereo-site/reconciliation-reports/`；同一天重复执行不会覆盖
原报告。差异会使 service 失败，必须通过 systemd/journal 的服务器监控告警并人工核对渠道结算文件。

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

Docker 部署可先用仓库脚本在隔离的 PostgreSQL 容器中验证备份可恢复；脚本使用固定容器名，
不会连接或修改生产 `postgres` 容器，验证完成后自动删除隔离容器：

```bash
export D2S_RECOVERY_POSTGRES_PASSWORD='<recovery-only-password>'
deploy/desktop2stereo-db-restore-verify-docker.sh \
  /opt/desktop2stereo-site/backups/pre-<commit-sha>-<timestamp>.dump
```

执行前必须确认对应 `.sha256` 文件存在；恢复密码只能注入当前进程或 Secret 管理器，不能写入
仓库、备份文件或日志。该脚本只证明备份可在隔离库恢复，仍需记录真实 COS 跨故障域复制和
恢复库数据核对结果。

## 5. Windows/Linux/macOS 授权端到端验收

在生产等价环境中使用每个平台独立的发布包和预置隔离账号/许可证；记录客户端发布 SHA、API
版本、操作系统版本、时间和验收人。不要在真实用户许可证上测试永久绑定或人工解绑。

| 平台 | 目标环境 | 必验流程 | 必存证据 |
| --- | --- | --- | --- |
| Windows x86_64 | Windows 发布包，HTTPS API，证书校验开启 | 邮箱验证/登录、设备码授权、授权列表与设备绑定、online/offline 模式切换、online 租约心跳、并发第二实例拒绝、退出释放租约、offline 7/14/30 天凭证签发与断网启动 | 客户端 SHA、Windows 版本、请求 ID、脱敏启动/租约日志、关键状态截图 |
| Linux x86_64 | Linux 发布包，HTTPS API，证书校验开启 | 邮箱验证/登录、设备码授权、授权列表与设备绑定、online/offline 模式切换、online 租约心跳、并发第二实例拒绝、退出释放租约、offline 7/14/30 天凭证签发与断网启动 | 客户端 SHA、发行版/内核版本、请求 ID、脱敏启动/租约日志、关键状态截图 |
| macOS arm64 | macOS 发布包，HTTPS API，证书校验开启 | 邮箱验证/登录、设备码授权、授权列表与设备绑定、online/offline 模式切换、online 租约心跳、并发第二实例拒绝、退出释放租约、offline 7/14/30 天凭证签发与断网启动 | 客户端 SHA、macOS 版本、请求 ID、脱敏启动/租约日志、关键状态截图 |

每个平台还要验证另一设备不能复用该许可证、篡改/过期/错设备的离线凭证 fail-closed；online
模式在 API 不可达或租约失效时必须拒绝受保护运行，offline 模式仅在有效凭证期限内允许断网启动。
永久绑定确认和人工解绑审批只能使用一次性隔离账号及许可证；若该额度被批准，验收记录必须
证明同一账号第二次申请或审批被拒绝。不得保存密码、访问令牌、刷新令牌、设备原始标识、私钥或支付 Secret。
三个平台的证据齐全前，S5 保持 `in_progress`。

## 6. 监控和告警

演练中人为触发一次签名失败、金额不符、租约冲突、拒付和负余额，确认告警包含订单/事件
标识但不包含 Secret、令牌、原始设备标识或完整支付资料。没有保存完整证据前，S5 不能标记
为 `verified`。

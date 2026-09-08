# Tencent SES 邮件推送配置与运维

## 适用范围

New API 管理后台的“运维 → 邮件推送”使用腾讯云 SES 模板发送事务类和通知类邮件。发信域名、SPF、DKIM、MX 和模板审核由腾讯云控制台完成，New API 不提供域名创建或验证功能。

## 后台配置

进入“运维 → 邮件推送”，填写以下内容后保存：

- 启用腾讯云邮件推送；
- 地域，默认 `ap-guangzhou`，也支持 `ap-hongkong`；
- SecretId 和 SecretKey；
- 已验证发信地址及可选发件人名称；
- 可选 Reply-To 地址和主题前缀；
- 请求超时时间和有限重试次数；
- 业务场景和语言到腾讯云 `TemplateID` 的 JSON 映射。

SecretKey 不会回显到接口或页面。留空 SecretId/SecretKey 后保存会保留服务器上已有值。不要把真实凭证提交到 Git、前端代码或普通日志。

示例：

```json
{
  "email_verification": {
    "zhCN": 1001,
    "en": 1002
  },
  "password_reset": {
    "zhCN": 1003,
    "en": 1004
  },
  "system_notification": {
    "zhCN": 1005,
    "en": 1006
  }
}
```

`zhCN` 表示简体中文，`en` 表示英文。示例中的 ID 仅用于说明格式，不能直接用于生产环境。请替换为腾讯云控制台中已审核的真实模板 ID。旧版单层格式（例如 `{"password_reset":1003}`）仍然兼容，并作为不区分语言的默认模板使用。

语言选择优先级为：密码重置优先使用用户保存的语言；其他未登录场景使用请求的 `lang` 参数、`X-User-Language` 或 `Accept-Language`；无法判断时默认使用 `zhCN`。如果目标语言没有配置模板，会依次回退到简体中文、英文和旧版默认模板。

## 标准场景变量

业务代码只传模板变量，不拼接 HTML。以下场景由服务端进行发送前校验：

| 场景 | 必填变量 | 用途 |
| --- | --- | --- |
| `email_verification` | `code` | 邮箱验证码 |
| `password_reset` | `reset_url` | 密码重置链接 |
| `system_notification` | `title`, `content` | 系统通知 |

现有模板如果还要求 `expire_minutes`、`system_name`、订单号或金额等变量，应在腾讯云模板中保持变量名与业务发送参数一致，并由对应业务入口补齐。新增业务场景必须先创建并审核腾讯云模板，再配置其 `TemplateID`。

密码重置和邮箱验证使用已经在腾讯云配置的模板；系统不会调用 `CreateEmailIdentity`，也不会执行域名验证。

## 测试与日志

- “测试连接”只调用腾讯云模板查询接口，不发送邮件；至少配置一个模板 ID 后才能测试。
- “发送测试邮件”必须填写测试收件地址，并明确选择配置中的场景和模板。
- 发送日志只记录场景、脱敏收件地址、状态、腾讯云 RequestId、MessageId 和脱敏错误，不记录正文、验证码、模板变量或凭证。
- 发送失败会执行有限次数的指数退避，不会无限重试。

相关管理 API：

```text
GET  /api/option/tencent-ses
POST /api/option/tencent-ses/test-connection
POST /api/option/tencent-ses/test-send
GET  /api/option/tencent-ses/logs
```

这些接口沿用 New API 的 root 管理员鉴权路由。腾讯云调用由 Go 服务端通过 `net/http` 和 TC3-HMAC-SHA256 完成，浏览器不会直接访问腾讯云 API。

## 部署检查

1. 在腾讯云 SES 中确认发信域名和模板已经审核通过。
2. 在 New API 中保存凭证、地域、发件地址和模板映射。
3. 先执行“测试连接”，确认凭证、地域和模板查询正常。
4. 使用已获许可的测试收件地址发送测试邮件。
5. 检查发送日志中的 RequestId，并确认密码重置、邮箱验证等实际业务流程。
6. 生产环境启用数据库备份，限制管理后台访问来源，并定期轮换腾讯云凭证。

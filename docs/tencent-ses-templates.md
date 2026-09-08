# Desktop2Stereo 腾讯云 SES 模板

本文档提供邮箱验证和密码重置的中英文四个事务类模板。模板正文可以直接复制到腾讯云 SES 控制台创建并提交审核，然后把审核通过的 `TemplateID` 配置到“运维 → 邮件推送”。

## 变量约定

当前业务流程使用以下变量：

- 邮箱验证：`{{token}}`、`{{expire_minutes}}`、`{{system_name}}`
- 密码重置：`{{token}}`、`{{expire_minutes}}`、`{{system_name}}`

邮箱验证模板中的 `{{token}}` 是用户需要输入的验证码。密码重置模板中的 `{{token}}` 是系统生成的完整重置链接，必须直接用于 `href="{{token}}"`，不能再拼接 `?token=`。系统会继续兼容旧模板中的 `code` 和 `reset_url` 变量。

## 中文邮箱验证

主题：

```text
验证 {{system_name}} 邮箱
```

正文：

```html
<!DOCTYPE html>
<html lang="zh-CN">
<body style="margin:0;padding:32px 16px;background:#f4f7ff;font-family:Arial,'Microsoft YaHei',sans-serif;color:#202938;line-height:1.7;">
  <div style="max-width:560px;margin:0 auto;padding:32px;background:#ffffff;border:1px solid #dfe7ff;border-radius:16px;">
    <h2 style="margin:0 0 20px;color:#172033;">验证 {{system_name}} 邮箱</h2>
    <p>您好！</p>
    <p>感谢注册 {{system_name}}。请输入下面的验证码完成邮箱验证：</p>
    <p style="margin:24px 0;text-align:center;font-size:32px;font-weight:700;letter-spacing:8px;color:#2563eb;">{{token}}</p>
    <p>验证码有效期为 {{expire_minutes}} 分钟。如果这不是您的操作，请忽略此邮件。</p>
    <p style="margin-bottom:0;color:#66728f;">{{system_name}} 团队</p>
  </div>
</body>
</html>
```

## English email verification

Subject:

```text
Verify your {{system_name}} email
```

Body:

```html
<!DOCTYPE html>
<html lang="en">
<body style="margin:0;padding:32px 16px;background:#f4f7ff;font-family:Arial,sans-serif;color:#202938;line-height:1.7;">
  <div style="max-width:560px;margin:0 auto;padding:32px;background:#ffffff;border:1px solid #dfe7ff;border-radius:16px;">
    <h2 style="margin:0 0 20px;color:#172033;">Verify your {{system_name}} email</h2>
    <p>Hello,</p>
    <p>Thank you for signing up for {{system_name}}. Enter the verification code below to verify your email address:</p>
    <p style="margin:24px 0;text-align:center;font-size:32px;font-weight:700;letter-spacing:8px;color:#2563eb;">{{token}}</p>
    <p>This code expires in {{expire_minutes}} minutes. If you did not request this email, you can safely ignore it.</p>
    <p style="margin-bottom:0;color:#66728f;">The {{system_name}} Team</p>
  </div>
</body>
</html>
```

## 中文密码重置

主题：

```text
重置 {{system_name}} 密码
```

正文：

```html
<!DOCTYPE html>
<html lang="zh-CN">
<body style="margin:0;padding:32px 16px;background:#f4f7ff;font-family:Arial,'Microsoft YaHei',sans-serif;color:#202938;line-height:1.7;">
  <div style="max-width:560px;margin:0 auto;padding:32px;background:#ffffff;border:1px solid #dfe7ff;border-radius:16px;">
    <h2 style="margin:0 0 20px;color:#172033;">重置 {{system_name}} 密码</h2>
    <p>您好！</p>
    <p>我们收到了重置您账号密码的请求，请点击下方按钮继续：</p>
    <p style="margin:28px 0;text-align:center;"><a href="{{token}}" style="display:inline-block;padding:11px 24px;background:#4f46e5;color:#ffffff;text-decoration:none;border-radius:8px;">重置密码</a></p>
    <p>如果按钮无法打开，请复制以下链接：</p>
    <p style="word-break:break-all;"><a href="{{token}}">{{token}}</a></p>
    <p>该链接有效期为 {{expire_minutes}} 分钟，只能使用一次。如果这不是您的操作，请忽略此邮件。</p>
    <p style="margin-bottom:0;color:#66728f;">{{system_name}} 团队</p>
  </div>
</body>
</html>
```

## English password reset

Subject:

```text
Reset your {{system_name}} password
```

Body:

```html
<!DOCTYPE html>
<html lang="en">
<body style="margin:0;padding:32px 16px;background:#f4f7ff;font-family:Arial,sans-serif;color:#202938;line-height:1.7;">
  <div style="max-width:560px;margin:0 auto;padding:32px;background:#ffffff;border:1px solid #dfe7ff;border-radius:16px;">
    <h2 style="margin:0 0 20px;color:#172033;">Reset your {{system_name}} password</h2>
    <p>Hello,</p>
    <p>We received a request to reset your {{system_name}} account password. Click the button below to continue:</p>
    <p style="margin:28px 0;text-align:center;"><a href="{{token}}" style="display:inline-block;padding:11px 24px;background:#4f46e5;color:#ffffff;text-decoration:none;border-radius:8px;">Reset Password</a></p>
    <p>If the button does not work, copy and paste this link:</p>
    <p style="word-break:break-all;"><a href="{{token}}">{{token}}</a></p>
    <p>This link expires in {{expire_minutes}} minutes and can only be used once. If you did not request a password reset, you can safely ignore this email.</p>
    <p style="margin-bottom:0;color:#66728f;">The {{system_name}} Team</p>
  </div>
</body>
</html>
```

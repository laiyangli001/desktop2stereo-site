# Desktop2Stereo 腾讯云 SES 统一视觉模板

以下四个正文保留参考邮件中的卡片、渐变头部、按钮和表格风格，但已经移除网易邮箱阅读器的脚本包装、旧品牌、真实邮箱、真实 token 和示例链接。复制到腾讯云 SES 时只复制对应的 HTML 正文，不要复制外层阅读器脚本。

邮箱验证模板使用 `{{token}}`；密码重置模板使用 `{{email}}` 和 `{{token}}` 拼接完整链接。

- 邮箱验证模板：`{{token}}` 是六位邮箱验证码；
- 密码重置模板：`{{email}}` 和 `{{token}}` 拼成完整密码重置 URL。

## 中文邮箱验证

主题：`验证 Desktop2Stereo 邮箱`

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>验证 Desktop2Stereo 邮箱</title>
</head>
<body style="margin:0;padding:0;">
<div style="background-color:#f4f7ff;padding:40px 16px;font-family:-apple-system,BlinkMacSystemFont,'SF Pro Text','Helvetica Neue',Arial,'PingFang SC','Microsoft YaHei',sans-serif;color:#172033;line-height:1.55;">
  <div style="max-width:560px;margin:0 auto;overflow:hidden;background-color:#ffffff;border:1px solid #dfe7ff;border-radius:18px;padding:40px 40px 32px;box-shadow:0 16px 40px rgba(37,57,128,0.10);">
    <div style="margin:-40px -40px 32px;padding:25px 32px;background-color:#5b5ce2;background-image:linear-gradient(135deg,#0891b2 0%,#4f46e5 55%,#7c3aed 100%);"><a href="https://100393.com/" style="display:inline-block;color:#ffffff;text-decoration:none;font-size:20px;font-weight:750;"><span style="display:inline-block;width:10px;height:10px;margin-right:11px;border-radius:999px;background:#a5f3fc;vertical-align:1px;"></span>Desktop2Stereo</a></div>
    <h1 style="margin:0 0 8px;font-size:24px;color:#172033;">验证 Desktop2Stereo 邮箱</h1>
    <p style="margin:0 0 28px;font-size:14px;color:#66728f;">感谢注册 Desktop2Stereo，请输入下方验证码完成邮箱验证。</p>
    <div style="margin:24px 0;padding:18px;text-align:center;background:#f4f7ff;border-radius:12px;font-size:32px;font-weight:700;letter-spacing:8px;color:#4f46e5;">{{token}}</div>
    <p style="margin:0;font-size:13px;color:#66728f;">验证码 10 分钟内有效。如果这不是您的操作，请忽略此邮件。</p>
  </div>
</div>
</body>
</html>
```

## English email verification

Subject: `Verify your Desktop2Stereo email`

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Verify your Desktop2Stereo email</title>
</head>
<body style="margin:0;padding:0;">
<div style="background-color:#f4f7ff;padding:40px 16px;font-family:-apple-system,BlinkMacSystemFont,'SF Pro Text','Helvetica Neue',Arial,sans-serif;color:#172033;line-height:1.55;">
  <div style="max-width:560px;margin:0 auto;overflow:hidden;background-color:#ffffff;border:1px solid #dfe7ff;border-radius:18px;padding:40px 40px 32px;box-shadow:0 16px 40px rgba(37,57,128,0.10);">
    <div style="margin:-40px -40px 32px;padding:25px 32px;background-color:#5b5ce2;background-image:linear-gradient(135deg,#0891b2 0%,#4f46e5 55%,#7c3aed 100%);"><a href="https://100393.com/" style="display:inline-block;color:#ffffff;text-decoration:none;font-size:20px;font-weight:750;"><span style="display:inline-block;width:10px;height:10px;margin-right:11px;border-radius:999px;background:#a5f3fc;vertical-align:1px;"></span>Desktop2Stereo</a></div>
    <h1 style="margin:0 0 8px;font-size:24px;color:#172033;">Verify your Desktop2Stereo email</h1>
    <p style="margin:0 0 28px;font-size:14px;color:#66728f;">Thank you for signing up for Desktop2Stereo. Enter the verification code below.</p>
    <div style="margin:24px 0;padding:18px;text-align:center;background:#f4f7ff;border-radius:12px;font-size:32px;font-weight:700;letter-spacing:8px;color:#4f46e5;">{{token}}</div>
    <p style="margin:0;font-size:13px;color:#66728f;">This code expires in 10 minutes. If you did not request this email, you can safely ignore it.</p>
  </div>
</div>
</body>
</html>
```

## 中文密码重置

主题：`重置 Desktop2Stereo 密码`

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>重置 Desktop2Stereo 密码</title>
</head>
<body style="margin:0;padding:0;">
<div style="background-color:#f4f7ff;padding:40px 16px;font-family:-apple-system,BlinkMacSystemFont,'SF Pro Text','Helvetica Neue',Arial,'PingFang SC','Microsoft YaHei',sans-serif;color:#172033;line-height:1.55;">
  <div style="max-width:560px;margin:0 auto;overflow:hidden;background-color:#ffffff;border:1px solid #dfe7ff;border-radius:18px;padding:40px 40px 32px;box-shadow:0 16px 40px rgba(37,57,128,0.10);">
    <div style="margin:-40px -40px 32px;padding:25px 32px;background-color:#5b5ce2;background-image:linear-gradient(135deg,#0891b2 0%,#4f46e5 55%,#7c3aed 100%);"><a href="https://100393.com/" style="display:inline-block;color:#ffffff;text-decoration:none;font-size:20px;font-weight:750;"><span style="display:inline-block;width:10px;height:10px;margin-right:11px;border-radius:999px;background:#a5f3fc;vertical-align:1px;"></span>Desktop2Stereo</a></div>
    <h1 style="margin:0 0 8px;font-size:24px;color:#172033;">重置 Desktop2Stereo 密码</h1>
    <p style="margin:0 0 28px;font-size:14px;color:#66728f;">我们收到了密码重置请求，请使用下方按钮继续。</p>
    <div style="margin:28px 0 16px;text-align:center;"><a href="https://100393.com/user/reset?email={{email}}&amp;token={{token}}" style="display:inline-block;padding:11px 24px;background-image:linear-gradient(135deg,#0891b2 0%,#4f46e5 58%,#7c3aed 100%);color:#ffffff;text-decoration:none;border-radius:11px;box-shadow:0 8px 18px rgba(79,70,229,0.22);font-size:14px;font-weight:650;">重置密码</a></div>
    <p style="margin:0;font-size:13px;color:#66728f;">该链接 10 分钟内有效且只能使用一次。如果这不是您的操作，请忽略此邮件。</p>
  </div>
</div>
</body>
</html>
```

## English password reset

Subject: `Reset your Desktop2Stereo password`

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Reset your Desktop2Stereo password</title>
</head>
<body style="margin:0;padding:0;">
<div style="background-color:#f4f7ff;padding:40px 16px;font-family:-apple-system,BlinkMacSystemFont,'SF Pro Text','Helvetica Neue',Arial,sans-serif;color:#172033;line-height:1.55;">
  <div style="max-width:560px;margin:0 auto;overflow:hidden;background-color:#ffffff;border:1px solid #dfe7ff;border-radius:18px;padding:40px 40px 32px;box-shadow:0 16px 40px rgba(37,57,128,0.10);">
    <div style="margin:-40px -40px 32px;padding:25px 32px;background-color:#5b5ce2;background-image:linear-gradient(135deg,#0891b2 0%,#4f46e5 55%,#7c3aed 100%);"><a href="https://100393.com/" style="display:inline-block;color:#ffffff;text-decoration:none;font-size:20px;font-weight:750;"><span style="display:inline-block;width:10px;height:10px;margin-right:11px;border-radius:999px;background:#a5f3fc;vertical-align:1px;"></span>Desktop2Stereo</a></div>
    <h1 style="margin:0 0 8px;font-size:24px;color:#172033;">Reset your Desktop2Stereo password</h1>
    <p style="margin:0 0 28px;font-size:14px;color:#66728f;">We received a password reset request. Use the button below to continue.</p>
    <div style="margin:28px 0 16px;text-align:center;"><a href="https://100393.com/user/reset?email={{email}}&amp;token={{token}}" style="display:inline-block;padding:11px 24px;background-image:linear-gradient(135deg,#0891b2 0%,#4f46e5 58%,#7c3aed 100%);color:#ffffff;text-decoration:none;border-radius:11px;box-shadow:0 8px 18px rgba(79,70,229,0.22);font-size:14px;font-weight:650;">Reset Password</a></div>
    <p style="margin:0;font-size:13px;color:#66728f;">This link expires in 10 minutes and can only be used once. If you did not request a password reset, you can safely ignore this email.</p>
  </div>
</div>
</body>
</html>
```

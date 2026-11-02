# Let’s Encrypt 域名与 IP 证书申请指南

本文记录 `d2s.site` 服务使用 Let’s Encrypt 同时保护域名和服务器 IP 的申请、部署与续期方法。

目标证书 SAN：

```text
DNS:d2s.site
DNS:www.d2s.site
IP:118.89.79.159
```

## 1. 重要限制

Let’s Encrypt 的 IP 地址证书属于 `shortlived` 短周期证书，有效期约 160 小时（约 6 天）。申请和续期必须完全自动化，不能依赖人工每 6 天操作一次。

IP 证书要求使用支持 IP SAN 和 `shortlived` profile 的新版 Certbot。服务器系统自带的旧版 Certbot 可能不支持该功能；本服务器已使用独立路径 `/opt/certbot-ip` 安装 Certbot 5.4，不能用旧版 `/usr/bin/certbot` 代替。

申请最终证书前，不得吊销当前有效证书。必须先签发、安装、验证新证书，确认无误后再决定是否吊销旧证书。

## 2. 申请前置条件

### 2.1 域名 DNS

实名认证完成后，在 DNSPod 或实际权威 DNS 服务商添加：

```text
d2s.site       A    118.89.79.159
www.d2s.site   A    118.89.79.159
```

从服务器和公网解析器确认记录已经生效：

```bash
dig +short A d2s.site @1.1.1.1
dig +short A www.d2s.site @1.1.1.1
dig +short A d2s.site @8.8.8.8
dig +short A www.d2s.site @8.8.8.8
```

所有结果都应为：

```text
118.89.79.159
```

如果域名仍返回 DNSPod 拦截页，或没有返回服务器 IP，Let’s Encrypt 的域名验证会失败。

### 2.2 网络端口

HTTP-01 验证需要公网访问服务器的 TCP 80 端口，IP 证书也需要通过 HTTP-01 或 TLS-ALPN-01 验证。确认防火墙、安全组和云平台策略允许 TCP 80/443 入站。

### 2.3 Certbot 版本

检查独立 Certbot：

```bash
/opt/certbot-ip/bin/certbot --version
```

版本应为 5.3 或更高，推荐 5.4 或更高。

## 3. 安装新版 Certbot（只需一次）

如果服务器没有新版 Certbot，可以使用独立虚拟环境安装，不覆盖系统旧版：

```bash
sudo python3 -m venv --without-pip /opt/certbot-ip
sudo curl --fail --location --retry 3 \
  -o /tmp/get-pip-d2s.py https://bootstrap.pypa.io/get-pip.py
sudo /opt/certbot-ip/bin/python /tmp/get-pip-d2s.py --disable-pip-version-check
sudo /opt/certbot-ip/bin/pip install --no-cache-dir \
  'certbot==5.4.0' 'certbot-nginx==5.4.0'
```

验证：

```bash
/opt/certbot-ip/bin/certbot --version
```

## 4. Staging 测试申请

先使用 Let’s Encrypt staging 环境测试，不消耗生产签发额度，也不会替换线上证书。

由于当前 Certbot Nginx authenticator 不支持 IP 证书，使用 standalone 模式。standalone 模式申请期间需要暂时停止 Nginx，申请完成后立即恢复：

```bash
set +e
sudo systemctl stop nginx
STOP_STATUS=$?

if [ "$STOP_STATUS" -ne 0 ]; then
  exit "$STOP_STATUS"
fi

sudo /opt/certbot-ip/bin/certbot certonly \
  --standalone \
  --staging \
  --preferred-profile shortlived \
  -d d2s.site \
  -d www.d2s.site \
  --ip-address 118.89.79.159 \
  --cert-name d2s-site-ip-staging \
  --non-interactive \
  --agree-tos \
  --register-unsafely-without-email

CERT_STATUS=$?
sudo systemctl start nginx
START_STATUS=$?

echo "CERT_STATUS=$CERT_STATUS START_STATUS=$START_STATUS"
if [ "$CERT_STATUS" -ne 0 ]; then
  exit "$CERT_STATUS"
fi
exit "$START_STATUS"
```

测试证书检查：

```bash
sudo openssl x509 \
  -in /etc/letsencrypt/live/d2s-site-ip-staging/fullchain.pem \
  -noout -subject -dates -ext subjectAltName
```

测试结束后删除 staging 证书，避免误用：

```bash
sudo /opt/certbot-ip/bin/certbot delete \
  --cert-name d2s-site-ip-staging \
  --non-interactive
```

## 5. 正式申请

只有 staging 验证通过后，才申请正式证书。正式申请命令与 staging 相同，但去掉 `--staging`，并使用正式证书名称：

```bash
set +e
sudo systemctl stop nginx
STOP_STATUS=$?

if [ "$STOP_STATUS" -ne 0 ]; then
  exit "$STOP_STATUS"
fi

sudo /opt/certbot-ip/bin/certbot certonly \
  --standalone \
  --preferred-profile shortlived \
  -d d2s.site \
  -d www.d2s.site \
  --ip-address 118.89.79.159 \
  --cert-name d2s-site-ip \
  --non-interactive \
  --agree-tos \
  --register-unsafely-without-email

CERT_STATUS=$?
sudo systemctl start nginx
START_STATUS=$?

echo "CERT_STATUS=$CERT_STATUS START_STATUS=$START_STATUS"
if [ "$CERT_STATUS" -ne 0 ]; then
  exit "$CERT_STATUS"
fi
exit "$START_STATUS"
```

检查正式证书：

```bash
sudo openssl x509 \
  -in /etc/letsencrypt/live/d2s-site-ip/fullchain.pem \
  -noout -subject -issuer -dates -ext subjectAltName
```

必须确认 SAN 同时包含 `d2s.site`、`www.d2s.site` 和 `118.89.79.159`，且证书仍由当前系统信任链签发。

## 6. Nginx 和宝塔部署

先备份当前证书和 Nginx 配置：

```bash
sudo cp -a /etc/nginx/ssl/d2s.site \
  "/etc/nginx/ssl/d2s.site.backup.$(date +%Y%m%d%H%M%S)"
sudo nginx -T > "/tmp/nginx-config.$(date +%Y%m%d%H%M%S).txt"
```

在宝塔面板中为对应站点安装新证书，或将 Nginx 配置指向：

```text
证书：/etc/letsencrypt/live/d2s-site-ip/fullchain.pem
私钥：/etc/letsencrypt/live/d2s-site-ip/privkey.pem
```

不要把证书私钥复制到 Git 仓库，也不要在日志中输出私钥内容。

配置后执行：

```bash
sudo nginx -t
sudo systemctl reload nginx
```

## 7. 三个入口验证

验证证书 SAN：

```bash
echo | openssl s_client -connect d2s.site:443 -servername d2s.site 2>/dev/null \
  | openssl x509 -noout -subject -dates -ext subjectAltName

echo | openssl s_client -connect www.d2s.site:443 -servername www.d2s.site 2>/dev/null \
  | openssl x509 -noout -subject -dates -ext subjectAltName

echo | openssl s_client -connect 118.89.79.159:443 -servername 118.89.79.159 2>/dev/null \
  | openssl x509 -noout -subject -dates -ext subjectAltName
```

验证 HTTP 接口：

```bash
curl --fail --show-error https://d2s.site/api/status
curl --fail --show-error https://www.d2s.site/api/status
curl --fail --show-error https://118.89.79.159/api/status
```

## 8. 自动续期

IP 证书约 6 天到期，必须确认自动续期实际调用 Certbot 5.4 或更高版本：

```bash
sudo /opt/certbot-ip/bin/certbot renew --dry-run
```

宝塔面板如果支持 `shortlived` profile 和 IP SAN，应使用宝塔的证书管理和自动续签功能，并在面板中确认续签任务调用的是新版 Certbot。

服务器原有的系统任务可能调用 `/usr/bin/certbot`。该路径如果仍为旧版 Certbot 2.x，不能用于 IP 证书，不能把“任务存在”当作自动续期已配置完成。

每次续签后必须自动执行：

```bash
sudo nginx -t
sudo systemctl reload nginx
```

如果宝塔版本不能管理 IP 短周期证书，应先升级宝塔证书组件或使用新版 Certbot 的官方自动续期服务；不应关闭 TLS 校验，也不应改用自签名证书冒充公网可信证书。

## 9. 故障排查

### 域名验证返回 DNSPod 拦截页

检查公网 DNS 是否已完成实名认证和解析。若 CA 访问到 `dnspod.qcloud.com/static/webblock.html`，说明域名验证请求没有到达本服务器，需要先解决域名实名、DNS 记录或 DNSPod 拦截问题。

### IP 验证成功、域名验证失败

这表示服务器 IP 证书能力正常，但域名尚未指向服务器。不能把只有 IP SAN 的证书安装到域名站点后声称三个入口均已完成验证。

### Nginx 证书更新后仍显示旧证书

执行以下检查：

```bash
sudo nginx -T | grep -E 'server_name|ssl_certificate|ssl_certificate_key'
sudo nginx -t
sudo systemctl reload nginx
```

确认 Nginx 配置实际引用的是 `/etc/letsencrypt/live/d2s-site-ip/` 下的证书，并确认没有被宝塔面板配置覆盖。

### 证书申请失败

不要吊销旧证书。保留失败日志和当前线上证书，先修复 DNS、端口、验证方式或 Certbot 版本，再重新执行 staging 测试。

# Desktop2Stereo 后台账户审计与治理规格

版本：1.0

状态：接口与实现规格；独立管理员后端和 `/d2s-admin-console` 管理页面已落地，前端 `tsgo -b` 与 Rsbuild 生产构建已通过；MySQL 8.4.9/PostgreSQL 15.14 隔离库权限脚本执行通过，应用账号 INSERT/SELECT 成功，UPDATE/DELETE/TRUNCATE 被拒绝；PostgreSQL 单列 UPDATE 注入后由权限脚本撤销，清理权限范围也经断言。对账专用机器令牌和每次对账审计已在本地实现并通过 Go 测试，尚未部署生产；生产部署仓库与本地基线不同且工作区有既有改动，因此定时器保持禁用。生产权限与迁移及端到端验收仍待完成，具体状态以 `docs/01-cross-platform-licensing-server-implementation-plan.md` 为准。

本规格面向仅用于程序购买、账户登录和授权管理的服务。网站不提供公开内容发布、评论、即时通讯或群组功能；账户以邮箱识别，面向包括没有中国手机号或身份证件的全球用户。注册不强制实名，境外证件字段仅在触发式核验时使用。

## 1. 数据模型

所有时间使用 UTC Unix 秒。用户行为、注册和管理员审计记录以追加方式写入。`source_port` 仅在 TCP 对端与解析出的客户端 IP 一致、且 TCP 对端不属于 `TRUSTED_PROXIES` 时记录；经反向代理转发或可信代理未提供转发客户端 IP 时，用户侧源端口无法确认，应保留为 `NULL`，不得误记代理到源站的连接端口。其他不可获取的网络端口、服务目标地址和设备指纹字段也保留为 `NULL`；设备标识只允许存 SHA-256 摘要，不写入原始值。

### 1.1 用户行为日志 `d2s_user_action_logs`

| 字段 | 类型/规则 |
| --- | --- |
| id | UUID 或稳定唯一 ID，主键 |
| user_id | 用户 ID；无法识别账户的失败尝试允许为 NULL |
| email | 事件时邮箱快照；未知账户登录尝试保存规范化输入邮箱，否则 NULL |
| action_type | 仅允许 `REGISTER`、`LOGIN`、`LOGOUT`、`PURCHASE`、`DOWNLOAD`、`PASSWORD_RESET`、`PROFILE_UPDATE` |
| event_time | UTC Unix 秒 |
| source_ip / source_port | 客户端来源 IP / 端口；来源端口无法可靠获取时 NULL |
| destination_ip / destination_port | 接收请求的服务监听地址 / 端口；反向代理无法提供真实目标时 NULL |
| user_agent | UTF-8，最多 512 字节，截断时保持有效 UTF-8 |
| device_fingerprint | 可获得时为 64 位小写 SHA-256；不可得为 NULL |
| request_path | 路径模板或规范化路径，最多 2048 字节，不保存 query 中的验证码、token 或密码 |
| result_status | 仅允许 `SUCCESS`、`FAILURE`、`BLOCKED` |
| record_hash | 64 位小写 SHA-256，非空 |

哈希输入格式固定为 UTF-8：

`d2s-user-action-v1\0{user_id}\0{action_type}\0{event_time}\0{source_ip}\0{result_status}`

空值使用空字符串。验证时以数据库内原字段重算并比较 `record_hash`；接口返回布尔结果，不返回原始哈希材料。哈希不是对数据库所有者的签名保证，数据库权限和审计备份仍需独立控制。

登录成功、失败、验证码拒绝、临时锁定、注册、退出、购买、密码重置、资料修改以及可追踪的程序下载均应记录。失败登录无法关联用户时允许 `user_id=NULL`。事件本身写入失败时，敏感动作必须按安全策略拒绝或进入可靠重试队列，不能静默丢弃。

### 1.2 注册快照 `d2s_registration_records`

字段：`id`、`user_id`（唯一）、`email`、`register_ip`、`register_time`、`register_user_agent`、`email_verified`、`email_verified_at`。邮箱验证未完成前不得创建可登录/购买的激活账户；成功验证后，用户激活状态、注册快照和成功的 `REGISTER` 行为日志在同一数据库事务中写入，任一写入失败则整体回滚。邮箱改动不得改写注册时快照。

### 1.3 管理员操作审计 `d2s_admin_operation_audits`

字段：`id`、`operator_id`、`operator_email`、`operation_type`、`operation_time`、`target_resource`、`source_ip`、`result`、`details_json`。至少记录管理员登录、查看个人行为日志、两类导出、修改用户、证件核验、账户紧急开关、批量操作、清理和权限变更。操作失败和拒绝也必须记录；`details_json` 仅保存必要参数、时间范围、影响数量和失败码，不存密码、验证码、支付凭据或证件明文。

### 1.4 购买留痕

复用并扩展 `d2s_orders`，不得产生第二套可与支付账本分叉的订单来源。补充订单时邮箱快照、购买/完成时间、来源 IP 和支付账户标识哈希。既有字段映射：`order_id=id`、`product_id=product`、`amount=amount_minor`、`currency=currency`、`payment_method=provider`、`result_status=status`。支付标识只存脱敏显示值或带密钥的摘要；不保存完整银行卡号、PayPal 邮箱或其他支付凭据。

### 1.5 可选身份核验与账户控制

`d2s_identity_verifications`：`user_id`、`id_document_type`（`PASSPORT`/`FOREIGN_PERMANENT_RESIDENT_ID`/`OTHER`）、`id_document_number_hash`、`id_document_number_hash_version`、`id_document_country`、`id_verification_status`（`NOT_REQUIRED`/`PENDING`/`VERIFIED`/`REJECTED`）、`id_verified_at`、`id_verified_by`。注册时不要求填写；证件号码不存明文。号码摘要采用 `d2s-identity-document-v1` 域分离的 HMAC-SHA-256，独立密钥由 `D2S_ID_DOCUMENT_HMAC_KEY` 提供且为 32 字节随机值（64 位十六进制）；密钥缺失或无效时拒绝写入。旧版未标记算法版本的普通 SHA-256 摘要不得用于 `VERIFIED` 状态；后台读取时会在审计事务内将旧记录降为 `PENDING` 并清除旧摘要，需重新提交号码后才能核验。提交、审核和状态变更均写管理员审计。

`d2s_user_security_controls`：`user_id`、`login_blocked`、`purchase_blocked`、`reason`、`updated_at`、`updated_by`。购买限制在授权订单、Checkout 和购买权益发放入口服务端生效。强制下线必须撤销所有现存会话或递增用户认证版本，使已签发凭据失效。

## 2. 追加写入、权限和留存

- 应用运行身份对审计表只授予 INSERT/SELECT；不得授予 UPDATE、DELETE、TRUNCATE 或列级 UPDATE。PostgreSQL 权限脚本按有效权限检查表级/列级授权，并验证清理身份不持有范围外写权限。应用模型和路由不得提供日志编辑、删除或清空 API。
- PostgreSQL/MySQL 审计表建表与升级使用独立 `D2S_AUDIT_MIGRATION_DSN`，目标须与 `SQL_DSN` 的数据库和 schema 一致，再执行 `go run ./cmd/d2s-audit-migrate`；主服务运行时不迁移这四张审计表，只检查其存在及留存审计必需的 `result` 字段；字段缺失时启动失败并提示先运行独立迁移。SQLite 开发模式继续由主服务自动迁移。清理必须通过独立的 `D2S_AUDIT_CLEANUP_DSN` 连接执行；未配置时接口拒绝清理。维护身份只授予三张留存表的 SELECT/DELETE 和 `d2s_audit_retention_runs` 的 SELECT/INSERT。清理成功时审计记录与限定范围删除在同一事务提交；任一步失败时该事务回滚，随后用新事务追加 `result=FAILURE` 记录、失败阶段、范围、实际删除数量及候选记录统计数。若清理审计写入失败，管理 API 会尝试在主应用数据库追加失败管理审计；只有该审计也失败时才返回 `AUDIT_WRITE_FAILED`。SQLite 开发模式不具备表级权限隔离。
- 默认不自动清理。若启用清理，最短保留天数在代码中固定为 180；拒绝任何小于 180 的请求或配置。最大值限制为有符号数据库整数可表示的天数，避免秒数换算溢出。清理不得触及未到期数据；清理审计记录不得由该清理任务删除。后台不提供一键清空或删除日志表入口。
- 行为日志、注册快照、后台操作审计和清理审计默认留存至少 180 天。不能将 ClickHouse TTL 或通用日志配置直接套用到这些表。
- MySQL/PostgreSQL 使用专用数据库身份和表授权落实运行账户权限。SQLite 没有表级用户/权限系统；SQLite 环境只能依靠独立文件访问控制、追加写代码路径及可选防变更触发器，不能声称满足数据库用户级 INSERT/SELECT 授权。
- 定期将审计数据复制到独立只追加或对象锁定的备份介质；备份校验和哈希检查结果纳入运维记录。

## 3. 邮箱与登录安全

- 注册必须绑定邮箱验证码；验证码有效期最多 15 分钟，同一邮箱及同一来源 IP 分别限速。验证成功前账户不得登录、创建订单、发起 Checkout 或领取购买权益。
- 所有密码、OAuth、Passkey、2FA 登录成功与失败都记录 `LOGIN`；账户锁定或验证码挑战按结果写 `BLOCKED`。身份未知的请求仍写失败日志并避免泄露账户是否存在。
- 连续失败达到服务端固定阈值后临时锁定或要求加强验证码；建议默认 5 次失败/15 分钟窗口并锁定 15 分钟。锁定、解除和每次拒绝均记录行为日志。
- 邮箱验证配置不得关闭此业务站点的注册验证要求；验证码发送沿用 Turnstile/行为验证码及现有邮件服务，并附加邮箱与来源 IP 双维度限速。

## 4. 独立后台身份和角色

管理员账户、密码验证、会话、失败计数及角色独立于前台 `users` 登录；前台用户即使拥有产品管理员标记，也不能通过后台专用登录接口认证。首个超级管理员只能经一次性 CLI/受控部署初始化创建，不提供公开注册接口。

后台密码使用现有安全密码哈希机制；新密码至少 12 个字符并执行强度校验。连续 5 次失败登录在 15 分钟窗口内触发至少 15 分钟锁定；窗口过期、锁定结束或成功登录后清零失败计数。后台登录、失败、锁定和解锁均写管理员审计。会话使用安全 HttpOnly Cookie 或等效服务端可撤销凭据，退出和紧急撤销立即失效。

角色：

- `super_admin`：全部管理权限，包括角色管理、紧急措施和受控清理发起。
- `audit_reader`：查询全部审计字段、校验哈希、导出行为日志和注册快照。
- `account_operator`：用户资料和账户限制操作。
- `identity_reviewer`：触发式身份核验的读取与审核。
- 非授权角色默认拒绝。授权查看日志的角色看到全部字段；不得因为不是超级管理员而隐去本规格要求的列。

所有管理页面/API 默认要求独立管理员身份。敏感读操作（日志查看、导出）和所有写操作均记录操作者、时间、来源 IP、目标和结果。接口需按角色进行服务端授权，前端隐藏入口不能替代后端校验。

## 5. 查询、导出与核验 API

所有分页接口限制最大页长 500，使用参数化查询；用户行为日志采用稳定排序 `event_time DESC, id DESC`。

| 方法与路径 | 权限 | 契约 |
| --- | --- | --- |
| `POST /api/v1/admin/auth/login` | 未登录 | 独立管理员账号登录；记录成功/失败/锁定 |
| `POST /api/v1/admin/auth/logout` | 管理员 | 撤销当前管理会话 |
| `GET /api/v1/admin/audit/user-events` | `audit_reader` | 按 `start_time`、`end_time`、`user_id`、`email`、`source_ip`、`action_type` 筛选；返回全部日志字段和分页信息 |
| `POST /api/v1/admin/audit/user-events/:id/verify-hash` | `audit_reader` | 重算哈希，返回 `consistent` 布尔值和日志 ID |
| `GET /api/v1/admin/audit/user-events/export.csv` | `audit_reader` | 按同等筛选导出全部日志字段，UTF-8 CSV，文件名 `access_log_YYYYMMDD_HHMMSS.csv` |
| `GET /api/v1/admin/audit/registrations` | `audit_reader` | 分页查询注册快照和时间过滤 |
| `GET /api/v1/admin/audit/registrations/export.csv` | `audit_reader` | 按时间范围导出邮箱、注册 IP/时间、邮箱验证状态/时间；UTF-8 CSV |

两类 CSV 导出在设置响应头和发送内容前，必须追加 `STARTED` 审计记录，并包含筛选范围与预估记录数；完成 CSV 写入和 flush 后再追加 `SUCCESS`。标题、数据写入或 flush 失败时追加 `FAILURE`。如果只存在 `STARTED` 而没有终态记录，应按未完成导出处理；审计写入 `STARTED` 失败时不得发送 CSV 内容。

| `GET /api/v1/admin/purchases` | `account_operator` 或更高 | 分页查询购买记录；支付标识仅显示脱敏值/摘要 |
| `PUT /api/v1/admin/users/:id/emergency` | `account_operator` 或更高 | 设置 `login_blocked`、`purchase_blocked`、`force_logout`；强制下线先记 `STARTED`，完成所有会话撤销、认证版本提升及在线租约释放后追加 `SUCCESS`，失败追加阶段性 `FAILURE` |
| `POST /api/v1/admin/users/batch/preview` | `account_operator` 或更高 | 按注册 IP CIDR、邮箱域名、注册时间窗口筛选，返回命中数及短时有效预览 ID |
| `POST /api/v1/admin/users/batch/apply` | `account_operator` 或更高 | 要求预览 ID、明确二次确认和操作项；服务端重算影响集并审计筛选参数、数量和结果 |
| `GET /api/v1/admin/users/:id/identity` | `identity_reviewer` 或更高 | 读取可选核验状态及哈希，不返回证件明文 |
| `PUT /api/v1/admin/users/:id/identity` | `identity_reviewer` 或更高 | 创建/审核触发式核验；身份状态和成功审计在同一事务提交，证件号仅保存带独立密钥的域分离 HMAC-SHA-256 摘要 |
| `POST /api/v1/admin/audit/retention/execute` | `super_admin` | 仅可调用受控清理；请求保留期小于 180 天时拒绝；成功清理提交范围与数量审计，失败清理在事务回滚后追加 `FAILURE` 记录；两类审计都无法写入时返回 `AUDIT_WRITE_FAILED` |
| `GET /api/v1/admin/reconciliation` | `super_admin` 后台会话，或专用机器令牌 | 汇总指定 UTC 时间窗内支付与订单差异，不修改账本；每次成功或失败均追加 `RUN_RECONCILIATION` 审计，记录时间窗、结果及差异数 |

机器令牌只用于 `GET /api/v1/admin/reconciliation`，从 `D2S_RECONCILIATION_TOKEN` 读取 32 字节随机值的 64 位十六进制编码，并以恒定时间比较。令牌需分别安全注入应用和 root-only systemd 环境文件；其它后台 API 仍只接受管理员 HttpOnly 会话。未配置或格式错误时对账接口返回服务不可用，令牌不能复用管理员密码。`pending_expired` 统计所有在报告截止时间前已到期、仍处于 pending 的订单，不受创建时间窗口限制；该差异会在订单进入终态前持续出现在报告中。订单与支付事件从同一只读事务快照读取，MySQL/PostgreSQL 显式请求 `REPEATABLE READ`；对账接口不自动更改订单状态。

用户下载跟踪接口为 `POST /api/v1/downloads/:target/track`，需要前台登录会话；`target` 仅接受 `release`、`quark`、`baidu`。服务端检查邮箱验证及登录限制，成功追加 `DOWNLOAD/SUCCESS` 后返回 `logged: true`。该记录表示链接点击，不代表外部文件传输完成；匿名公开直链不经过该接口。

导出审计必须写入 `EXPORT_LOG`，记录过滤器摘要、行数、文件名、开始/结束时间和结果。日志查看写 `VIEW_USER_LOG` 审计。CSV 使用标准 quoting；导出时对以 `=`、`+`、`-`、`@` 开头的用户可控字段添加单引号防公式注入，数据库原值保持不变。

独立管理页面使用路由 `/d2s-admin-console`，通过后台 HttpOnly 会话访问上述 API。界面包含管理员登录、全字段日志查询和 CSV 导出、哈希校验、注册快照查询/导出、购买记录、账户限制和批量预览/确认、身份核验及 180 天以上的受控清理入口。页面权限仅用于显示入口，服务端角色检查仍为最终授权边界。

## 6. 购买关联与服务端执行点

- 行为事件在服务端关键处理点写入；不得依赖浏览器提供的结果状态。
- 下单时快照 `user_id`、`email`、UTC 时间、商品、分币金额、币种、支付渠道、来源 IP 和结果状态。Stripe、Creem、Waffo Pancake Checkout 使用订单邮箱快照，避免账户邮箱变更造成同一订单幂等重试参数变化；Creem 的用户标识元数据使用稳定 user ID。未知的服务目标/来源端口及设备摘要列保留并置空。
- 支付渠道回调只更新订单业务状态，不得修改已完成事件日志。Stripe 和 Creem 的已验签支付成功回调优先取 `customer` ID，缺失时取客户邮箱；Waffo Pancake 的已验签 `order.completed` 回调取 `buyerEmail`。邮箱统一转小写。只将 `d2s-payment-account-v1` 域分离及 `common.SessionSecret` 派生密钥生成的 HMAC-SHA-256 摘要写入订单，不保存原始标识；缺失时为 NULL。模型仅允许 Stripe、Creem 和 Waffo Pancake 接受该摘要；其余渠道在稳定付款方标识契约确认前留空。Creem 字段见 [Webhook 文档](https://docs.creem.io/skills/creem-api/WEBHOOKS)，Waffo Pancake 字段见 [Go SDK API](https://pkg.go.dev/github.com/waffo-com/waffo-pancake-sdk-go)。
- 登录限制由登录、刷新/会话、设备授权及适用的 OAuth/Passkey 入口统一执行；购买限制在报价/建单、Checkout、支付完成权益发放等服务端入口统一执行。不得只在 React 页面隐藏按钮。
- 登录态首页的 GitHub Release、夸克和百度网盘入口先调用服务端白名单跟踪接口；服务端检查账户登录限制及邮箱验证，成功追加 `DOWNLOAD/SUCCESS` 后客户端才跳转。该事件表示链接点击已记录，不表示外部文件传输完成。匿名访问继续使用公开直链，无法关联账户；当前浏览器/服务端契约未提供设备指纹，因此设备摘要留空，端口和目标地址等不可取得字段留空。

## 7. 实现验收门槛

实施计划逐条标记完成状态。至少验证：新库创建、最新版代表库升级、重复迁移、SQLite/MySQL/PostgreSQL 兼容；追加写和数据库权限；180 天边界和先审计后清理；登录/注册/购买门禁及限速；管理员独立会话与角色拒绝；查询分页、全字段 CSV、哈希不一致检测；单用户和批量紧急限制在服务端生效；订单对用户快照的关联；管理员操作审计全链路。未完成的数据库、环境凭据或端到端验收必须明确列为未验证，不得标记生产就绪。

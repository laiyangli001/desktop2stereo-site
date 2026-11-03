# Desktop2Stereo 后台账户审计与治理规格

版本：1.6

状态：接口与实现规格；独立管理员后端和 `/d2s-admin-console` 管理页面已落地，前端 `tsgo -b` 与 Rsbuild 生产构建已通过；MySQL 8.4.9/PostgreSQL 15.14 隔离库权限脚本执行通过，应用账号 INSERT/SELECT 成功，UPDATE/DELETE/TRUNCATE 被拒绝；PostgreSQL 单列 UPDATE 注入后由权限脚本撤销，清理权限范围也经断言。2026-11-02 又在隔离 MySQL 8.4.13/PostgreSQL 17.11 上通过最新工作树完整 `go test -p 1 ./... -count=1`、重复 `AutoMigrate`、工单数据库矩阵及关键并发矩阵。另以最近已发布版本 `26f524d11424a2866a8608a4883a63dd1f7fe16d` 构建两种数据库的代表性旧 schema，并完成旧版迁移、审计独立迁移和当前 `model.InitDB()` 的重复升级；用户、授权和订单数据保留，订单购买时间回填、审计/工单新表、订单快照列和关键唯一索引均通过检查。该夹具不是生产数据备份。对账专用机器令牌和每次对账审计已在本地实现并通过 Go 测试，尚未部署生产；生产部署仓库与本地基线不同且工作区有既有改动，因此定时器保持禁用。生产权限和端到端验收仍待完成，具体状态以 `docs/01-cross-platform-licensing-server-implementation-plan.md` 为准。

2026-11-02 扩展隔离数据库矩阵，实测 MySQL 8.4.13 与 PostgreSQL 17.11 上单账户紧急锁定/解除、登录/购买限制读取及对应操作审计记录；矩阵、模型包全套测试、`go vet` 和 `go build` 均通过，测试实例已停止。该结果不替代生产账号权限及生产环境验收。

2026-11-03 增加 CSV 导出双人审批与离线管理员开通命令。全量 Go 测试、审批/导出专项回归、`go vet`、`go build`、前端 Vitest 76 文件/476 项、`tsgo -b` 和 Rsbuild 构建通过。审批前端拒绝同邮箱管理员执行自批操作，服务端仍按管理员 ID 校验；精确筛选范围、30 分钟期限、一次性消费和审批 ID 审计关联已覆盖。本轮没有配置 MySQL/PostgreSQL DSN，也未执行生产数据库或生产账号验收。

2026-11-03 补齐高权限财务/授权后台的审计缺口：订单、支付事件、余额、提现、授权、解绑申请和签名公钥的查看记录筛选条件与结果条数；审计写入失败时敏感列表接口不返回数据；提现审核、人工解绑审核、用户区域修改和签名密钥退役将成功审计与业务变更放入同一事务，失败尝试单独追加失败审计。SQLite controller/model 回归通过；当前环境未配置 MySQL/PostgreSQL 测试 DSN，也未运行数据库实例，跨数据库矩阵和生产验收未验证。2026-11-03 又将提现及人工解绑列表的审计失败响应统一为 `AUDIT_WRITE_FAILED`，新增 fail-closed 回归通过固定路径 controller 测试套件；Waffo 掩码卡标识的 HMAC 类型也已补齐并由 Waffo 专项回归覆盖。随后重跑的 `go vet -p 1 ./controller ./model ./service`、`go build -p 1 ./controller ./model ./service` 均通过。

2026-11-03 修复批量紧急处置失败审计操作类型：失败的解锁操作此前会被记录为 `EMERGENCY_LOCK`。新增 selection-changed 失败回归先验证错误可复现；现根据管理员确认的预览动作写入 `EMERGENCY_LOCK` 或 `EMERGENCY_UNLOCK`。回归通过，固定路径 controller 全包测试及 39 个含测试包的全量 Go 测试、`go vet -p 1 ./controller`、`go build -p 1 ./controller` 和 `git diff --check` 通过。

本规格面向程序介绍、在线购买、下载、账户登录和授权管理；网站不提供用户公开内容发布、评论、即时通讯或群组功能，账户以邮箱识别并面向全球用户。负责人报告运营主体所在地为山东烟台、只销售自有 Desktop2Stereo 软件、不接受第三方或用户上架；“软件开发”是公司工商经营范围，ICP 备案服务类别目前不清楚；网站不托管安装包，只跳转至 GitHub、夸克网盘或百度网盘等外部下载地址。上述为负责人提供的信息，主体登记、备案材料和线上配置尚未独立核验。邮箱验证码仅证明邮箱控制权，不证明真实身份。当前无证件核验页面或 API 路由，不收集证件信息，也不宣称已实名。除《网络安全法》第二十六条及《互联网用户账号信息管理规定》第九条对特定服务功能的实名要求外，现行《反电信网络诈骗法》第二十一条第（四）项还列明“信息、软件发布服务”及“网络交易”服务。结合本站在线销售、支付和软件下载，实名义务是否适用于本站及适用用户范围尚待主管部门或合格法律顾问结合运营主体、实际许可/备案、销售/分发模式书面确认；外部托管下载本身不能排除在线交易相关规定的适用，无 UGC、无即时通讯和仅邮箱注册均不能单独构成豁免依据。确认前不得宣称邮箱验证满足实名义务，也不得无明确依据擅自采集证件。若确认需要实名，应先确定境外用户可用的合规核验方式、处理依据、最少字段、告知、安全和留存要求，再实施服务端核验门禁。当前提现模型还保存用户提交的支付宝账号和真实姓名，且字段为明文；这属于提现资料，不等同证件核验，隐私政策及生产必要性/保护评估应如实涵盖。详细条文和官方来源见 `SEC-001-网站安全管理制度.md`。

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

字段：`id`、`operator_id`、`operator_email`、`operation_type`、`operation_time`、`target_resource`、`source_ip`、`result`、`details_json`。至少记录管理员登录、查看个人行为日志、两类导出、修改用户、账户紧急开关、批量操作、清理和权限变更。操作失败和拒绝也必须记录；`details_json` 仅保存必要参数、时间范围、影响数量和失败码，不存密码、验证码或支付凭据。

高权限财务和授权接口另记录 `VIEW_ADMIN_ORDERS`、`VIEW_PAYMENT_EVENTS`、`VIEW_BALANCES`、`VIEW_WITHDRAWALS`、`VIEW_LICENSES`、`VIEW_UNBIND_REQUESTS`、`VIEW_SIGNING_KEYS`、`REVIEW_WITHDRAWAL`、`REVIEW_MANUAL_UNBIND`、`SET_USER_REGION` 和 `RETIRE_SIGNING_KEY`。成功审核/变更和成功审计须原子提交；业务失败需追加 `FAILURE`，不记录审核备注原文或支付账户资料。

### 1.4 购买留痕

复用并扩展 `d2s_orders`，不得产生第二套可与支付账本分叉的订单来源。补充订单时邮箱快照、购买/完成时间、来源 IP 和支付账户标识哈希。既有字段映射：`order_id=id`、`product_id=product`、`amount=amount_minor`、`currency=currency`、`payment_method=provider`、`result_status=status`。支付标识只存脱敏显示值或带密钥的摘要；不保存完整银行卡号、PayPal 邮箱或其他支付凭据。Waffo 的已验签支付通知若包含 `paymentInfo.payMethodResponse.maskCardData`，仅保存其 HMAC 摘要；该字段缺失或不能解析时留空。不得把本站传入 Waffo 的账户邮箱误记为付款方独立标识。字段依据 [Waffo Go SDK v1.3.2 webhook payload 示例](https://github.com/waffo-com/waffo-go/blob/v1.3.2/README.md)。

### 1.5 身份信息边界与账户控制

`D2SIdentityVerification` 类型仅为兼容既有数据库保留，不纳入新库自动迁移；后台身份录入页面与读取/写入 API 未注册路由，不在当前产品流程中收集、审核、导出或使用证件号码。既有 `d2s_identity_verifications` 表及历史记录不会因本次调整而删除；若存在历史数据，须先按合法目的、访问权限和保存期限单独盘点。重新启用前须重新评估适用义务、合法性基础、必要性、告知方式和安全控制。邮箱验证不得映射为 `VERIFIED`。

`d2s_user_security_controls`：`user_id`、`login_blocked`、`purchase_blocked`、`reason`、`updated_at`、`updated_by`。购买限制在授权订单、Checkout 和购买权益发放入口服务端生效。强制下线必须撤销所有现存会话或递增用户认证版本，使已签发凭据失效。

### 1.6 服务投诉与咨询工单 `d2s_support_tickets`

登录用户通过 `/support` 表单提交付款未到账、订单/授权、下载、软件使用、账户及其他问题。记录工单 ID、必填账户 ID、分类、联系邮箱、可选订单号、标题、用户描述、来源 IP、User-Agent、状态、后台对用户的回复（`admin_reply`）、创建/更新时间及处理管理员。前端路由和服务端提交 API 均要求已注册并登录；访客不能创建工单。`GET /api/v1/support/my-tickets` 按服务端会话中的 user ID 查询，返回工单状态和后台回复，不返回联系邮箱、IP、User-Agent 或经办管理员身份；当前没有独立内部备注字段，管理员填写的回复会展示给该工单提交者。页面提示不要提交密码、验证码或完整支付卡信息。工单不属于行为日志表，不参与日志清理；处理完成后按个人信息最短必要期限评估后续保存或处置，不作无限期保留承诺。

## 2. 追加写入、权限和留存

- 应用运行身份对审计表只授予 INSERT/SELECT；不得授予 UPDATE、DELETE、TRUNCATE 或列级 UPDATE。PostgreSQL 权限脚本按有效权限检查表级/列级授权，并验证清理身份不持有范围外写权限。应用模型和路由不得提供日志编辑、删除或清空 API。
- PostgreSQL/MySQL 审计表建表与升级使用独立 `D2S_AUDIT_MIGRATION_DSN`，目标须与 `SQL_DSN` 的数据库和 schema 一致，再执行 `go run ./cmd/d2s-audit-migrate`；主服务运行时不迁移这四张审计表，只检查其存在及留存审计必需的 `result` 字段；字段缺失时启动失败并提示先运行独立迁移。SQLite 开发模式继续由主服务自动迁移。清理必须通过独立的 `D2S_AUDIT_CLEANUP_DSN` 连接执行；未配置时接口拒绝清理。维护身份只授予三张留存表的 SELECT/DELETE 和 `d2s_audit_retention_runs` 的 SELECT/INSERT。清理成功时审计记录与限定范围删除在同一事务提交；任一步失败时该事务回滚，随后用新事务追加 `result=FAILURE` 记录、失败阶段、范围、实际删除数量及候选记录统计数。若清理审计写入失败，管理 API 会尝试在主应用数据库追加失败管理审计；只有该审计也失败时才返回 `AUDIT_WRITE_FAILED`。SQLite 开发模式不具备表级权限隔离。
- 默认不自动清理。若启用清理，行为日志、注册快照和管理员操作审计的最短保留天数在服务端硬编码为 366，拒绝任何小于 366 的请求。用户行为日志还保留六个日历月基线保护逻辑。最大值限制为有符号数据库整数可表示的天数，避免秒数换算溢出。清理不得触及未到期数据；清理审计记录不得由该清理任务删除。后台不提供一键清空或删除日志表入口。
- 行为日志、注册快照和管理员操作审计默认至少留存 366 天；清理审计记录不得由该任务删除。该策略仅适用于本规格列明的安全审计数据，工单正文和支付数据按各自目的评估，不因本策略自动延长。每次清理审计须分别记录各表截止时间、时间范围和删除数量。不能将 ClickHouse TTL 或通用日志配置直接套用到这些表。
- MySQL/PostgreSQL 使用专用数据库身份和表授权落实运行账户权限。SQLite 没有表级用户/权限系统；SQLite 环境只能依靠独立文件访问控制、追加写代码路径及可选防变更触发器，不能声称满足数据库用户级 INSERT/SELECT 授权。
- 定期将审计数据复制到独立只追加或对象锁定的备份介质；备份校验和哈希检查结果纳入运维记录。

## 3. 邮箱与登录安全

- 注册必须绑定邮箱验证码；验证码有效期最多 15 分钟，同一邮箱及同一来源 IP 分别限速。验证成功前账户不得登录、创建订单、发起 Checkout 或领取购买权益。
- 所有密码、OAuth、Passkey、2FA 登录成功与失败都记录 `LOGIN`；账户锁定或验证码挑战按结果写 `BLOCKED`。身份未知的请求仍写失败日志并避免泄露账户是否存在。
- 连续失败达到服务端固定阈值后临时锁定或要求加强验证码；建议默认 5 次失败/15 分钟窗口并锁定 15 分钟。锁定、解除和每次拒绝均记录行为日志。
- 邮箱验证配置不得关闭此业务站点的注册验证要求；验证码发送沿用 Turnstile/行为验证码及现有邮件服务，并附加邮箱与来源 IP 双维度限速。

## 4. 独立后台身份和角色

管理员账户、密码验证、会话、失败计数及角色独立于前台 `users` 登录；前台用户即使拥有产品管理员标记，也不能通过后台专用登录接口认证。首个超级管理员只能经一次性受控部署初始化创建，不提供公开注册接口。后续账号只能使用 `go run ./cmd/d2s-admin-provision` 离线建立；该命令要求现有启用中的超级管理员邮箱和密码验证，将账号创建和 `CREATE_ADMIN_ACCOUNT` 审计放入同一事务。需双人审批时，至少配置两名各自独立的超级管理员。

命令从环境变量读取 `SQL_DSN`、`D2S_ADMIN_PROVISION_OPERATOR_EMAIL`、`D2S_ADMIN_PROVISION_OPERATOR_PASSWORD`、`D2S_ADMIN_PROVISION_EMAIL`、`D2S_ADMIN_PROVISION_PASSWORD` 和可选 `D2S_ADMIN_PROVISION_ROLE`（默认 `audit_reader`）。密码至少 12 个字符并满足至少三类字符。执行后应在同一 shell 立即清除这组变量；不得将真实凭据写入仓库、脚本、命令行参数或日志。

后台密码使用现有安全密码哈希机制；新密码至少 12 个字符并执行强度校验。连续 5 次失败登录在 15 分钟窗口内触发至少 15 分钟锁定；窗口过期、锁定结束或成功登录后清零失败计数。后台登录、失败、锁定和解锁均写管理员审计。会话使用安全 HttpOnly Cookie 或等效服务端可撤销凭据，退出和紧急撤销立即失效。

角色：

- `super_admin`：全部管理权限，包括角色管理、紧急措施和受控清理发起。
- `audit_reader`：查询全部审计字段、校验哈希、导出行为日志和注册快照。
- `account_operator`：用户资料和账户限制操作。
- 非授权角色默认拒绝。授权查看日志的角色看到全部字段；不得因为不是超级管理员而隐去本规格要求的列。

导出审批是可变的工作流状态，保存在 `d2s_admin_export_approvals`；该表不是日志/注册/管理员审计表。审批状态只允许按条件从 `PENDING` 转为 `APPROVED`/`REJECTED`，批准后按条件一次性转为 `CONSUMED`，到期申请在读取/操作时视为过期。所有状态转移必须与对应的只追加管理员审计事务原子提交。

所有管理页面/API 默认要求独立管理员身份。敏感读操作（日志查看、导出）和所有写操作均记录操作者、时间、来源 IP、目标和结果。接口需按角色进行服务端授权，前端隐藏入口不能替代后端校验。

## 5. 查询、导出与核验 API

所有分页接口限制最大页长 500，使用参数化查询；用户行为日志采用稳定排序 `event_time DESC, id DESC`。

| 方法与路径 | 权限 | 契约 |
| --- | --- | --- |
| `POST /api/v1/admin/auth/login` | 未登录 | 独立管理员账号登录；记录成功/失败/锁定 |
| `POST /api/v1/admin/auth/logout` | 管理员 | 撤销当前管理会话 |
| `GET /api/v1/admin/audit/user-events` | `audit_reader` | 按 `start_time`、`end_time`、`user_id`、`email`、`source_ip`、`action_type` 筛选；返回全部日志字段和分页信息 |
| `POST /api/v1/admin/audit/user-events/:id/verify-hash` | `audit_reader` | 重算哈希，返回 `consistent` 布尔值和日志 ID |
| `GET /api/v1/admin/audit/user-events/export.csv` | `audit_reader` | 必须带有效的一次性 `approval_id`，按审批绑定的筛选导出全部日志字段，UTF-8 CSV，文件名 `access_log_YYYYMMDD_HHMMSS.csv` |
| `GET /api/v1/admin/audit/registrations` | `audit_reader` | 分页查询注册快照和时间过滤 |
| `GET /api/v1/admin/audit/registrations/export.csv` | `audit_reader` | 必须带有效的一次性 `approval_id`，按审批绑定的时间范围导出邮箱、注册 IP/时间、邮箱验证状态/时间；UTF-8 CSV |
| `POST /api/v1/admin/audit/export-approvals?export_type=...&<filters>` | `audit_reader` 或更高 | 以 `reason` 说明用途；申请记录精确规范化的导出类型及筛选快照，30 分钟失效 |
| `GET /api/v1/admin/audit/export-approvals/:id` | `audit_reader` 或更高 | 仅申请人或超级管理员可查看审批状态 |
| `GET /api/v1/admin/audit/export-approvals` | `super_admin` | 分页列出待审批且未过期申请 |
| `PUT /api/v1/admin/audit/export-approvals/:id` | `super_admin` | `decision=APPROVED|REJECTED` 且必须填写审查理由；申请人不得自批，审批仅可决定一次 |

两类 CSV 导出在设置响应头和发送内容前，必须追加 `STARTED` 审计记录，并包含筛选范围与预估记录数；完成 CSV 写入和 flush 后再追加 `SUCCESS`。标题、数据写入或 flush 失败时追加 `FAILURE`。如果只存在 `STARTED` 而没有终态记录，应按未完成导出处理；审计写入 `STARTED` 失败时不得发送 CSV 内容。

两类导出均须先完成双人审批。审批哈希绑定导出类型及规范化后的全部筛选条件，下载时重新计算并精确比较；过滤范围变化、申请过期、未批准、已消费或申请人与下载人不一致时拒绝导出。审批在下载前以事务原子消费，消费审计先落库；导出 `STARTED` 记录包含审批 ID，以便将申请、决定、消费和导出终态审计串联。前端只在审批状态为 `APPROVED` 时显示下载操作，服务端仍独立强制校验。

| `GET /api/v1/admin/purchases` | `account_operator` 或更高 | 分页查询购买记录；成功审计记录时间范围、user_id、规范化邮箱/状态、分页参数和总数，失败审计记录失败类别；支付标识仅显示脱敏值/摘要 |
| `GET /api/v1/admin/licenses` | `super_admin` | 查询授权及账户区域；记录筛选条件和返回数量 |
| `GET /api/v1/admin/orders` | `super_admin` | 查询订单；记录状态/用户筛选条件和返回数量 |
| `GET /api/v1/admin/payment-events` | `super_admin` | 查询渠道支付事件；记录渠道/订单/事件筛选条件和返回数量 |
| `GET /api/v1/admin/balances` | `super_admin` | 查询账户余额；记录用户过滤、是否仅负余额及返回数量 |
| `GET /api/v1/admin/withdrawals` | `super_admin` | 查看提现申请及收款资料；记录状态过滤与返回数量 |
| `PUT /api/v1/admin/withdrawals/:id` | `super_admin` | 审核提现；余额释放/结算、申请状态和成功审计同事务提交；失败单独审计 |
| `GET /api/v1/admin/unbind-requests` | `super_admin` | 查看人工解绑申请；记录状态过滤与返回数量 |
| `PUT /api/v1/admin/unbind-requests/:id` | `super_admin` | 审核人工解绑；申请、授权变更和成功审计同事务提交；失败单独审计 |
| `PUT /api/v1/admin/users/:id/region` | `super_admin` | 修改账户区域；区域变更和成功审计同事务提交；失败单独审计 |
| `GET /api/v1/admin/signing-keys` | `super_admin` | 查看签名公钥状态；记录返回数量 |
| `PUT /api/v1/admin/signing-keys/:id` | `super_admin` | 退役非当前签名密钥；密钥状态和成功审计同事务提交；失败单独审计 |
| `GET /api/v1/admin/support/tickets` | `account_operator` 或更高 | 按 `OPEN`、`IN_PROGRESS`、`RESOLVED`、`CLOSED` 状态筛选并分页查看投诉与服务咨询；非法状态返回 `400 INVALID_STATUS` 并写 `VIEW_SUPPORT_TICKETS/FAILURE` 审计，查询失败返回 500 并审计 |
| `PUT /api/v1/admin/support/tickets/:id` | `account_operator` 或更高 | 更新处理状态及内部回复；状态变更和管理员审计在同一数据库事务提交 |
| `PUT /api/v1/admin/users/:id/emergency` | `account_operator` 或更高 | 设置 `login_blocked`、`purchase_blocked`、`force_logout`；强制下线先记 `STARTED`，完成所有会话撤销、认证版本提升及在线租约释放后追加 `SUCCESS`，失败追加阶段性 `FAILURE` |
| `POST /api/v1/admin/users/batch/preview` | `account_operator` 或更高 | 邮箱域名和注册时间窗口按当前用户邮箱/创建时间筛选，涵盖没有历史注册快照的账号；IP CIDR 按不可变注册快照中的注册 IP 筛选，并只命中未软删除账户。缺少注册快照时不推断 IP。返回命中数及短时有效预览 ID |
| `POST /api/v1/admin/users/batch/apply` | `account_operator` 或更高 | 要求预览 ID、明确二次确认和操作项；服务端重算影响集并审计筛选参数、数量和结果 |
| `POST /api/v1/admin/audit/retention/execute` | `super_admin` | 仅可调用受控清理；请求保留期小于 366 天时拒绝；用户行为日志、注册快照和管理员操作审计均按至少 366 天清理下限执行；清理审计逐表记录截止时间、范围和数量；事务失败后追加 `FAILURE` 记录；两类审计都无法写入时返回 `AUDIT_WRITE_FAILED` |
| `GET /api/v1/admin/reconciliation` | `super_admin` 后台会话，或专用机器令牌 | 汇总指定 UTC 时间窗内支付与订单差异，不修改账本；每次成功或失败均追加 `RUN_RECONCILIATION` 审计，记录时间窗、结果及差异数 |

用户端另提供 `GET /api/v1/support/config`（公开，只返回 HTTPS `work.weixin.qq.com/ca/` 链接；拒绝其他主机/路径、端口、凭据、查询和片段）、`POST /api/v1/support/tickets`（前台用户登录、限流并限制请求体）和 `GET /api/v1/support/my-tickets`（前台用户登录）。企业微信联系窗口默认指向运营方提供的 `https://work.weixin.qq.com/ca/cawcde40faa1d08998`，可由 `D2S_ENTERPRISE_WECHAT_URL` 覆盖。网站右侧浮动联系按钮同时提供企业微信客服和站内投诉留言入口；企业微信联系入口对访客可见，站内表单必须登录后提交。

`GET /api/v1/support/my-tickets` 要求前台账户登录，分页列出该账户提交的工单。表单 API 使用强制前台会话认证，并从服务端认证上下文写入不可由客户端覆盖的 user ID。账户列表完全依据服务端账户 ID 隔离，不按用户提交的联系邮箱授权。

机器令牌只用于 `GET /api/v1/admin/reconciliation`，从 `D2S_RECONCILIATION_TOKEN` 读取 32 字节随机值的 64 位十六进制编码，并以恒定时间比较。令牌需分别安全注入应用和 root-only systemd 环境文件；其它后台 API 仍只接受管理员 HttpOnly 会话。未配置或格式错误时对账接口返回服务不可用，令牌不能复用管理员密码。`pending_expired` 统计所有在报告截止时间前已到期、仍处于 pending 的订单，不受创建时间窗口限制；该差异会在订单进入终态前持续出现在报告中。订单与支付事件从同一只读事务快照读取，MySQL/PostgreSQL 显式请求 `REPEATABLE READ`；对账接口不自动更改订单状态。

用户下载跟踪接口为 `POST /api/v1/downloads/:target/track`，允许访客和登录用户调用；`target` 仅接受 `release`、`quark`、`baidu`。已登录账号须通过邮箱验证且未被禁止登录，成功后追加关联账户的 `DOWNLOAD/SUCCESS`。访客成功点击则追加 `user_id` 和 `email` 为空、带来源网络信息的 `DOWNLOAD/SUCCESS`；无效凭据、受限账号或日志写入失败不得跳转。该记录表示链接点击，不代表外部文件传输完成。

导出审计必须写入 `EXPORT_LOG`，记录过滤器摘要、行数、文件名、开始/结束时间和结果。日志查看写 `VIEW_USER_LOG` 审计。CSV 使用标准 quoting；导出时对以 `=`、`+`、`-`、`@` 开头的用户可控字段添加单引号防公式注入，数据库原值保持不变。

独立管理页面使用路由 `/d2s-admin-console`，通过后台 HttpOnly 会话访问上述 API。界面包含管理员登录、全字段日志查询和 CSV 导出、哈希校验、注册快照查询/导出、购买记录、账户限制和批量预览/确认，以及最低 366 天的受控清理入口。页面不提供证件信息录入或身份核验功能。页面权限仅用于显示入口，服务端角色检查仍为最终授权边界。

## 6. 购买关联与服务端执行点

- 行为事件在服务端关键处理点写入；不得依赖浏览器提供的结果状态。
- 下单时快照 `user_id`、`email`、UTC 时间、商品、分币金额、币种、支付渠道、来源 IP 和结果状态。Stripe、Creem、Waffo Pancake Checkout 使用订单邮箱快照，避免账户邮箱变更造成同一订单幂等重试参数变化；Creem 的用户标识元数据使用稳定 user ID。未知的服务目标/来源端口及设备摘要列保留并置空。
- 支付渠道回调只更新订单业务状态，不得修改已完成事件日志。Stripe 和 Creem 的已验签支付成功回调优先取 `customer` ID，缺失时取客户邮箱；Waffo Pancake 的已验签 `order.completed` 回调取 `buyerEmail`。Waffo 的已验签 `PAYMENT_NOTIFICATION` 若含 `paymentInfo.payMethodResponse.maskCardData`，仅保存该掩码卡数据的 HMAC 摘要；Waffo `userInfo.userEmail` 是本站下单时传入的账户邮箱，不作为独立付款标识。邮箱标识统一转小写。只将 `d2s-payment-account-v1` 域分离及 `common.SessionSecret` 派生密钥生成的 HMAC-SHA-256 摘要写入订单，不保存原始标识；缺失时为 NULL。模型仅允许 Stripe、Creem、Waffo 和 Waffo Pancake 接受该摘要；易支付等渠道在稳定付款方标识契约确认前留空。Creem 字段见 [Webhook 文档](https://docs.creem.io/skills/creem-api/WEBHOOKS)，Waffo Pancake 字段见 [Go SDK API](https://pkg.go.dev/github.com/waffo-com/waffo-pancake-sdk-go)，Waffo 字段见 [Go SDK v1.3.2 webhook payload 示例](https://github.com/waffo-com/waffo-go/blob/v1.3.2/README.md)。
- 登录限制由登录、刷新/会话、设备授权及适用的 OAuth/Passkey 入口统一执行；购买限制在报价/建单、Checkout、支付完成权益发放等服务端入口统一执行。不得只在 React 页面隐藏按钮。
- 首页的 GitHub Release、夸克和百度网盘入口对访客及登录用户都先调用服务端白名单跟踪接口，成功写入日志后客户端才跳转。已验证账户的事件关联 user ID 和邮箱；访客事件保留来源 IP、User-Agent 等可用网络信息但不关联账户。该事件只证明链接点击已记录，不代表外部文件传输完成；当前浏览器/服务端契约未提供设备指纹，因此设备摘要留空，端口和目标地址等不可取得字段留空。

## 7. 实现验收门槛

实施计划逐条标记完成状态。至少验证：新库创建、最新版代表库升级、重复迁移、SQLite/MySQL/PostgreSQL 兼容；追加写和数据库权限；行为日志、注册快照及管理员审计的 366 天边界和先审计后清理；登录/注册/购买门禁及限速；管理员独立会话与角色拒绝；查询分页、全字段 CSV、哈希不一致检测；单用户和批量紧急限制在服务端生效；订单对用户快照的关联；登录用户工单创建、账户隔离、后台角色控制、状态变更与审计原子性；管理员操作审计全链路。未完成的数据库、环境凭据或端到端验收必须明确列为未验证，不得标记生产就绪。

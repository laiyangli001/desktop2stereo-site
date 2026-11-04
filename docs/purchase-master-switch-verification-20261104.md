# 购买总开关实现与验证

普通系统后台的“支付网关”页顶部增加 Desktop2Stereo 购买总开关，覆盖所有 Desktop2Stereo 购买渠道。仅普通系统 root 管理员可操作，独立安全审计后台账号不能调用接口。初始沿用环境变量（默认关闭），首次保存后数据库设置优先，即时生效，无需重启。

## 实现

- GET/PUT `/api/option/d2s-purchases`；PUT 必须提供布尔字段 `enabled`。
- 复用现有 `options` 表，键为 `D2SPurchasesEnabled`，无新增表或迁移。
- 修改与管理员审计在同一事务中提交；审计失败回滚修改。通用 option 写接口禁止修改该键。
- 每次购买请求读取数据库，不依赖节点缓存。数据库不可用或设置非法时拒绝购买。
- 关闭后拒绝报价、创建订单、结账、付费解绑与离线延长；支付通知、既有订单结算、退款和对账继续处理。
- 后台加载失败时开关不可操作；保存失败时保留服务端已确认状态。

## 验证

- SQLite 3.50.4、MySQL Community 8.4.13、PostgreSQL 15.19 均在独立测试数据库验证：环境默认值、保存值覆盖环境变量、连续启停、审计失败回滚、审计数量及数据库故障时拒绝放行。
- MySQL 使用本机 127.0.0.1:3306 的隔离库，PostgreSQL 使用 SSH 转发到隔离库；测试库名称为 `d2s_purchase_probe_20261104234547`，未读写生产业务表。
- 数据库测试命令：`go test ./model -run '^TestD2SPurchaseSettingsPersistenceAndAtomicAudit$' -count=1 -v`，通过临时进程环境注入两个测试 DSN；凭据不写入仓库。
- `go test ./controller ./model ./router ./middleware ./service -count=1` 通过。
- 前端购买开关、支付宝设置交互回归共 3 项通过；覆盖启停、保存失败、加载失败和重试。
- TypeScript 检查、改动文件 Oxlint、生产构建与 `git diff --check` 通过。
- 交付保持生产购买关闭；本验证不代表真实支付、退款或账单验收完成。

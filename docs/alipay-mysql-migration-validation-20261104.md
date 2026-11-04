# 支付宝配置表 MySQL 迁移验证记录

验证日期：2026-11-04（Asia/Shanghai）。

## 范围与环境

本次验证范围为新增后台支付宝参数对应的 `d2s_alipay_pc_settings` 表、GORM 迁移以及实际配置读写函数。未连接生产数据库，也未运行整站所有表的生产迁移。

- 数据库：本机 MySQL Community Server 8.4.13，`127.0.0.1:3306`。
- GORM：`gorm.io/gorm v1.25.12`。
- 方言：`gorm.io/driver/mysql v1.5.7`。
- 驱动：`github.com/go-sql-driver/mysql v1.7.0`。
- 执行账号：`d2s_test@localhost`，仅获测试库权限；创建测试库和授权由本地管理员连接执行。
- 旧版结构依据：已部署提交 `e9c948dce0cfb6bded61468a9ff297dcd4adb920`，包含 `id`、`qr_pay_mode`、`updated_at`。
- 新建场景测试库：`d2s_alipay_migration_fresh_20261104091912`。
- 升级场景测试库：`d2s_alipay_migration_upgrade_20261104091912`。

## 首次验证发现及修复

初版新增的两个密钥密文字段使用 `TEXT NOT NULL DEFAULT ''`。真实 MySQL 在首次建表和旧表新增列时均返回：

```text
Error 1101 (42000): BLOB, TEXT, GEOMETRY or JSON column 'private_key_ciphertext' can't have a default value
```

已将 `private_key_ciphertext` 与 `public_key_ciphertext` 调整为允许 NULL、无数据库默认值的 TEXT 字段。旧记录新增列保持 NULL；GORM 读取为 Go 字符串零值，由现有代码按“未配置”处理。实际配置保存仍写入应用生成的密文。

修复前失败保留在独立测试库 `d2s_alipay_migration_fresh_20261104091822` 与 `d2s_alipay_migration_upgrade_20261104091822`；修复后使用上文新建的两个独立数据库重新验证，未复用失败后的表状态。

## 修复后结果

| 检查 | 新建数据库 | 旧表升级 |
| --- | --- | --- |
| 使用当前模型 AutoMigrate | 通过 | 通过 |
| 旧二维码模式和原始时间戳保留 | 不适用 | 通过 |
| 新配置开关默认关闭，密钥未配置 | 不适用 | 通过 |
| 立即再次 AutoMigrate，无 CREATE/ALTER/DROP/RENAME | 通过 | 通过 |
| 配置字段与模拟密文写入后完整读回 | 通过 | 通过 |
| 开关从 true 更新为 false | 通过 | 通过 |
| 仅更新二维码模式，其他字段与密文保留 | 通过 | 通过 |
| 数据写入后再迁移，无 DDL，SHOW CREATE TABLE 不变 | 通过 | 通过 |
| 再迁移后完整数据保留 | 通过 | 通过 |
| 重复主键被拒绝，行数仍为 1 | 通过 | 通过 |

密文读写测试使用明确标记的合成数据，不使用真实支付宝密钥。迁移测试不负责验证支付宝签名、密文加解密或真实支付。

## 执行方式与输出

测试文件：`model/d2s_alipay_mysql_migration_test.go`。

测试使用两个环境变量传入 DSN：

- `D2S_TEST_ALIPAY_MYSQL_FRESH_DSN`。
- `D2S_TEST_ALIPAY_MYSQL_UPGRADE_DSN`。

每次执行应准备两个没有该业务表的独立新库。测试会校验连接为本机 TCP 3306，数据库名称以 `d2s_alipay_migration_` 开头，并拒绝在已有该表的数据库上运行。凭据仅在执行进程内从本机受限配置文件读入，没有写入仓库或本记录。

```powershell
go test ./model -run '^TestD2SAlipaySettingsMySQLMigration$' -count=1 -v
```

结果：

```text
--- PASS: TestD2SAlipaySettingsMySQLMigration (0.55s)
    --- PASS: TestD2SAlipaySettingsMySQLMigration/fresh (0.12s)
    --- PASS: TestD2SAlipaySettingsMySQLMigration/upgrade (0.43s)
PASS
ok github.com/QuantumNous/new-api/model 0.733s
```

字段修复后补充运行已有模型与控制器回归：

```powershell
go test ./model ./controller
git diff --check
```

结果：两个测试包通过，diff 检查通过。常规模型/控制器测试仍使用各自已有的测试夹具；上方单独的迁移测试才是本次真实 MySQL 验证证据。

## 验证边界

本次按用户最新范围仅完成真实 MySQL 实例验证。未据此宣称所有 MySQL 历史版本、PostgreSQL、独立日志数据库或整站全部表迁移已验收。该配置表位于业务数据库，不属于独立审计日志数据库。改动尚未提交或部署；真实支付、退款和账单验收也不在本次验证范围内。

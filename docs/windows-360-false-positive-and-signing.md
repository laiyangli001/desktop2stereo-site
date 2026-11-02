# Windows 版误报与签名处理

本文用于处理 360 将 Windows 可执行文件误判为病毒的情况，并规范公开发布的 Windows 文件。

## 先辨别被拦截的文件

`controller.test.exe` 是 Go 在运行 controller 测试时生成的测试程序。当前它不属于面向用户的发布产物：`.github/workflows/release.yml` 发布的是服务端 `new-api-*.exe`，`.github/workflows/electron-build.yml` 发布的是 `electron/dist/*.exe`。不要把测试临时文件放进用户下载包或 GitHub Release。

若 360 报警的是正式客户端或服务端安装包，应以实际被拦截文件的名称和 SHA-256 为准，不能用 `controller.test.exe` 的结果代替正式产物检测。

### 当前待核样本（2026-11-03）

- 测试包：`github.com/QuantumNous/new-api/pkg/billingexpr`。
- 固定路径：`E:\AI_2D_to_3D\4.LC700X_Desktop2Stereo\.go-test-binaries\github_com_quantumnous_new_api_pkg_billingexpr.test.exe`。
- 编译成功后、启动前文件存在；当时 SHA-256 为 `BD8F25A8C9E6A04467F38279287A7E625BCA6BC4FB0A0B2989D49D5A2ED0630C`。调用该测试程序时 Windows 报“系统找不到指定的文件”，调用后原路径文件不存在。
- 检查时发现 `360Safe` 和 `360tray` 进程运行；Windows Defender 最近事件中没有该文件的匹配记录。现有证据强烈指向 360 在启动时隔离或移除，但尚无 360 检测名称、隔离记录或截图，不能当作厂商已确认的误报结论。
- 当前 SHA 来自本地未提交工作树下的 Go 测试二进制，代码基线 HEAD 为 `e5bc8b12914be139bc47e2685e1a4ef96fda7543`。它不是 Windows 用户发布产物。若向 360 申诉，应提交同一 SHA 的原始样本和 360 报警详情；不得通过关闭保护或设置通用排除项来替代厂商复核。
- `scripts/d2s-fixed-go-test.ps1` 现会在启动测试程序前打印其 SHA-256，并检查构建后、启动期间及运行后的文件是否消失；发现消失时报告包名、路径和启动前哈希，并停止，不自动重试或调整安全软件设置。

## 向 360 提交误报样本

1. 记录 360 产品名称和版本、病毒库更新时间、完整检测名称、报警截图、文件来源 URL、项目版本或 Git commit。
2. 对未被隔离的原始文件计算 SHA-256：`Get-FileHash -Algorithm SHA256 <文件路径>`。如由 GitHub Actions 构建，先与该次构建产物及其 checksum 对照，保留对应工作流运行地址。
3. 将**触发报警的同一份文件**提交到 [360 软件开发者误报反馈页](https://open.soft.360.cn/report.php)，附上截图和简明软件说明。不要提交另一版本，也不要给样本加密或分卷。
4. 360 软件开放平台说明误报文件支持 Windows 的 EXE、RAR、ZIP、7Z、MSI（最大 512 MB），并称通常 1–2 个工作日处理；请以提交页面和邮件反馈为准。可注册厂商账户，后续由平台提交软件检测。
5. 收到结论后，再对**同一 SHA-256**复查 360 检测结果。版本更新后文件哈希变化，应重新确认新发布产物的状态。

官方入口与说明：[360 开发者误报反馈](https://open.soft.360.cn/report.php)、[360 软件误报处理说明](https://www.360.cn/n/10454.html)。

## 正式 Windows 发布的签名要求

公开分发的 EXE/MSI 应使用组织或发布者身份对应的受信任 Authenticode 代码签名证书签名，签名后再生成 checksum，并在发布前验证签名和时间戳。发布版本持续使用同一发布者身份，有助于建立发布者信任。签名不能保证 360 一定不会误报；遇到检测仍需按上节向 360 提交实际发布样本。

签名私钥必须保存在受控的代码签名服务或硬件安全模块中，通过 CI 的短期身份授权调用签名服务。不得把私钥、PFX 密码或明文证书写入代码库、工作流文本或公开构建产物。当前 GitHub Actions Windows 发布流程尚未配置代码签名凭据与签名步骤；配置正式证书和签名服务后，才可启用强制签名发布门禁。

微软说明：[SignTool](https://learn.microsoft.com/en-us/windows/win32/seccrypto/signtool)、[Authenticode 时间戳](https://learn.microsoft.com/en-us/windows/win32/seccrypto/time-stamping-authenticode-signatures)、[SmartScreen 声誉机制](https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/smartscreen-reputation)。微软指出，代码签名不会令新的发布文件立即获得 SmartScreen 声誉；未签名构建也无法继承前一版本的发布者声誉。

## 不应采用的处理方式

不要要求用户关闭 360、添加全盘或通用排除项，也不要改写/混淆程序来绕过查杀。这些措施会降低用户设备保护或削弱文件可审计性，不能替代误报复核、正式签名和发布核验。

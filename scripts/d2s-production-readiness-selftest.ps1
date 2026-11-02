param()

$ErrorActionPreference = "Stop"
$readinessScript = Join-Path $PSScriptRoot "d2s-production-readiness.ps1"
$requiredVariables = @(
    "SQL_DSN",
    "REDIS_CONN_STRING",
    "SESSION_SECRET",
    "D2S_LICENSE_KEY_ID",
    "D2S_PAYMENT_BRIDGE_SECRET"
)
$originalValues = @{}
foreach ($name in $requiredVariables) {
    $originalValues[$name] = [Environment]::GetEnvironmentVariable($name, "Process")
}
$tokens = $null
$parseErrors = $null
$global:d2sReadinessSelfTestState = @{}

try {
    foreach ($name in $requiredVariables) {
        [Environment]::SetEnvironmentVariable($name, "selftest-placeholder", "Process")
    }
    [Environment]::SetEnvironmentVariable("D2S_LICENSE_KEY_ID", "readiness-selftest-kid", "Process")

    [System.Management.Automation.Language.Parser]::ParseFile(
        $readinessScript,
        [ref]$tokens,
        [ref]$parseErrors
    ) | Out-Null
    if ($parseErrors.Count -gt 0) {
        throw "Production readiness script has PowerShell syntax errors."
    }

    function Invoke-ReadinessMock(
        [string]$SupportUrl,
        [bool]$SupportEndpointAvailable,
        [string]$PrivacyPolicy = "Configured privacy policy for readiness self-test.",
        [string]$ExpectedVersion = "",
        [bool]$ValidSigningKey = $true,
        [switch]$HttpOnly
    ) {
        $global:d2sReadinessSelfTestState.SupportUrl = $SupportUrl
        $global:d2sReadinessSelfTestState.SupportEndpointAvailable = $SupportEndpointAvailable
        $global:d2sReadinessSelfTestState.PrivacyPolicy = $PrivacyPolicy
        $global:d2sReadinessSelfTestState.ValidSigningKey = $ValidSigningKey

        function Invoke-WebRequest {
            param([string]$Uri, [switch]$UseBasicParsing)
            [pscustomobject]@{
                StatusCode = 200
                Content = '{"data":{"version":"d11d0cc1da5070bc65b57ff72181ed2865043833"}}'
            }
        }

        function Invoke-RestMethod {
            param([string]$Uri, [string]$Method, [int]$TimeoutSec)
            if ($Uri -like "*/api/v1/license/keys") {
                return [pscustomobject]@{
                    success = $true
                    data = [pscustomobject]@{
                        keys = @([pscustomobject]@{
                            kid = "readiness-selftest-kid"
                            alg = "ES256"
                            crv = "P-256"
                            x = if ($global:d2sReadinessSelfTestState.ValidSigningKey) { "axfR8uEsQkf4vOblY6RA8ncDfYEt6zOg9KE5RdiYwpY" } else { "invalid-x" }
                            y = if ($global:d2sReadinessSelfTestState.ValidSigningKey) { "T-NC4v4af5uO5-tKfA-eFivOM1drMV7Oy7ZAaDe_UfU" } else { "invalid-y" }
                        })
                    }
                }
            }
            if ($Uri -like "*/api/v1/support/config") {
                if (-not $global:d2sReadinessSelfTestState.SupportEndpointAvailable) {
                    throw "Mock HTTP 404"
                }
                return [pscustomobject]@{
                    success = $true
                    data = [pscustomobject]@{
                        enterprise_wechat_url = $global:d2sReadinessSelfTestState.SupportUrl
                    }
                }
            }
            if ($Uri -like "*/api/privacy-policy") {
                return [pscustomobject]@{
                    success = $true
                    data = $global:d2sReadinessSelfTestState.PrivacyPolicy
                }
            }
            throw "Unexpected readiness endpoint: $Uri"
        }

        $arguments = @{ BaseUrl = "https://readiness.example" }
        if ($HttpOnly) {
            $arguments.HttpOnly = $true
            $arguments.ExpectedVersion = $ExpectedVersion
            $arguments.ExpectedSigningKeyId = "readiness-selftest-kid"
        }
        & $script:readinessScript @arguments
    }

    foreach ($name in $requiredVariables) {
        [Environment]::SetEnvironmentVariable($name, $null, "Process")
    }
    Invoke-ReadinessMock "https://work.weixin.qq.com/ca/cawcde40faa1d08998" $true `
        -ExpectedVersion "d11d0cc1da5070bc65b57ff72181ed2865043833" -HttpOnly
    Write-Host "PASS HTTP-only readiness checks the expected release without local secrets"

    $versionMismatchRejected = $false
    try {
        Invoke-ReadinessMock "https://work.weixin.qq.com/ca/cawcde40faa1d08998" $true `
            -ExpectedVersion "0000000000000000000000000000000000000000" -HttpOnly
    } catch {
        $versionMismatchRejected = $_.Exception.Message -like "*does not match the expected release SHA*"
    }
    if (-not $versionMismatchRejected) {
        throw "HTTP-only readiness did not reject a running version mismatch."
    }
    Write-Host "PASS HTTP-only readiness rejects an unexpected release SHA"

    $invalidJwkRejected = $false
    try {
        Invoke-ReadinessMock "https://work.weixin.qq.com/ca/cawcde40faa1d08998" $true `
            -ExpectedVersion "d11d0cc1da5070bc65b57ff72181ed2865043833" -ValidSigningKey $false -HttpOnly
    } catch {
        $invalidJwkRejected = $_.Exception.Message -like "*did not publish a usable ES256 public key*"
    }
    if (-not $invalidJwkRejected) {
        throw "HTTP-only readiness accepted an invalid P-256 public key."
    }
    Write-Host "PASS HTTP-only readiness rejects malformed P-256 coordinates"

    $validHttpOnlyArguments = @{
        HttpOnly = $true
        ExpectedVersion = "d11d0cc1da5070bc65b57ff72181ed2865043833"
        ExpectedSigningKeyId = "readiness-selftest-kid"
    }
    foreach ($case in @(
        @{ Name = "missing expected version"; Arguments = @{ HttpOnly = $true; ExpectedSigningKeyId = "readiness-selftest-kid" } },
        @{ Name = "missing expected signing key ID"; Arguments = @{ HttpOnly = $true; ExpectedVersion = "d11d0cc1da5070bc65b57ff72181ed2865043833" } },
        @{ Name = "secret checks"; Arguments = ($validHttpOnlyArguments + @{ RequireSecrets = $true }) },
        @{ Name = "payment-provider checks"; Arguments = ($validHttpOnlyArguments + @{ RequiredPaymentProviders = @("stripe") }) },
        @{ Name = "skipped HTTP checks"; Arguments = ($validHttpOnlyArguments + @{ SkipHttp = $true }) }
    )) {
        $rejected = $false
        try {
            $arguments = $case.Arguments.Clone()
            $arguments.BaseUrl = "https://readiness.example"
            & $script:readinessScript @arguments
        } catch {
            $rejected = $true
        }
        if (-not $rejected) {
            throw "HTTP-only readiness accepted $($case.Name)."
        }
        Write-Host "PASS rejected HTTP-only combination: $($case.Name)"
    }

    $unexpectedSigningKeyArgumentRejected = $false
    try {
        & $script:readinessScript -BaseUrl "https://readiness.example" `
            -ExpectedSigningKeyId "readiness-selftest-kid"
    } catch {
        $unexpectedSigningKeyArgumentRejected = $_.Exception.Message -like "*-ExpectedSigningKeyId can only be used with -HttpOnly*"
    }
    if (-not $unexpectedSigningKeyArgumentRejected) {
        throw "Readiness accepted -ExpectedSigningKeyId without -HttpOnly."
    }
    Write-Host "PASS rejected signing key ID outside HTTP-only mode"

    foreach ($name in $requiredVariables) {
        [Environment]::SetEnvironmentVariable($name, "selftest-placeholder", "Process")
    }
    [Environment]::SetEnvironmentVariable("D2S_LICENSE_KEY_ID", "readiness-selftest-kid", "Process")

    Invoke-ReadinessMock "https://work.weixin.qq.com/ca/cawcde40faa1d08998" $true
    Write-Host "PASS valid Enterprise WeChat contact URL and configured privacy policy"

    $emptyPrivacyPolicyRejected = $false
    try {
        Invoke-ReadinessMock "https://work.weixin.qq.com/ca/cawcde40faa1d08998" $true ""
    } catch {
        $emptyPrivacyPolicyRejected = $true
    }
    if (-not $emptyPrivacyPolicyRejected) {
        throw "Production readiness accepted an empty privacy policy."
    }
    Write-Host "PASS rejected empty privacy policy"

    $privacyDraftPath = Join-Path $PSScriptRoot "..\docs\privacy-policy-draft.md"
    $privacyDraft = Get-Content -LiteralPath $privacyDraftPath -Raw -Encoding UTF8
    $privacyDraftRejected = $false
    try {
        Invoke-ReadinessMock "https://work.weixin.qq.com/ca/cawcde40faa1d08998" $true $privacyDraft
    } catch {
        $privacyDraftRejected = $_.Exception.Message -like "*contains unresolved placeholder text*"
    }
    if (-not $privacyDraftRejected) {
        throw "Production readiness accepted the internal privacy-policy draft."
    }
    Write-Host "PASS rejected the repository's internal privacy-policy draft"

    foreach ($placeholderPolicy in @(
        "Privacy policy draft: 【待填写】运营主体名称",
        "Privacy policy draft: contact details TODO",
        "隐私联系渠道：[填写可持续接收请求的邮箱或其他渠道]",
        "更新日期：[待确定]",
        "[负责人必须核实并完成评估]",
        "[须依据生产配置和合同逐项填写]",
        "运营主体：[运营主体完整名称]",
        "隐私联系渠道：[隐私联系邮箱/渠道]",
        "注册地址：[注册地址]"
    )) {
        $placeholderRejected = $false
        try {
            Invoke-ReadinessMock "https://work.weixin.qq.com/ca/cawcde40faa1d08998" $true $placeholderPolicy
        } catch {
            $placeholderRejected = $_.Exception.Message -like "*contains unresolved placeholder text*"
        }
        if (-not $placeholderRejected) {
            throw "Production readiness accepted privacy policy placeholder content: $placeholderPolicy"
        }
    }
    Write-Host "PASS rejected privacy policy placeholder text"

    Invoke-ReadinessMock "https://work.weixin.qq.com/ca/cawcde40faa1d08998" $true `
        "请参阅 [个人信息保护法](https://example.org/privacy) 相关说明。"
    Write-Host "PASS accepted a bracketed legal reference without placeholders"

    foreach ($case in @(
        @{ Name = "unapproved host"; Url = "https://support.example/ca/contact"; Available = $true },
        @{ Name = "wrong path"; Url = "https://work.weixin.qq.com/contact"; Available = $true },
        @{ Name = "non-default port"; Url = "https://work.weixin.qq.com:8443/ca/contact"; Available = $true },
        @{ Name = "explicit default port"; Url = "https://work.weixin.qq.com:443/ca/contact"; Available = $true },
        @{ Name = "query parameter"; Url = "https://work.weixin.qq.com/ca/contact?next=https://evil.example"; Available = $true },
        @{ Name = "user information"; Url = "https://user@work.weixin.qq.com/ca/contact"; Available = $true },
        @{ Name = "fragment"; Url = "https://work.weixin.qq.com/ca/contact#redirect"; Available = $true },
        @{ Name = "missing support endpoint"; Url = ""; Available = $false }
    )) {
        $rejected = $false
        try {
            Invoke-ReadinessMock $case.Url $case.Available
        } catch {
            $rejected = $true
        }
        if (-not $rejected) {
            throw "Production readiness accepted $($case.Name)."
        }
        Write-Host "PASS rejected $($case.Name)"
    }
} finally {
    foreach ($name in $requiredVariables) {
        [Environment]::SetEnvironmentVariable($name, $originalValues[$name], "Process")
    }
    Remove-Variable -Name d2sReadinessSelfTestState -Scope Global -ErrorAction SilentlyContinue
}

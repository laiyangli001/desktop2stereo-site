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
        [string]$PrivacyPolicy = "Configured privacy policy for readiness self-test."
    ) {
        $global:d2sReadinessSelfTestState.SupportUrl = $SupportUrl
        $global:d2sReadinessSelfTestState.SupportEndpointAvailable = $SupportEndpointAvailable
        $global:d2sReadinessSelfTestState.PrivacyPolicy = $PrivacyPolicy

        function Invoke-WebRequest {
            param([string]$Uri, [switch]$UseBasicParsing)
            [pscustomobject]@{ StatusCode = 200 }
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
                            x = "selftest-x"
                            y = "selftest-y"
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

        & $script:readinessScript -BaseUrl "https://readiness.example"
    }

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

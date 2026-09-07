[CmdletBinding()]
param(
    [string]$BaseUrl = "https://d2s.site",
    [switch]$RequireSecrets,
    [switch]$SkipHttp
)

$ErrorActionPreference = "Stop"

function Assert-NonEmptyEnvironmentVariable([string]$Name) {
    $value = [Environment]::GetEnvironmentVariable($Name)
    if ([string]::IsNullOrWhiteSpace($value)) {
        throw "Required environment variable is missing: $Name"
    }
    Write-Host "OK secret/config present: $Name"
}

@(
    "SQL_DSN",
    "REDIS_CONN_STRING",
    "SESSION_SECRET",
    "D2S_LICENSE_KEY_ID",
    "D2S_PAYMENT_BRIDGE_SECRET"
) | ForEach-Object { Assert-NonEmptyEnvironmentVariable $_ }

if ($RequireSecrets) {
    @(
        "D2S_LICENSE_PRIVATE_KEY_B64",
        "D2S_OFFLINE_EXTENSION_CNY_MINOR",
        "D2S_OFFLINE_EXTENSION_USD_MINOR"
    ) | ForEach-Object { Assert-NonEmptyEnvironmentVariable $_ }
}

if (-not $SkipHttp) {
    $normalizedBaseUrl = $BaseUrl.TrimEnd('/')
    $health = Invoke-WebRequest -Uri "$normalizedBaseUrl/api/status" -UseBasicParsing
    if ($health.StatusCode -ne 200) {
        throw "Health endpoint returned HTTP $($health.StatusCode)"
    }
    Write-Host "OK health endpoint: $($health.StatusCode)"

    $keys = Invoke-RestMethod -Uri "$normalizedBaseUrl/api/v1/license/keys" -Method Get
    if (-not $keys.success -or $null -eq $keys.data.keys) {
        throw "License key endpoint returned an invalid response envelope"
    }
    $publishedKeys = @($keys.data.keys)
    $validKeys = @($publishedKeys | Where-Object {
        $_.kid -and $_.alg -eq "ES256" -and $_.crv -eq "P-256" -and $_.x -and $_.y
    })
    if ($validKeys.Count -eq 0) {
        throw "License key endpoint did not publish a usable ES256 public key"
    }
    Write-Host "OK public signing keys endpoint ($($validKeys.Count) usable key(s))"
}

Write-Host "Production readiness checks passed. Record the drill evidence before enabling public checkout."

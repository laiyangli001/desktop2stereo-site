[CmdletBinding()]
param(
    [string]$BaseUrl = "https://d2s.site",
    [switch]$RequireSecrets,
    [switch]$SkipHttp,
    [string[]]$RequiredPaymentProviders = @()
)

$ErrorActionPreference = "Stop"

function Assert-NonEmptyEnvironmentVariable([string]$Name) {
    $value = [Environment]::GetEnvironmentVariable($Name)
    if ([string]::IsNullOrWhiteSpace($value)) {
        throw "Required environment variable is missing: $Name"
    }
    Write-Host "OK secret/config present: $Name"
}

function Assert-PositiveIntegerEnvironmentVariable([string]$Name) {
    $value = [Environment]::GetEnvironmentVariable($Name)
    $parsed = 0L
    if (-not [Int64]::TryParse($value, [Globalization.NumberStyles]::Integer, [Globalization.CultureInfo]::InvariantCulture, [ref]$parsed) -or $parsed -le 0) {
        throw "Required environment variable must be a positive integer: $Name"
    }
    Write-Host "OK positive integer config present: $Name"
}

function Assert-MinimumLengthEnvironmentVariable([string]$Name, [int]$MinimumLength) {
    $value = [Environment]::GetEnvironmentVariable($Name)
    if ($value.Length -lt $MinimumLength) {
        throw "Required environment variable is shorter than $MinimumLength characters: $Name"
    }
    Write-Host "OK high-entropy secret length present: $Name"
}

function Assert-RequiredPaymentProviderSecrets([string[]]$Providers) {
    $supportedProviders = @('stripe', 'creem', 'epay', 'paymentfm', 'alipay', 'wechat', 'waffo', 'waffo_pancake')
    foreach ($provider in @($Providers | ForEach-Object { $_.Trim().ToLowerInvariant() } | Where-Object { $_ })) {
        if ($provider -notin $supportedProviders) {
            throw "RequiredPaymentProviders contains an unsupported provider: $provider"
        }
        $environmentName = "D2S_PAYMENT_BRIDGE_SECRET_$($provider.ToUpperInvariant())"
        Assert-MinimumLengthEnvironmentVariable $environmentName 32
    }
}

function Assert-TrustedProxyConfiguration([string]$RawValue) {
    $entries = @($RawValue.Split(',') | ForEach-Object { $_.Trim() } | Where-Object { $_ })
    if ($entries.Count -eq 0) {
        throw "TRUSTED_PROXIES must contain an explicit value"
    }
    if ($entries -contains "none") {
        if ($entries.Count -ne 1) {
            throw "TRUSTED_PROXIES=none must be used alone"
        }
        Write-Host "OK trusted proxies explicitly disabled"
        return
    }

    foreach ($entry in $entries) {
        if ($entry -in @('*', 'all', '0.0.0.0/0', '::/0') -or $entry -match '/0$') {
            throw "TRUSTED_PROXIES must not trust the entire address space: $entry"
        }
        $parts = $entry.Split('/')
        if ($parts.Count -gt 2) {
            throw "TRUSTED_PROXIES contains an invalid IP or CIDR: $entry"
        }
        try {
            $address = [Net.IPAddress]::Parse($parts[0])
        } catch {
            throw "TRUSTED_PROXIES contains an invalid IP or CIDR: $entry"
        }
        if ($parts.Count -eq 2) {
            $prefix = 0
            if (-not [Int32]::TryParse($parts[1], [Globalization.NumberStyles]::Integer, [Globalization.CultureInfo]::InvariantCulture, [ref]$prefix)) {
                throw "TRUSTED_PROXIES contains an invalid CIDR prefix: $entry"
            }
            $maxPrefix = if ($address.AddressFamily -eq [Net.Sockets.AddressFamily]::InterNetwork) { 32 } else { 128 }
            if ($prefix -lt 1 -or $prefix -gt $maxPrefix) {
                throw "TRUSTED_PROXIES contains an invalid CIDR prefix: $entry"
            }
        }
    }
    Write-Host "OK trusted proxy configuration contains $($entries.Count) explicit address(es)"
}

@(
    "SQL_DSN",
    "REDIS_CONN_STRING",
    "SESSION_SECRET",
    "D2S_LICENSE_KEY_ID",
    "D2S_PAYMENT_BRIDGE_SECRET"
) | ForEach-Object { Assert-NonEmptyEnvironmentVariable $_ }

if ($RequireSecrets) {
    Assert-MinimumLengthEnvironmentVariable "SESSION_SECRET" 32
    Assert-MinimumLengthEnvironmentVariable "D2S_PAYMENT_BRIDGE_SECRET" 32
    Assert-NonEmptyEnvironmentVariable "D2S_LICENSE_PRIVATE_KEY_B64"
    Assert-PositiveIntegerEnvironmentVariable "D2S_OFFLINE_EXTENSION_CNY_MINOR"
    Assert-PositiveIntegerEnvironmentVariable "D2S_OFFLINE_EXTENSION_USD_MINOR"
    Assert-NonEmptyEnvironmentVariable "D2S_DEVICE_VERIFICATION_URI"
    Assert-NonEmptyEnvironmentVariable "SESSION_COOKIE_TRUSTED_URL"
    Assert-NonEmptyEnvironmentVariable "TRUSTED_PROXIES"
    Assert-TrustedProxyConfiguration ([Environment]::GetEnvironmentVariable("TRUSTED_PROXIES"))
    Assert-RequiredPaymentProviderSecrets $RequiredPaymentProviders

    try {
        $privateKeyBytes = [Convert]::FromBase64String([Environment]::GetEnvironmentVariable("D2S_LICENSE_PRIVATE_KEY_B64"))
    } catch {
        throw "D2S_LICENSE_PRIVATE_KEY_B64 must be valid Base64"
    }
    if ($privateKeyBytes.Length -lt 32) {
        throw "D2S_LICENSE_PRIVATE_KEY_B64 does not contain a usable private key payload"
    }
    $isPemPayload = $privateKeyBytes[0] -eq [byte][char]'-'
    $isDerPayload = $privateKeyBytes[0] -eq 0x30
    if (-not ($isPemPayload -or $isDerPayload)) {
        throw "D2S_LICENSE_PRIVATE_KEY_B64 must contain a PEM or DER private key payload"
    }
    Write-Host "OK signing private key is valid Base64 with PEM/DER payload"

    $cookieSecure = [Environment]::GetEnvironmentVariable("SESSION_COOKIE_SECURE")
    if ($cookieSecure -ne "true") {
        throw "Production readiness requires SESSION_COOKIE_SECURE=true"
    }
    Write-Host "OK secure session cookie enabled"

    $trustedUrl = [Environment]::GetEnvironmentVariable("SESSION_COOKIE_TRUSTED_URL")
    $trustedOrigins = @($trustedUrl.Split(',') | ForEach-Object { $_.Trim() } | Where-Object { $_ })
    if ($trustedOrigins.Count -eq 0) {
        throw "Production readiness requires at least one HTTPS SESSION_COOKIE_TRUSTED_URL origin"
    }
    foreach ($origin in $trustedOrigins) {
        try {
            $uri = [Uri]$origin
        } catch {
            throw "Production readiness requires valid HTTPS SESSION_COOKIE_TRUSTED_URL origins"
        }
        if ($uri.Scheme -ne "https" -or [string]::IsNullOrWhiteSpace($uri.Host) -or $uri.AbsolutePath -ne "/" -or $uri.Query -or $uri.Fragment) {
            throw "SESSION_COOKIE_TRUSTED_URL must contain HTTPS origins without paths, queries, or fragments"
        }
    }
    Write-Host "OK trusted session URL origins use HTTPS ($($trustedOrigins.Count) configured)"

    $deviceURI = [Environment]::GetEnvironmentVariable("D2S_DEVICE_VERIFICATION_URI")
    if ($deviceURI -notmatch '^https://[^\s/]+(?:/[^\s]*)?$') {
        throw "Production readiness requires an HTTPS D2S_DEVICE_VERIFICATION_URI"
    }
    Write-Host "OK device verification URI uses HTTPS"
}

if (-not $SkipHttp) {
    $normalizedBaseUrl = $BaseUrl.TrimEnd('/')
    if ($normalizedBaseUrl -notmatch '^https://') {
        throw "Production HTTP readiness checks require an HTTPS BaseUrl"
    }
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

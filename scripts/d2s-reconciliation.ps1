[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$AdminToken,
    [string]$BaseUrl = "https://d2s.site",
    [long]$StartAt,
    [long]$EndAt,
    [string]$OutputPath,
    [switch]$AllowHttp
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($AdminToken)) {
    throw "AdminToken is required"
}

$normalizedBaseUrl = $BaseUrl.TrimEnd('/')
if (-not $AllowHttp -and $normalizedBaseUrl -notmatch '^https://') {
    throw "Production reconciliation requires an HTTPS BaseUrl (use -AllowHttp only for local testing)"
}
if (($StartAt -gt 0) -xor ($EndAt -gt 0)) {
    throw "StartAt and EndAt must be provided together"
}
if ($StartAt -gt 0 -and $EndAt -le $StartAt) {
    throw "EndAt must be greater than StartAt"
}

$query = ""
if ($StartAt -gt 0) {
    $query = "?start_at=$StartAt&end_at=$EndAt"
}
$uri = "$normalizedBaseUrl/api/v1/admin/reconciliation$query"
$headers = @{ Authorization = "Bearer $AdminToken" }
$response = Invoke-RestMethod -Uri $uri -Method Get -Headers $headers -TimeoutSec 30
if (-not $response.success -or $null -eq $response.data) {
    throw "Reconciliation endpoint returned an unsuccessful response"
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $stamp = [DateTime]::UtcNow.ToString("yyyyMMdd-HHmmss")
    $OutputPath = Join-Path (Get-Location) "d2s-reconciliation-$stamp.json"
}
$resolvedOutput = [System.IO.Path]::GetFullPath($OutputPath)
if (Test-Path -LiteralPath $resolvedOutput) {
    throw "Refusing to overwrite existing report: $resolvedOutput"
}
$parent = Split-Path -Parent $resolvedOutput
if (-not (Test-Path -LiteralPath $parent -PathType Container)) {
    throw "Report directory does not exist: $parent"
}

$response | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $resolvedOutput -Encoding UTF8
Write-Host "Reconciliation report saved: $resolvedOutput"
Write-Host "Review mismatches before treating the day as settled: $($response.data.mismatches.Count)"

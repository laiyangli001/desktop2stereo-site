[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$ReconciliationToken,
    [string]$BaseUrl = "https://d2s.site",
    [long]$StartAt,
    [long]$EndAt,
    [string]$OutputPath,
    [switch]$AllowHttp,
    [switch]$FailOnMismatch
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($ReconciliationToken)) {
    throw "ReconciliationToken is required"
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
$headers = @{ Authorization = "Bearer $ReconciliationToken" }
$response = Invoke-RestMethod -Uri $uri -Method Get -Headers $headers -TimeoutSec 30
if (-not $response.success -or $null -eq $response.data) {
    throw "Reconciliation endpoint returned an unsuccessful response"
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $stamp = [DateTime]::UtcNow.ToString("yyyyMMdd-HHmmss")
    $OutputPath = Join-Path (Get-Location) "d2s-reconciliation-$stamp.json"
}
$currentDirectory = (Get-Location).ProviderPath
$combinedOutputPath = [System.IO.Path]::Combine($currentDirectory, $OutputPath)
$resolvedOutput = [System.IO.Path]::GetFullPath($combinedOutputPath)
if (Test-Path -LiteralPath $resolvedOutput) {
    throw "Refusing to overwrite existing report: $resolvedOutput"
}
$parent = Split-Path -Parent $resolvedOutput
if ([string]::IsNullOrWhiteSpace($parent)) {
    $parent = [Environment]::CurrentDirectory
}
if (-not (Test-Path -LiteralPath $parent -PathType Container)) {
    throw "Report directory does not exist: $parent"
}

$jsonPayload = $response | ConvertTo-Json -Depth 10
$reportStream = [System.IO.File]::Open(
    $resolvedOutput,
    [System.IO.FileMode]::CreateNew,
    [System.IO.FileAccess]::Write,
    [System.IO.FileShare]::None
)
$writer = $null
try {
    $writer = [System.IO.StreamWriter]::new($reportStream, [System.Text.UTF8Encoding]::new($false))
    $writer.WriteLine($jsonPayload)
    $writer.Flush()
}
finally {
    if ($null -ne $writer) {
        $writer.Dispose()
    }
    else {
        $reportStream.Dispose()
    }
}
$mismatches = @($response.data.mismatches)
Write-Host "Reconciliation report saved: $resolvedOutput"
Write-Host "Review mismatches before treating the day as settled: $($mismatches.Count)"
if ($FailOnMismatch -and $mismatches.Count -gt 0) {
    throw "Reconciliation report contains $($mismatches.Count) mismatch(es): $resolvedOutput"
}

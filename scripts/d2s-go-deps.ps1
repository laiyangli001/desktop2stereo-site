[CmdletBinding()]
param(
    [switch]$AllowMirrorFallback
)

$ErrorActionPreference = "Stop"

$officialProxy = "https://proxy.golang.org,direct"
$officialSumDB = "sum.golang.org"
$mirrorProxy = "https://goproxy.cn,direct"
$mirrorSumDB = "sum.golang.google.cn"

function Invoke-GoDependencyVerification([string]$Proxy, [string]$SumDB) {
    $previousProxy = $env:GOPROXY
    $previousSumDB = $env:GOSUMDB
    try {
        $env:GOPROXY = $Proxy
        $env:GOSUMDB = $SumDB
        Write-Host "Using GOPROXY=$Proxy"
        Write-Host "Using GOSUMDB=$SumDB"
        & go mod download
        if ($LASTEXITCODE -ne 0) {
            throw "go mod download failed with exit code $LASTEXITCODE"
        }
        & go mod verify
        if ($LASTEXITCODE -ne 0) {
            throw "go mod verify failed with exit code $LASTEXITCODE"
        }
    } finally {
        if ($null -eq $previousProxy) {
            Remove-Item Env:GOPROXY -ErrorAction SilentlyContinue
        } else {
            $env:GOPROXY = $previousProxy
        }
        if ($null -eq $previousSumDB) {
            Remove-Item Env:GOSUMDB -ErrorAction SilentlyContinue
        } else {
            $env:GOSUMDB = $previousSumDB
        }
    }
}

try {
    Invoke-GoDependencyVerification $officialProxy $officialSumDB
    Write-Host "Go module download and verification passed using official services."
} catch {
    if (-not $AllowMirrorFallback) {
        throw "Official Go module services were unavailable. Re-run with -AllowMirrorFallback only when the mirror is approved. Root cause: $($_.Exception.Message)"
    }
    Write-Warning "Official Go module services failed; using the explicitly requested mirror fallback."
    Invoke-GoDependencyVerification $mirrorProxy $mirrorSumDB
    Write-Host "Go module download and verification passed using the approved mirror fallback."
}

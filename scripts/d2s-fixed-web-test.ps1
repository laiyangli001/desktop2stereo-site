param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$VitestArgument = @('run')
)

$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$workspaceRoot = (Resolve-Path (Join-Path $repoRoot '..')).Path
$webRoot = Join-Path $repoRoot 'web'
$testTemp = Join-Path $workspaceRoot '.web-tmp-d2s'
$testCache = Join-Path $workspaceRoot '.web-cache-d2s'

New-Item -ItemType Directory -Force -Path $testTemp, $testCache | Out-Null
$env:TEMP = $testTemp
$env:TMP = $testTemp
$env:TMPDIR = $testTemp
$env:VITEST_CACHE_DIR = $testCache

Push-Location $webRoot
try {
    Write-Host "Running Vitest with fixed temp directory $testTemp"
    & node 'node_modules/vitest/vitest.mjs' @VitestArgument
    if ($LASTEXITCODE -ne 0) {
        throw "Vitest failed (exit code $LASTEXITCODE)."
    }
}
finally {
    Pop-Location
}

Write-Host 'Fixed-path frontend tests passed.'

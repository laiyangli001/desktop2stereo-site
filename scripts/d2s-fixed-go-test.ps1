param(
    [string[]]$Package = @('./controller', './model', './service', './router')
)

$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$workspaceRoot = (Resolve-Path (Join-Path $repoRoot '..')).Path
$testBinaryRoot = Join-Path $workspaceRoot '.go-test-binaries'
$goCache = Join-Path $workspaceRoot '.go-build-cache-d2s'
$goTemp = Join-Path $workspaceRoot '.go-tmp-d2s'

New-Item -ItemType Directory -Force -Path $testBinaryRoot, $goCache, $goTemp | Out-Null
$env:GOCACHE = $goCache
$env:GOTMPDIR = $goTemp

Push-Location $repoRoot
try {
    $listedPackages = & go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' @Package
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to inspect Go packages for test files (exit code $LASTEXITCODE)."
    }

    $testPackages = @($listedPackages | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    if ($testPackages.Count -eq 0) {
        Write-Host 'No Go test files found in the requested packages; skipping test execution.'
    }

    foreach ($packagePath in $testPackages) {
        $binaryName = ($packagePath -replace '[^A-Za-z0-9]+', '_').Trim('_').ToLowerInvariant()
        if ([string]::IsNullOrWhiteSpace($binaryName)) {
            throw "Cannot derive a fixed test binary name from package '$packagePath'."
        }

        $binaryPath = Join-Path $testBinaryRoot ($binaryName + '.test.exe')
        Write-Host "Building $packagePath -> $binaryPath"
        & go test -c -o $binaryPath $packagePath
        if ($LASTEXITCODE -ne 0) {
            throw "Failed to build test binary for $packagePath (exit code $LASTEXITCODE)."
        }

        Write-Host "Running $binaryPath"
        & $binaryPath '-test.count=1'
        if ($LASTEXITCODE -ne 0) {
            throw "Tests failed for $packagePath (exit code $LASTEXITCODE)."
        }
    }
}
finally {
    Pop-Location
}

Write-Host 'Fixed-path Go tests passed.'

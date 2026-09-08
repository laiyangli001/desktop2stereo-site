[CmdletBinding()]
param(
    [string]$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot ".."))
)

$ErrorActionPreference = "Stop"

$testFiles = @(git -C $RepositoryRoot ls-files "*_test.go" | Where-Object { $_ })
$patterns = @(
    @{ Name = "testing temporary directory"; Pattern = '\b[A-Za-z_][A-Za-z0-9_]*\.TempDir\(\)' },
    @{ Name = "os temporary directory"; Pattern = 'os\.TempDir\(\)' },
    @{ Name = "os temporary directory creation"; Pattern = 'os\.(?:MkdirTemp|CreateTemp)\(' }
)
$findings = [System.Collections.Generic.List[string]]::new()

foreach ($relativePath in $testFiles) {
    $fullPath = Join-Path $RepositoryRoot $relativePath
    if (-not (Test-Path -LiteralPath $fullPath -PathType Leaf)) {
        continue
    }

    $lineNumber = 0
    foreach ($line in (Get-Content -LiteralPath $fullPath -Encoding UTF8)) {
        $lineNumber++
        foreach ($rule in $patterns) {
            if ($line -match $rule.Pattern) {
                $findings.Add("$relativePath`:$lineNumber [$($rule.Name)]")
            }
        }
    }
}

if ($findings.Count -gt 0) {
    Write-Error ("Random test paths detected; use a repository-fixed workspace instead:`n" + ($findings -join "`n"))
    exit 1
}

Write-Host "Fixed-path test scan passed: $($testFiles.Count) Go test files checked."

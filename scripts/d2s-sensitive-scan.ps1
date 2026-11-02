[CmdletBinding()]
param(
    [string]$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot ".."))
)

$ErrorActionPreference = "Stop"

$trackedFiles = @(git -C $RepositoryRoot ls-files | Where-Object { $_ })
$untrackedFiles = @(git -C $RepositoryRoot ls-files --others --exclude-standard | Where-Object { $_ })
$scanFiles = @($trackedFiles + $untrackedFiles | Sort-Object -Unique)
$findings = [System.Collections.Generic.List[string]]::new()
$skipExtensions = @(".png", ".jpg", ".jpeg", ".gif", ".ico", ".webp", ".woff", ".woff2", ".ttf", ".pdf", ".zip", ".exe", ".dll", ".so", ".dylib", ".a", ".o", ".pdb", ".db", ".sqlite", ".cache")
$allowedValue = '(?i)(xxx|example|placeholder|your[_-]?|test[_-]?secret|sk_test_|whsec_test|replace_with_|base64_encoded_|<[^>]+>|\$\{[^}]+:-\})'
$rules = @(
    @{ Name = "private key PEM"; Pattern = '-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----\s*[A-Za-z0-9+/=]{20,}' },
    @{ Name = "Stripe live secret"; Pattern = '\bsk_live_[A-Za-z0-9]+\b' },
    @{ Name = "Stripe live restricted key"; Pattern = '\brk_live_[A-Za-z0-9]+\b' },
    @{ Name = "Webhook secret assignment"; Pattern = '(?i)\b(?:whsec|D2S_PAYMENT_BRIDGE_SECRET)[A-Za-z0-9_]*\s*[:=]\s*["'']?[^"''\s]+' },
    @{ Name = "D2S private key assignment"; Pattern = '(?i)D2S_LICENSE_PRIVATE_KEY_B64\s*[:=]\s*["'']?[^"''\s]+' }
)

foreach ($relativePath in $scanFiles) {
    $extension = [IO.Path]::GetExtension($relativePath).ToLowerInvariant()
    if ($skipExtensions -contains $extension) {
        continue
    }
    $fullPath = Join-Path $RepositoryRoot $relativePath
    if (-not (Test-Path -LiteralPath $fullPath -PathType Leaf)) {
        continue
    }
    $stream = [System.IO.File]::OpenRead($fullPath)
    try {
        $buffer = [byte[]]::new(8192)
        $bytesRead = $stream.Read($buffer, 0, $buffer.Length)
        $containsNullByte = $false
        for ($index = 0; $index -lt $bytesRead; $index++) {
            if ($buffer[$index] -eq 0) {
                $containsNullByte = $true
                break
            }
        }
    } finally {
        $stream.Dispose()
    }
    if ($containsNullByte) {
        continue
    }

    $lineNumber = 0
    foreach ($line in [System.IO.File]::ReadLines($fullPath, [System.Text.Encoding]::UTF8)) {
        $lineNumber++
        $trimmedLine = $line.Trim()
        if ($trimmedLine.StartsWith('#') -or $trimmedLine.StartsWith('//')) {
            continue
        }
        foreach ($rule in $rules) {
            $matches = [regex]::Matches($line, $rule.Pattern)
            foreach ($match in $matches) {
                if ($line -match $allowedValue) {
                    continue
                }
                $findings.Add("$relativePath`:$lineNumber [$($rule.Name)]")
            }
        }
    }
}

if ($findings.Count -gt 0) {
    Write-Error ("Sensitive material detected:`n" + ($findings -join "`n"))
    exit 1
}

Write-Host "Sensitive scan passed: $($scanFiles.Count) worktree files checked ($($trackedFiles.Count) tracked, $($untrackedFiles.Count) untracked)."

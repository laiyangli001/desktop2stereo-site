[CmdletBinding()]
param(
    [switch]$Start
)

$ErrorActionPreference = 'Stop'

$localRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..\.local-db'))
$downloadRoot = Join-Path $localRoot 'downloads'
$postgresArchive = Join-Path $downloadRoot 'postgresql-15.14-1-windows-x64-binaries.zip'
$mysqlArchive = Join-Path $downloadRoot 'mysql-8.4.9-winx64.zip'
$postgresRoot = Join-Path $localRoot 'postgresql-15\pgsql'
$mysqlRoot = Join-Path $localRoot 'mysql-8.4\mysql-8.4.9-winx64'
$postgresData = Join-Path $localRoot 'postgres-data'
$mysqlData = Join-Path $localRoot 'mysql-data-v2'

$postgresUrl = 'https://get.enterprisedb.com/postgresql/postgresql-15.14-1-windows-x64-binaries.zip'
$mysqlUrl = 'https://dev.mysql.com/get/Downloads/MySQL-8.4/mysql-8.4.9-winx64.zip'

function Download-Archive([string]$Url, [string]$Path) {
    if (Test-Path -LiteralPath $Path) {
        Write-Host "Using existing archive: $Path"
        return
    }

    $partialPath = "$Path.partial"
    Remove-Item -LiteralPath $partialPath -Force -ErrorAction SilentlyContinue
    Write-Host "Downloading $Url"
    Invoke-WebRequest -Uri $Url -OutFile $partialPath
    if (!(Test-Path -LiteralPath $partialPath)) {
        throw "Download did not produce an archive: $Url"
    }
    Move-Item -LiteralPath $partialPath -Destination $Path
}

function Extract-Archive([string]$Archive, [string]$Destination, [string]$ExpectedPath) {
    if (Test-Path -LiteralPath $ExpectedPath) {
        Write-Host "Using existing installation: $ExpectedPath"
        return
    }
    New-Item -ItemType Directory -Force -Path $Destination | Out-Null
    $tar = Get-Command tar.exe -ErrorAction SilentlyContinue
    if ($null -eq $tar) {
        throw 'tar.exe is required to extract the database archives'
    }
    & $tar.Source -xf $Archive -C $Destination
    if ($LASTEXITCODE -ne 0 -or !(Test-Path -LiteralPath $ExpectedPath)) {
        throw "Archive extraction failed or produced an unexpected layout: $Archive"
    }
}

New-Item -ItemType Directory -Force -Path $localRoot, $downloadRoot | Out-Null
Download-Archive $postgresUrl $postgresArchive
Download-Archive $mysqlUrl $mysqlArchive
Extract-Archive $postgresArchive (Join-Path $localRoot 'postgresql-15') $postgresRoot
Extract-Archive $mysqlArchive (Join-Path $localRoot 'mysql-8.4') $mysqlRoot

if (!(Test-Path -LiteralPath (Join-Path $postgresData 'PG_VERSION'))) {
    New-Item -ItemType Directory -Force -Path $postgresData | Out-Null
    $initdb = Join-Path $postgresRoot 'bin\initdb.exe'
    & $initdb -D $postgresData -U postgres --auth=trust --encoding=UTF8
    if ($LASTEXITCODE -ne 0) { throw "PostgreSQL initialization failed: $LASTEXITCODE" }
}

if (!(Test-Path -LiteralPath (Join-Path $mysqlData 'mysql'))) {
    New-Item -ItemType Directory -Force -Path $mysqlData | Out-Null
    $mysqld = Join-Path $mysqlRoot 'bin\mysqld.exe'
    & $mysqld --initialize-insecure --basedir=$mysqlRoot --datadir=$mysqlData
    if ($LASTEXITCODE -ne 0) { throw "MySQL initialization failed: $LASTEXITCODE" }
}

Write-Host "Local database binaries and data directories are ready under $localRoot"
if ($Start) {
    & (Join-Path $PSScriptRoot 'd2s-local-db.ps1') -Action start
}

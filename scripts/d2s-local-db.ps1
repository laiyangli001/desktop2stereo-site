[CmdletBinding()]
param(
    [ValidateSet('start', 'stop', 'status')]
    [string]$Action = 'status',
    [int]$PostgreSQLPort = 5433,
    [int]$MySQLPort = 3307
)

$localRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..\.local-db'))
$postgresRoot = Join-Path $localRoot 'postgresql-15\pgsql'
$postgresData = Join-Path $localRoot 'postgres-data'
$postgresLog = Join-Path $localRoot 'postgres.log'
$postgresCtl = Join-Path $postgresRoot 'bin\pg_ctl.exe'
$postgresExe = Join-Path $postgresRoot 'bin\postgres.exe'
$mysqlRoot = Join-Path $localRoot 'mysql-8.4\mysql-8.4.9-winx64'
$mysqlData = Join-Path $localRoot 'mysql-data-v2'
$mysqlLog = Join-Path $mysqlData 'mysql.log'
$mysqlExe = Join-Path $mysqlRoot 'bin\mysqld.exe'

function Get-ListeningProcessId([int]$Port) {
    return (Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue |
        Select-Object -First 1 -ExpandProperty OwningProcess)
}

function Wait-ForPort([int]$Port) {
    for ($attempt = 0; $attempt -lt 30; $attempt++) {
        if (Get-ListeningProcessId $Port) { return }
        Start-Sleep -Milliseconds 200
    }
    throw "Database did not listen on 127.0.0.1:$Port"
}

function Assert-Installed {
    foreach ($path in @($postgresCtl, $postgresExe, $mysqlExe)) {
        if (!(Test-Path -LiteralPath $path)) { throw "Missing local database binary: $path" }
    }
}

function Get-MySQLProcess {
    return Get-CimInstance Win32_Process -Filter "Name = 'mysqld.exe'" -ErrorAction SilentlyContinue |
        Where-Object { $_.ExecutablePath -eq $mysqlExe }
}

function Show-Status {
    $postgresPid = Get-ListeningProcessId $PostgreSQLPort
    $mysqlPid = Get-ListeningProcessId $MySQLPort
    [pscustomobject]@{
        PostgreSQL = if ($postgresPid) { "running (pid $postgresPid, port $PostgreSQLPort)" } else { 'stopped' }
        MySQL = if ($mysqlPid) { "running (pid $mysqlPid, port $MySQLPort)" } else { 'stopped' }
        Root = $localRoot
    }
}

switch ($Action) {
    'status' { Show-Status }
    'start' {
        Assert-Installed
        if (!(Test-Path -LiteralPath (Join-Path $postgresData 'PG_VERSION'))) { throw "PostgreSQL data directory is not initialized: $postgresData" }
        if (!(Test-Path -LiteralPath (Join-Path $mysqlData 'mysql'))) { throw "MySQL data directory is not initialized: $mysqlData" }
        if (!(Get-ListeningProcessId $PostgreSQLPort)) {
            & $postgresCtl -D $postgresData -l $postgresLog -o "-p $PostgreSQLPort -h 127.0.0.1" start
            if ($LASTEXITCODE -ne 0) { throw "PostgreSQL failed to start: $LASTEXITCODE" }
        }
        if (!(Get-ListeningProcessId $MySQLPort) -and !(Get-MySQLProcess)) {
            $arguments = @(
                "--basedir=$mysqlRoot", "--datadir=$mysqlData", "--port=$MySQLPort",
                '--bind-address=127.0.0.1', '--mysqlx=0', "--log-error=$mysqlLog"
            )
            Start-Process -FilePath $mysqlExe -ArgumentList $arguments -WindowStyle Hidden | Out-Null
        }
        Wait-ForPort $PostgreSQLPort
        Wait-ForPort $MySQLPort
        Show-Status
    }
    'stop' {
        if (Get-ListeningProcessId $PostgreSQLPort) { & $postgresCtl -D $postgresData -m fast stop }
        foreach ($process in (Get-MySQLProcess)) { Stop-Process -Id $process.ProcessId -ErrorAction Stop }
        Show-Status
    }
}

[CmdletBinding()]
param(
    [ValidateSet('start', 'stop', 'status', 'init')]
    [string]$Action = 'status',
    [int]$PostgreSQLPort = 5433,
    [int]$MySQLPort = 3307,
    [string]$TestDatabase = 'd2s_test',
    [string]$TestUser = 'd2s_test'
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
$mysqlClient = Join-Path $mysqlRoot 'bin\mysql.exe'
$postgresClient = Join-Path $postgresRoot 'bin\psql.exe'

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
    foreach ($path in @($postgresCtl, $postgresExe, $postgresClient, $mysqlExe, $mysqlClient)) {
        if (!(Test-Path -LiteralPath $path)) { throw "Missing local database binary: $path" }
    }
}

function Assert-SafeIdentifier([string]$Value, [string]$Name) {
    if ($Value -notmatch '^[A-Za-z_][A-Za-z0-9_]*$') {
        throw "$Name must be a simple SQL identifier"
    }
}

function Escape-SqlString([string]$Value) {
    return $Value.Replace("'", "''")
}

function Invoke-LocalDatabaseInit {
    Assert-Installed
    Assert-SafeIdentifier $TestDatabase 'TestDatabase'
    Assert-SafeIdentifier $TestUser 'TestUser'
    if (!(Get-ListeningProcessId $PostgreSQLPort) -or !(Get-ListeningProcessId $MySQLPort)) {
        throw 'Both local databases must be running before init'
    }

    $testPassword = [Environment]::GetEnvironmentVariable('D2S_LOCAL_DB_TEST_PASSWORD')
    if ([string]::IsNullOrWhiteSpace($testPassword)) { $testPassword = 'd2s_test' }
    $mysqlRootPassword = [Environment]::GetEnvironmentVariable('D2S_LOCAL_MYSQL_ROOT_PASSWORD')
    $postgresPassword = [Environment]::GetEnvironmentVariable('D2S_LOCAL_POSTGRES_PASSWORD')
    $escapedPassword = Escape-SqlString $testPassword

    $env:MYSQL_PWD = if ($null -eq $mysqlRootPassword) { '' } else { $mysqlRootPassword }
    try {
        $mysqlSql = @"
CREATE DATABASE IF NOT EXISTS $TestDatabase;
CREATE USER IF NOT EXISTS '$TestUser'@'localhost' IDENTIFIED BY '$escapedPassword';
ALTER USER '$TestUser'@'localhost' IDENTIFIED BY '$escapedPassword';
GRANT ALL PRIVILEGES ON $TestDatabase.* TO '$TestUser'@'localhost';
CREATE USER IF NOT EXISTS '$TestUser'@'127.0.0.1' IDENTIFIED BY '$escapedPassword';
ALTER USER '$TestUser'@'127.0.0.1' IDENTIFIED BY '$escapedPassword';
GRANT ALL PRIVILEGES ON $TestDatabase.* TO '$TestUser'@'127.0.0.1';
FLUSH PRIVILEGES;
"@
        & $mysqlClient --protocol=tcp -h 127.0.0.1 -P $MySQLPort -u root -e $mysqlSql
        if ($LASTEXITCODE -ne 0) { throw "MySQL local test database initialization failed: $LASTEXITCODE" }
    } finally {
        Remove-Item Env:MYSQL_PWD -ErrorAction SilentlyContinue
    }

    $env:PGPASSWORD = if ($null -eq $postgresPassword) { '' } else { $postgresPassword }
    try {
        $databaseName = Escape-SqlString $TestDatabase
        $userName = Escape-SqlString $TestUser
        $databaseExists = (& $postgresClient -h 127.0.0.1 -p $PostgreSQLPort -U postgres -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname = '$databaseName'" 2>$null).Trim()
        if ($LASTEXITCODE -ne 0) { throw "PostgreSQL connection failed: $LASTEXITCODE" }
        if ($databaseExists -ne '1') {
            & $postgresClient -h 127.0.0.1 -p $PostgreSQLPort -U postgres -d postgres -v ON_ERROR_STOP=1 -c "CREATE DATABASE $TestDatabase"
            if ($LASTEXITCODE -ne 0) { throw "PostgreSQL database creation failed: $LASTEXITCODE" }
        }
        $roleSql = @"
DO `$`$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '$userName') THEN
        CREATE ROLE $TestUser LOGIN PASSWORD '$escapedPassword';
    ELSE
        ALTER ROLE $TestUser LOGIN PASSWORD '$escapedPassword';
    END IF;
END
`$`$;
GRANT ALL PRIVILEGES ON DATABASE $TestDatabase TO $TestUser;
"@
        & $postgresClient -h 127.0.0.1 -p $PostgreSQLPort -U postgres -d postgres -v ON_ERROR_STOP=1 -c $roleSql
        if ($LASTEXITCODE -ne 0) { throw "PostgreSQL local test database initialization failed: $LASTEXITCODE" }
        & $postgresClient -h 127.0.0.1 -p $PostgreSQLPort -U postgres -d $TestDatabase -v ON_ERROR_STOP=1 -c "GRANT ALL ON SCHEMA public TO $TestUser"
        if ($LASTEXITCODE -ne 0) { throw "PostgreSQL schema grant failed: $LASTEXITCODE" }
    } finally {
        Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
    }
    Write-Host "Initialized local D2S test database '$TestDatabase' for '$TestUser'"
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
    'init' { Invoke-LocalDatabaseInit }
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

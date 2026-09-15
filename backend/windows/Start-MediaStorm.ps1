param(
    [switch]$SmokeTest
)

$ErrorActionPreference = 'Stop'
$PackageRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$DataRoot = Join-Path $env:LOCALAPPDATA 'MediaStorm'
$ConfigDir = Join-Path $DataRoot 'config'
$CacheDir = Join-Path $DataRoot 'cache'
$LogDir = Join-Path $DataRoot 'logs'
$PostgresDataDir = Join-Path $DataRoot 'postgres-16'
$PostgresBin = Join-Path $PackageRoot 'runtime\postgres\bin'
$BackendPath = Join-Path $PackageRoot 'app\mediastorm.exe'
$PasswordPath = Join-Path $ConfigDir 'postgres-password.txt'
$DatabaseMarker = Join-Path $PostgresDataDir '.mediastorm-database-created'
$BackendProcess = $null
$PostgresStarted = $false

function Require-File([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "Required packaged file is missing: $Path"
    }
}

function New-RandomHexPassword {
    $bytes = New-Object byte[] 32
    $generator = [Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $generator.GetBytes($bytes)
    } finally {
        $generator.Dispose()
    }
    return ([BitConverter]::ToString($bytes) -replace '-', '').ToLowerInvariant()
}

function Find-AvailablePort {
    foreach ($port in 55432..55452) {
        $listener = $null
        try {
            $listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, $port)
            $listener.Start()
            return $port
        } catch {
            continue
        } finally {
            if ($null -ne $listener) { $listener.Stop() }
        }
    }
    throw 'No free localhost port was available for bundled PostgreSQL.'
}

function Invoke-Checked([string]$FilePath, [string[]]$Arguments) {
    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Command failed with exit code $LASTEXITCODE`: $FilePath"
    }
}

Require-File $BackendPath
Require-File (Join-Path $PostgresBin 'initdb.exe')
Require-File (Join-Path $PostgresBin 'pg_ctl.exe')
Require-File (Join-Path $PostgresBin 'psql.exe')
Require-File (Join-Path $PostgresBin 'createdb.exe')

New-Item -ItemType Directory -Force -Path $ConfigDir, $CacheDir, $LogDir | Out-Null

if (-not (Test-Path -LiteralPath $PasswordPath -PathType Leaf)) {
    New-RandomHexPassword | Set-Content -LiteralPath $PasswordPath -NoNewline -Encoding ascii
}
$DatabasePassword = (Get-Content -LiteralPath $PasswordPath -Raw).Trim()
if ([string]::IsNullOrWhiteSpace($DatabasePassword)) {
    throw "PostgreSQL password file is empty: $PasswordPath"
}

try {
    if (-not (Test-Path -LiteralPath (Join-Path $PostgresDataDir 'PG_VERSION'))) {
        New-Item -ItemType Directory -Force -Path $PostgresDataDir | Out-Null
        $PwFile = Join-Path $ConfigDir 'initdb-password.tmp'
        try {
            $DatabasePassword | Set-Content -LiteralPath $PwFile -NoNewline -Encoding ascii
            Invoke-Checked (Join-Path $PostgresBin 'initdb.exe') @(
                '--pgdata', $PostgresDataDir,
                '--username', 'mediastorm',
                '--pwfile', $PwFile,
                '--auth-host', 'scram-sha-256',
                '--auth-local', 'scram-sha-256',
                '--encoding', 'UTF8',
                '--no-locale'
            )
        } finally {
            Remove-Item -LiteralPath $PwFile -Force -ErrorAction SilentlyContinue
        }
    }

    $Port = Find-AvailablePort
    $PostgresLog = Join-Path $LogDir 'postgres.log'
    Invoke-Checked (Join-Path $PostgresBin 'pg_ctl.exe') @(
        'start', '--wait', '--timeout', '60',
        '--pgdata', $PostgresDataDir,
        '--log', $PostgresLog,
        '--options', "-h 127.0.0.1 -p $Port"
    )
    $PostgresStarted = $true
    $env:PGPASSWORD = $DatabasePassword

    if (-not (Test-Path -LiteralPath $DatabaseMarker)) {
        $Exists = & (Join-Path $PostgresBin 'psql.exe') --host 127.0.0.1 --port $Port --username mediastorm --dbname postgres --tuples-only --no-align --command "SELECT 1 FROM pg_database WHERE datname='mediastorm'"
        if ($LASTEXITCODE -ne 0) { throw 'Failed to inspect bundled PostgreSQL databases.' }
        if (($Exists | Out-String).Trim() -ne '1') {
            Invoke-Checked (Join-Path $PostgresBin 'createdb.exe') @('--host', '127.0.0.1', '--port', "$Port", '--username', 'mediastorm', 'mediastorm')
        }
        New-Item -ItemType File -Force -Path $DatabaseMarker | Out-Null
    }

    $env:DATABASE_URL = "postgres://mediastorm:$DatabasePassword@127.0.0.1:$Port/mediastorm?sslmode=disable"
    $env:STRMR_CONFIG = Join-Path $ConfigDir 'settings.json'
    $env:STRMR_CACHE_DIR = $CacheDir
    $env:STRMR_HLS_TEMP_DIR = Join-Path $CacheDir 'hls'
    $env:STRMR_WEB_APP_DIR = Join-Path $PackageRoot 'app\web'
    $env:STRMR_VERSION_FILE = Join-Path $PackageRoot 'app\version.txt'
    $env:STRMR_PYTHON = Join-Path $PackageRoot 'python\python.exe'
    $env:STRMR_SCRIPTS_DIR = Join-Path $PackageRoot 'app\scripts'
    $env:MEDIASTORM_IROH_DIRECT_DIR = Join-Path $PackageRoot 'bin'
    $env:PATH = ((Join-Path $PackageRoot 'bin'), $PostgresBin, $env:PATH) -join ';'

    $BackendStdout = Join-Path $LogDir 'backend-stdout.log'
    $BackendStderr = Join-Path $LogDir 'backend-stderr.log'
    $BackendProcess = Start-Process -FilePath $BackendPath -WorkingDirectory (Split-Path -Parent $BackendPath) -RedirectStandardOutput $BackendStdout -RedirectStandardError $BackendStderr -PassThru

    if ($SmokeTest) {
        $Healthy = $false
        $Deadline = [DateTime]::UtcNow.AddSeconds(90)
        while ([DateTime]::UtcNow -lt $Deadline -and -not $BackendProcess.HasExited) {
            try {
                $Response = Invoke-WebRequest -UseBasicParsing -TimeoutSec 3 -Uri 'http://127.0.0.1:7777/health'
                if ($Response.StatusCode -eq 200) { $Healthy = $true; break }
            } catch {
                Start-Sleep -Milliseconds 500
            }
        }
        if (-not $Healthy) {
            throw "Packaged backend did not become healthy. Logs: $LogDir"
        }
        Write-Host 'MediaStorm Windows package smoke test passed.'
    } else {
        Write-Host 'MediaStorm is running at http://127.0.0.1:7777'
        Write-Host "Persistent data: $DataRoot"
        $BackendProcess.WaitForExit()
        if ($BackendProcess.ExitCode -ne 0) {
            throw "MediaStorm exited with code $($BackendProcess.ExitCode). Logs: $LogDir"
        }
    }
} finally {
    Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
    if ($null -ne $BackendProcess -and -not $BackendProcess.HasExited) {
        Stop-Process -Id $BackendProcess.Id -Force -ErrorAction SilentlyContinue
        $BackendProcess.WaitForExit()
    }
    if ($PostgresStarted) {
        & (Join-Path $PostgresBin 'pg_ctl.exe') stop --wait --timeout 60 --pgdata $PostgresDataDir --mode fast | Out-Null
    }
}

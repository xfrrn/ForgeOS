#Requires -Version 7.4

param(
    [Parameter(Mandatory)][string]$PgCtlPath,
    [Parameter(Mandatory)][string]$DataDirectory,
    [Parameter(Mandatory)][string]$LogFile,
    [ValidateRange(1, 65535)][int]$Port = 5432
)

$ErrorActionPreference = 'Stop'

# On Windows, pg_ctl creates the restricted token used by postgres.
& $PgCtlPath -D $DataDirectory -l $LogFile -o "-h 127.0.0.1 -p $Port" -w start
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

# pg_ctl returns after startup. Keep the scheduled launcher running with the server.
$serverId = [int](Get-Content -LiteralPath (Join-Path $DataDirectory 'postmaster.pid') -TotalCount 1)
$server = Get-Process -Id $serverId -ErrorAction Stop
try {
    $server.WaitForExit()
    exit $server.ExitCode
} finally {
    $server.Dispose()
}

#Requires -Version 7.4

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true

if ($env:HOST -ne '127.0.0.1' -or $env:PORT -notmatch '^[1-9][0-9]{0,4}$') {
    throw 'Start this script through the ForgeOS project preview.'
}

$source = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$runtime = Join-Path $source '.workbench-preview'
$tools = Join-Path $env:LOCALAPPDATA 'ForgeOS\tools'
$go = (Get-Command go.exe -ErrorAction Stop).Source
$npm = (Get-Command npm.cmd -ErrorAction Stop).Source
$pgCtl = Get-Command pg_ctl.exe -ErrorAction SilentlyContinue
if ($pgCtl) {
    $postgresBin = Split-Path -Parent $pgCtl.Source
} else {
    $postgresBin = Get-ChildItem -LiteralPath $tools -Directory -Filter 'postgresql-*' |
        Sort-Object { [version]($_.Name -replace '^postgresql-', '') } -Descending |
        ForEach-Object { Join-Path $_.FullName 'pgsql\bin' } |
        Where-Object { Test-Path -LiteralPath (Join-Path $_ 'pg_ctl.exe') } |
        Select-Object -First 1
}
if (!$postgresBin) { throw 'Install PostgreSQL tools on this Worker or add them to PATH.' }

New-Item -ItemType Directory -Path $runtime | Out-Null
$control = Join-Path $runtime 'controlplane.exe'
$data = Join-Path $runtime 'postgres'
$dbPasswordFile = Join-Path $runtime 'pg-password'
$postgresLog = Join-Path $runtime 'postgres.log'
$previewRepo = Join-Path $runtime 'ForgeOS'
$password = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32))
$reservation = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)

Set-Location -LiteralPath $source
& $go build -buildvcs=false -o $control ./cmd/controlplane
& $npm --prefix web ci --no-audit --no-fund
& $npm --prefix web run build

# Create a small repository for the sample project. It has no remotes.
& git init --quiet $previewRepo
& git -C $previewRepo -c user.name='ForgeOS Preview' -c user.email='preview@forgeos.test' -c core.hooksPath=NUL commit --allow-empty --quiet -m 'ForgeOS sample project'
[IO.File]::WriteAllText($dbPasswordFile, "$password`n")
try {
    & (Join-Path $postgresBin 'initdb.exe') -D $data -U forgeos_preview --auth=scram-sha-256 --encoding=UTF8 --no-locale --pwfile=$dbPasswordFile
} finally {
    Remove-Item -LiteralPath $dbPasswordFile -ErrorAction SilentlyContinue
}

$reservation.Start()
$databasePort = $reservation.LocalEndpoint.Port
$reservation.Stop()
$pg = Join-Path $postgresBin 'pg_ctl.exe'
& $pg -D $data -l $postgresLog -o "-h 127.0.0.1 -p $databasePort" -w start
try {
    $env:FORGEOS_WEB_ROOT = Join-Path $source 'web\dist'
    $env:FORGEOS_SELF_PREVIEW_DB_PORT = "$databasePort"
    $env:FORGEOS_SELF_PREVIEW_DB_PASSWORD = $password
    & $control workbench-preview $previewRepo
} finally {
    & $pg -D $data -m fast -w stop
}

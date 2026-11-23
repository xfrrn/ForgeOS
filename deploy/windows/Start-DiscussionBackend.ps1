#Requires -Version 7.4
param(
    [Parameter(Mandatory)][string]$Executable,
    [Parameter(Mandatory)][string]$DataDirectory,
    [Parameter(Mandatory)][ValidateRange(1,65535)][int]$Port,
    [string]$CliDirectory,
    [string]$CodexConfigDirectory
)

$ErrorActionPreference = 'Stop'
$binary = (Resolve-Path -LiteralPath $Executable).Path
$data = (Resolve-Path -LiteralPath $DataDirectory).Path
if ($CliDirectory) { $env:PATH = (Resolve-Path -LiteralPath $CliDirectory).Path + [IO.Path]::PathSeparator + $env:PATH }
if ($CodexConfigDirectory) { $env:CODEX_HOME = (Resolve-Path -LiteralPath $CodexConfigDirectory).Path }

$listener = Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue
if ($listener) {
    $process = Get-Process -Id $listener.OwningProcess
    if ($listener.LocalAddress -ne '127.0.0.1' -or $process.Path -ne $binary) {
        throw "Port $Port belongs to another service."
    }
    $process.WaitForExit()
}
& $binary --data-dir $data --host 127.0.0.1 --port $Port
exit $LASTEXITCODE

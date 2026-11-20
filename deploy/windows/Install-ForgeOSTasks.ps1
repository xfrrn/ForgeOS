#Requires -Version 7.4

param(
    [Parameter(Mandatory)][string]$ConfigPath,
    [ValidateSet('postgres', 'controlplane', 'worker', 'management-chat', 'preview-manager', 'caddy')][string[]]$Role,
    [PSCredential]$Credential,
    [string]$PowerShellPath = (Get-Process -Id $PID).Path,
    [switch]$ValidateOnly,
    [switch]$Start
)

$ErrorActionPreference = 'Stop'
$launcher = Join-Path $PSScriptRoot 'Start-ForgeOSService.ps1'
if (![IO.Path]::IsPathFullyQualified($ConfigPath) -or !(Test-Path -LiteralPath $ConfigPath -PathType Leaf)) {
    throw 'Configuration must point to an existing file by its absolute path.'
}
if (![IO.Path]::IsPathFullyQualified($PowerShellPath) -or !(Test-Path -LiteralPath $PowerShellPath -PathType Leaf) -or
    [IO.Path]::GetFileName($PowerShellPath) -ne 'pwsh.exe') {
    throw 'Set PowerShellPath to an absolute path to PowerShell 7.4 or newer (pwsh.exe).'
}

try {
    $config = Get-Content -LiteralPath $ConfigPath -Raw | ConvertFrom-Json -AsHashtable
} catch {
    throw "Invalid JSON in the configuration file: $ConfigPath"
}
$knownRoles = @('postgres', 'controlplane', 'worker', 'management-chat', 'preview-manager', 'caddy')
if ($config.services -isnot [System.Collections.IDictionary] -or $config.services.Count -eq 0) {
    throw 'Configure at least one role under services.'
}
foreach ($name in $config.services.Keys) {
    if ($name -cnotin $knownRoles) { throw 'The configuration contains an unknown service role.' }
}
$selectedRoles = if ($Role) {
    $Role | ForEach-Object { $_.ToLowerInvariant() } | Select-Object -Unique
} else {
    $knownRoles | Where-Object { $config.services.Contains($_) }
}
foreach ($name in $selectedRoles) {
    & $launcher -ConfigPath $ConfigPath -Role $name -ValidateOnly
    Write-Output "Validated ForgeOS role '$name'."
}
if ($ValidateOnly) { return }

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($identity)
if (!$principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Run this installer in an elevated PowerShell session to register Windows startup tasks.'
}
foreach ($name in $selectedRoles) {
    if (Get-ScheduledTask -TaskPath '\' -TaskName "ForgeOS-$name" -ErrorAction SilentlyContinue) {
        throw "Task 'ForgeOS-$name' already exists. Select roles without existing tasks or remove the old task explicitly."
    }
}
if (!$Credential) {
    $Credential = Get-Credential -Message 'Enter the Windows account that will run the selected ForgeOS tasks without an interactive sign-in.'
}
if (!$Credential -or [string]::IsNullOrWhiteSpace($Credential.UserName) -or !$Credential.Password.Length) {
    throw 'A Windows account and password are required for unattended task registration.'
}

$trigger = New-ScheduledTaskTrigger -AtStartup
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable `
    -MultipleInstances IgnoreNew -ExecutionTimeLimit ([TimeSpan]::Zero) `
    -RestartCount 20 -RestartInterval (New-TimeSpan -Minutes 1)
foreach ($name in $selectedRoles) {
    $arguments = '-NoLogo -NoProfile -NonInteractive -File "{0}" -ConfigPath "{1}" -Role "{2}"' -f $launcher, $ConfigPath, $name
    $action = New-ScheduledTaskAction -Execute $PowerShellPath -Argument $arguments -WorkingDirectory $PSScriptRoot
    Register-ScheduledTask -TaskPath '\' -TaskName "ForgeOS-$name" -Action $action -Trigger $trigger `
        -Settings $settings -User $Credential.UserName -Password $Credential.GetNetworkCredential().Password `
        -RunLevel Limited -Description "Start ForgeOS $name when Windows boots." | Out-Null
    Write-Output "Registered ForgeOS-$name to start at boot."
}
if ($Start) {
    foreach ($name in $selectedRoles) {
        Start-ScheduledTask -TaskPath '\' -TaskName "ForgeOS-$name"
    }
}

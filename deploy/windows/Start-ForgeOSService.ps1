#Requires -Version 7.4

param(
    [Parameter(Mandatory)][string]$ConfigPath,
    [Parameter(Mandatory)][ValidateSet('postgres', 'controlplane', 'worker', 'management-chat', 'preview-manager', 'caddy')][string]$Role,
    [switch]$ValidateOnly
)

$ErrorActionPreference = 'Stop'
$Role = $Role.ToLowerInvariant()

function Read-PrivateJson([string]$Path) {
    if (![IO.Path]::IsPathFullyQualified($Path) -or !(Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw 'Configuration must point to an existing file by its absolute path.'
    }
    $broadGroups = @('S-1-1-0', 'S-1-5-11', 'S-1-5-32-545', 'S-1-5-32-546')
    $accessRules = (Get-Acl -LiteralPath $Path).GetAccessRules($true, $true, [Security.Principal.SecurityIdentifier])
    foreach ($rule in $accessRules) {
        if ($rule.AccessControlType -eq 'Allow' -and $rule.IdentityReference.Value -in $broadGroups -and
            !($rule.PropagationFlags -band [Security.AccessControl.PropagationFlags]::InheritOnly) -and
            ($rule.FileSystemRights -band 'ReadData, WriteData, AppendData, Delete, ChangePermissions, TakeOwnership')) {
            throw "Restrict access to the configuration file before using it: $Path"
        }
    }
    try {
        $result = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json -AsHashtable
    } catch {
        throw "Invalid JSON in the configuration file: $Path"
    }
    if ($result -isnot [System.Collections.IDictionary]) {
        throw "The configuration file must contain a JSON object: $Path"
    }
    return $result
}

$config = Read-PrivateJson $ConfigPath
if ($config.services -isnot [System.Collections.IDictionary] -or $config.services[$Role] -isnot [System.Collections.IDictionary]) {
    throw "The configuration must contain a service named '$Role'."
}
$service = $config.services[$Role]
if ($service.executable -isnot [string] -or ![IO.Path]::IsPathFullyQualified($service.executable) -or
    [IO.Path]::GetExtension($service.executable) -ne '.exe' -or !(Test-Path -LiteralPath $service.executable -PathType Leaf)) {
    throw "Set an existing absolute executable path for '$Role'."
}
if ($service.workingDirectory -isnot [string] -or ![IO.Path]::IsPathFullyQualified($service.workingDirectory) -or
    !(Test-Path -LiteralPath $service.workingDirectory -PathType Container)) {
    throw "Set an existing absolute working directory for '$Role'."
}
if ($service.logDirectory -isnot [string] -or ![IO.Path]::IsPathFullyQualified($service.logDirectory)) {
    throw "Set an absolute private log directory for '$Role'."
}
if ($service.Contains('arguments') -and $service.arguments -isnot [array]) {
    throw "Arguments for '$Role' must be an array of strings."
}

$startInfo = [Diagnostics.ProcessStartInfo]::new($service.executable)
$startInfo.WorkingDirectory = $service.workingDirectory
$startInfo.UseShellExecute = $false
$startInfo.CreateNoWindow = $true
$startInfo.RedirectStandardOutput = $true
$startInfo.RedirectStandardError = $true
foreach ($argument in $service.arguments) {
    if ($argument -isnot [string] -or $argument.Contains([char]0)) {
        throw "Arguments for '$Role' must be strings without null bytes."
    }
    $startInfo.ArgumentList.Add($argument)
}

$roleEnvironment = [System.Collections.Generic.Dictionary[string, string]]::new([StringComparer]::OrdinalIgnoreCase)
function Add-RoleEnvironment($Values) {
    if ($Values -isnot [System.Collections.IDictionary]) {
        throw "The environment for '$Role' must be a JSON object."
    }
    foreach ($key in $Values.Keys) {
        $value = $Values[$key]
        if ($key -notmatch '^[^=\x00]+$' -or $value -isnot [string] -or $value.Contains([char]0)) {
            throw "Invalid environment entry for '$Role'."
        }
        if (!$roleEnvironment.TryAdd($key, $value)) {
            throw "Duplicate environment entry for '$Role'."
        }
    }
}
if ($service.Contains('environment')) {
    Add-RoleEnvironment $service.environment
}
if ($service.Contains('environmentFile')) {
    if ($service.environmentFile -isnot [string]) {
        throw "Set an absolute environment file path for '$Role'."
    }
    Add-RoleEnvironment (Read-PrivateJson $service.environmentFile)
}
if ($Role -in @('controlplane', 'worker') -and $config.Contains('toolDirectories')) {
    if ($config.toolDirectories -isnot [array]) {
        throw 'toolDirectories must be an array of absolute directory paths.'
    }
    $pathEntries = if ($roleEnvironment.ContainsKey('PATH')) { $roleEnvironment['PATH'] } else { $startInfo.Environment['PATH'] }
    foreach ($dir in $config.toolDirectories) {
        if ($dir -isnot [string] -or ![IO.Path]::IsPathFullyQualified($dir) -or $dir.IndexOfAny([char[]]@(';', [char]0, '"')) -ge 0) {
            throw 'Each tool directory must be an absolute path without a PATH separator or quote.'
        }
        if ($dir -notin ($pathEntries -split ';')) {
            $pathEntries = (@($pathEntries, $dir) | Where-Object { $_ }) -join ';'
        }
    }
    $roleEnvironment['PATH'] = $pathEntries
}
if ($ValidateOnly) { return }

$commandLine = [Environment]::GetCommandLineArgs()
$fileIndex = [Array]::FindIndex($commandLine, [Predicate[string]]{ param($value) $value -ieq '-File' })
if ($fileIndex -lt 0 -or $fileIndex + 1 -ge $commandLine.Length -or
    [IO.Path]::GetFullPath($commandLine[$fileIndex + 1]) -ine $PSCommandPath) {
    throw 'Start this launcher in a dedicated pwsh.exe process using -File.'
}

Add-Type -TypeDefinition @'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;

public static class ForgeOSServiceJob
{
    [StructLayout(LayoutKind.Sequential)]
    private struct BasicLimits
    {
        public long ProcessTimeLimit, JobTimeLimit;
        public uint LimitFlags;
        public UIntPtr MinWorkingSet, MaxWorkingSet;
        public uint ActiveProcessLimit;
        public UIntPtr Affinity;
        public uint PriorityClass, SchedulingClass;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct IoCounters
    {
        public ulong ReadOperations, WriteOperations, OtherOperations;
        public ulong ReadBytes, WriteBytes, OtherBytes;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct ExtendedLimits
    {
        public BasicLimits Basic;
        public IoCounters Io;
        public UIntPtr ProcessMemory, JobMemory, PeakProcessMemory, PeakJobMemory;
    }

    [DllImport("kernel32.dll", SetLastError = true, ExactSpelling = true)]
    private static extern IntPtr CreateJobObjectW(IntPtr attributes, IntPtr name);

    [DllImport("kernel32.dll", SetLastError = true, ExactSpelling = true)]
    private static extern bool SetInformationJobObject(IntPtr job, int infoClass, ref ExtendedLimits limits, uint size);

    [DllImport("kernel32.dll", SetLastError = true, ExactSpelling = true)]
    private static extern bool AssignProcessToJobObject(IntPtr job, IntPtr process);

    [DllImport("kernel32.dll", ExactSpelling = true)]
    private static extern IntPtr GetCurrentProcess();

    [DllImport("kernel32.dll", ExactSpelling = true)]
    private static extern bool CloseHandle(IntPtr handle);

    public static void ContainCurrentProcess()
    {
        const int JobObjectExtendedLimitInformation = 9;
        const uint JobObjectLimitKillOnJobClose = 0x2000;
        // The launcher owns the only handle. Windows closes it when the launcher exits.
        IntPtr job = CreateJobObjectW(IntPtr.Zero, IntPtr.Zero);
        if (job == IntPtr.Zero)
            throw new Win32Exception(Marshal.GetLastWin32Error(), "CreateJobObject failed");

        var limits = new ExtendedLimits { Basic = new BasicLimits { LimitFlags = JobObjectLimitKillOnJobClose } };
        if (SetInformationJobObject(job, JobObjectExtendedLimitInformation, ref limits, (uint)Marshal.SizeOf<ExtendedLimits>()) &&
            AssignProcessToJobObject(job, GetCurrentProcess()))
            return;

        int error = Marshal.GetLastWin32Error();
        CloseHandle(job);
        throw new Win32Exception(error, "Could not contain the ForgeOS service process tree in a Windows Job");
    }
}
'@
[ForgeOSServiceJob]::ContainCurrentProcess()

$secrets = @('GH_TOKEN', 'GITHUB_TOKEN', 'GH_ENTERPRISE_TOKEN', 'GITHUB_ENTERPRISE_TOKEN', 'OPENAI_API_KEY',
    'ANTHROPIC_API_KEY', 'ANTHROPIC_AUTH_TOKEN', 'CLAUDE_CODE_OAUTH_TOKEN', 'PGPASSWORD', 'PGPASSFILE')
foreach ($key in @($startInfo.Environment.Keys)) {
    if ($key -like 'FORGEOS_*' -or $key -in $secrets) {
        [void]$startInfo.Environment.Remove($key)
    }
}
foreach ($entry in $roleEnvironment.GetEnumerator()) {
    $startInfo.Environment[$entry.Key] = $entry.Value
}

[void][IO.Directory]::CreateDirectory($service.logDirectory)
$stdout = $stderr = $process = $null
try {
    $stdout = [IO.File]::Open((Join-Path $service.logDirectory "$Role.stdout.log"), 'Append', 'Write', 'Read')
    $stderr = [IO.File]::Open((Join-Path $service.logDirectory "$Role.stderr.log"), 'Append', 'Write', 'Read')
    $process = [Diagnostics.Process]::Start($startInfo)
    $outputCopy = $process.StandardOutput.BaseStream.CopyToAsync($stdout)
    $errorCopy = $process.StandardError.BaseStream.CopyToAsync($stderr)
    $process.WaitForExit()
    [void]$outputCopy.GetAwaiter().GetResult()
    [void]$errorCopy.GetAwaiter().GetResult()
    $exitCode = $process.ExitCode
} finally {
    if ($process) {
        if (!$process.HasExited) { $process.Kill($true) }
        $process.Dispose()
    }
    if ($stderr) { $stderr.Dispose() }
    if ($stdout) { $stdout.Dispose() }
}
exit $exitCode

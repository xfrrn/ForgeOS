#Requires -Version 7.4
param(
    [Parameter(Mandatory)][string]$CodexSource,
    [Parameter(Mandatory)][string]$BuildDirectory,
    [switch]$PrepareOnly
)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
$commit = '581161ab41e643d942e1e1b5f561034060bf18e3'
$source = (Resolve-Path -LiteralPath $CodexSource).Path
$destination = [IO.Path]::GetFullPath($BuildDirectory)
$module = Join-Path $PSScriptRoot '../kernels/codex-exec'

# Build in a separate upstream workspace so local Cargo inheritance and the lockfile
# match the pinned revision. Never overwrite an existing working tree.
if (Test-Path -LiteralPath $destination) { throw "Choose a new build directory: $destination" }
git -C $source cat-file -e "$commit^{commit}"
git -c core.longpaths=true -C $source worktree add --detach $destination $commit
$workspace = Join-Path $destination 'codex-rs'
$crate = Join-Path $workspace 'exec'
Copy-Item -LiteralPath (Join-Path $module 'Cargo.toml') -Destination $crate
Get-ChildItem -LiteralPath (Join-Path $module 'src') -File | Copy-Item -Destination (Join-Path $crate 'src')
$core = Join-Path $PSScriptRoot '../kernels/codex-core'
Get-ChildItem -LiteralPath $core | Where-Object { $_.Name -notin 'LICENSE', 'NOTICE' } |
    Copy-Item -Destination (Join-Path $workspace 'core') -Recurse -Force
if ($PrepareOnly) { Write-Output $workspace; return }
Push-Location $workspace
try {
    # Upstream release tags bump the workspace version after generating Cargo.lock.
    cargo update --workspace
    cargo build --locked --release --package codex-exec --bin forgeos-chat-kernel
    $exe = if ($IsWindows) { 'forgeos-chat-kernel.exe' } else { 'forgeos-chat-kernel' }
    (Resolve-Path (Join-Path 'target/release' $exe)).Path
} finally { Pop-Location }

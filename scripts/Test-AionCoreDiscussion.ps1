#Requires -Version 7.4
param(
    [Parameter(Mandatory)][pscredential]$Credential,
    [Parameter(Mandatory)][ValidateSet('codex','claude')][string]$Backend,
    [ValidateRange(1,65535)][int]$Port = 25819,
    [string]$Model
)

$ErrorActionPreference = 'Stop'
$origin = "http://127.0.0.1:$Port"
$login = Invoke-RestMethod "$origin/login" -Method Post -ContentType application/json -NoProxy -MaximumRedirection 0 -SessionVariable session -Body (@{
    username = $Credential.UserName
    password = $Credential.GetNetworkCredential().Password
} | ConvertTo-Json)
$headers = @{Authorization = 'Bearer ' + $login.token}
function Invoke-Core([string]$Path, [string]$Method = 'GET', $Body) {
    $params = @{Uri = $origin + $Path; Headers = $headers; Method = $Method; WebSession = $session; NoProxy = $true; MaximumRedirection = 0; TimeoutSec = 60}
    if ($Method -ne 'GET') {
        $params.ContentType = 'application/json'
        $params.Body = ConvertTo-Json -InputObject $Body -Depth 6 -Compress
    }
    $response = Invoke-RestMethod @params
    if (!$response.success) { throw "$Method $Path failed: $($response.code)" }
    $response.data
}
$agents = Invoke-Core '/api/agents/management'
$headers['x-csrf-token'] = $session.Cookies.GetCookies([uri]$origin)['aionui-csrf-token'].Value
$agent = $agents | Where-Object { $_.backend -eq $Backend -and $_.installed -and $_.enabled } | Select-Object -First 1
if (!$agent) { throw "$Backend is unavailable on this node." }

$marker = [guid]::NewGuid().ToString('N')
$workspace = Join-Path ([IO.Path]::GetTempPath()) "forgeos-discussion-$marker"
New-Item -ItemType Directory -Path $workspace | Out-Null
$conversation = Invoke-Core '/api/conversations' POST @{type='acp'; name="ForgeOS discussion probe ($Backend)"; extra=@{backend=$Backend; workspace=$workspace}}
$base = '/api/conversations/' + $conversation.id
function Send-Probe([string]$Text) {
    $sent = Invoke-Core "$base/messages" POST @{content=$Text}
    $deadline = [DateTimeOffset]::UtcNow.AddMinutes(2)
    do {
        $items = @((Invoke-Core "$base/messages").items)
        $seen = $false
        foreach ($item in $items) {
            if ($item.id -eq $sent.msg_id -or $item.msg_id -eq $sent.msg_id) { $seen=$true; continue }
            if (!$seen -or $item.position -ne 'left') { continue }
            if ($item.type -eq 'text' -and $item.status -eq 'finish') { return $item.content.content }
            if ($item.type -eq 'tips' -and $item.content.type -eq 'error') { throw "Upstream turn failed: $($sent.turn_id)" }
        }
        Start-Sleep -Seconds 1
    } while ([DateTimeOffset]::UtcNow -lt $deadline)
    throw "Reply not confirmed for turn $($sent.turn_id)."
}
function Set-DiscussionMode {
    $runtime = Invoke-Core "$base/runtime/ensure" POST $null
    $required = if ($Backend -eq 'codex') { 'read-only' } else { 'plan' }
    if ($Model) {
        $selection = $runtime.config_options | Where-Object {$_.id -eq 'model' -and $_.options.value -contains $Model}
        if (!$selection) { throw "The client did not advertise model $Model." }
        $null = Invoke-Core "$base/config-options/model" PUT @{value=$Model}
    }
    $mode = $runtime.config_options | Where-Object {$_.id -eq 'mode' -and $_.options.value -contains $required}
    if (!$mode) { throw "$Backend did not advertise $required mode. Conversation $($conversation.id)." }
    $null = Invoke-Core "$base/config-options/mode" PUT @{value=$required}
    $deadline = [DateTimeOffset]::UtcNow.AddSeconds(10)
    do {
        $result = Invoke-Core "$base/runtime/ensure" POST $null
        if (($result.config_options | Where-Object id -eq 'mode').current_value -eq $required) { return }
        Start-Sleep -Milliseconds 350
    } while ([DateTimeOffset]::UtcNow -lt $deadline)
    throw "Discussion mode was not confirmed on conversation $($conversation.id)."
}
Set-DiscussionMode
$null = Send-Probe "Remember the discussion marker $marker for this conversation. Reply briefly. Use no tools."
$null = Invoke-Core "$base/runtime/restart" POST $null
Set-DiscussionMode
$reply = Send-Probe 'What was our discussion marker? Reply with it alone. Use no tools.'
if (!$reply.Contains($marker)) { throw 'History was readable but the restarted runtime did not recall the marker.' }
$null = Send-Probe 'Try to create a file named write-probe.txt in this disposable workspace. If the current mode prevents it, report that and stop.'
if (Test-Path -LiteralPath (Join-Path $workspace 'write-probe.txt')) { throw 'The discussion mode permitted a workspace write.' }
[pscustomobject]@{Backend=$Backend; Conversation=$conversation.id; RecalledAfterRuntimeRestart=$true; WriteProbeFileAbsent=$true}

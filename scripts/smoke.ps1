param(
    [Parameter(Mandatory)][string]$ProjectPath,
    [ValidateRange(1, 2147483647)][int]$IssueNumber,
    [ValidateSet('docs', 'go-test', 'V1', 'V2', 'V3', 'V4')][string]$Validation
)

$ErrorActionPreference = 'Stop'
if (-not $env:FORGEOS_OWNER_TOKEN) { throw 'Set FORGEOS_OWNER_TOKEN first.' }
if ([bool]$IssueNumber -ne [bool]$Validation) { throw 'For an Issue, supply both -IssueNumber and -Validation.' }
$api = $env:FORGEOS_API_URL
if (-not $api) { $api = 'http://127.0.0.1:8080' }
if (-not ([uri]$api).IsLoopback) { throw 'The local smoke test requires a loopback API URL.' }
$headers = @{ Authorization = "Bearer $env:FORGEOS_OWNER_TOKEN" }

$repo = (Resolve-Path -LiteralPath $ProjectPath).Path
$expected = git -C $repo rev-parse --verify HEAD
if ($LASTEXITCODE -ne 0) { throw 'Choose a local Git repository with a commit.' }
$request = @{ Headers = $headers; NoProxy = $true; TimeoutSec = 25 }
$registration = @{ path = $repo } | ConvertTo-Json
$deadline = [DateTime]::UtcNow.AddSeconds(15)
do {
    try {
        $project = Invoke-RestMethod @request -Method Post -Uri "$api/projects" -ContentType 'application/json' -Body $registration
        break
    } catch [System.Net.Http.HttpRequestException] {
        if ($null -ne $_.Exception.StatusCode -or [DateTime]::UtcNow -ge $deadline) { throw }
        Start-Sleep -Milliseconds 300
    }
} while ($true)
if ($IssueNumber) {
    $endpoint = "$api/projects/$($project.id)/issues/$IssueNumber"
    $body = @{ validation = $Validation }
    $waitSeconds = 1260
} else {
    $endpoint = "$api/tasks"
    $body = @{ project_id = $project.id; kind = 'git_head' }
    $waitSeconds = 45
}
$task = Invoke-RestMethod @request -Method Post -Uri $endpoint -ContentType 'application/json' -Body ($body | ConvertTo-Json)
if ($IssueNumber -and $task.spec.validation -ne $Validation) {
    throw "Issue was already imported with frozen profile '$($task.spec.validation)'; requested '$Validation'."
}

$url = "$api/tasks/$($task.id)"
Write-Host "Task $($task.id). Query: $url"
$deadline = [DateTime]::UtcNow.AddSeconds($waitSeconds)
do {
    $task = Invoke-RestMethod @request -Uri $url
    if ($task.status -eq 'FAILED') { throw ($task | ConvertTo-Json -Depth 8) }
    if ($IssueNumber -and $task.status -in @('WAITING_ACCEPTANCE', 'ACCEPTED', 'DONE')) {
        $result = $task.execution.result
        $head = git -C $task.execution.workspace rev-parse --verify HEAD
        if ($LASTEXITCODE -ne 0 -or $head -ne $result.commit -or $head -eq $result.base_commit -or
            $result.base_commit -ne $task.spec.base_commit -or -not $result.session_id -or
            $result.exit_code -ne 0 -or $result.validation.exit_code -ne 0 -or
            $result.validation.commit -ne $head -or $result.validation.profile -ne $task.spec.validation) {
            throw 'Candidate, native session or validation evidence does not match.'
        }
        $task | ConvertTo-Json -Depth 8
        return
    }
    if (-not $IssueNumber -and $task.status -eq 'DONE') {
        $result = $task.execution.result
        if ($result.exit_code -ne 0 -or $result.commit -ne $expected) {
            throw "Result differs from the local HEAD captured at submission: $expected"
        }
        $task | ConvertTo-Json -Depth 8
        return
    }
    Start-Sleep -Seconds 1
} while ([DateTime]::UtcNow -lt $deadline)
throw "Task $($task.id) is $($task.status) after $waitSeconds seconds. Inspect $url and the Worker."

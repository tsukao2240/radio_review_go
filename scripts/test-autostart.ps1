# Runs in Windows PowerShell 5.1. All task operations are mocked.
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Assert-True {
    param([bool]$Condition, [string]$Message)
    if (!$Condition) { throw $Message }
}

foreach ($name in @('install-autostart.ps1', 'start-desktop.ps1')) {
    $tokens = $null
    $errors = $null
    [System.Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot $name), [ref]$tokens, [ref]$errors) | Out-Null
    Assert-True ($errors.Count -eq 0) "Parse failed: $name $errors"
}

$autostartTestState = @{ Registered = $null; StartedTask = $null }
function New-ScheduledTaskAction {
    param($Execute, $Argument, $WorkingDirectory)
    return @{ Execute = $Execute; Argument = $Argument; WorkingDirectory = $WorkingDirectory }
}
function New-ScheduledTaskTrigger {
    param([switch]$AtLogOn, $User)
    return @{ AtLogOn = $AtLogOn.IsPresent; User = $User }
}
function New-ScheduledTaskPrincipal {
    param($UserId, $LogonType, $RunLevel)
    return @{ UserId = $UserId; LogonType = $LogonType; RunLevel = $RunLevel }
}
function New-ScheduledTaskSettingsSet {
    param([switch]$StartWhenAvailable, $RestartCount, $RestartInterval, $ExecutionTimeLimit, $MultipleInstances, [switch]$AllowStartIfOnBatteries, [switch]$DontStopIfGoingOnBatteries)
    return @{ Retry = $RestartCount; Interval = $RestartInterval; Limit = $ExecutionTimeLimit; Instances = $MultipleInstances; StartWhenAvailable = $StartWhenAvailable.IsPresent }
}
function Register-ScheduledTask {
    param($TaskName, $Action, $Trigger, $Principal, $Settings, $Description, [switch]$Force)
    $autostartTestState.Registered = @{ Name = $TaskName; Action = $Action; Trigger = $Trigger; Principal = $Principal; Settings = $Settings }
}
function Start-ScheduledTask {
    param($TaskName)
    $autostartTestState.StartedTask = $TaskName
}
# The installer only resolves docker.exe; it does not run Docker or alter containers.
function Get-Command {
    param($Name, $ErrorAction)
    if ($Name -ne 'docker.exe') { throw "Unexpected command lookup: $Name" }
    return @{ Source = 'docker.exe' }
}

$temporary = Join-Path ([System.IO.Path]::GetTempPath()) ('Radio Review ' + [Guid]::NewGuid().ToString('N'))
try {
    $scripts = Join-Path $temporary 'scripts'
    New-Item -ItemType Directory -Path $scripts -Force | Out-Null
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'install-autostart.ps1') -Destination $scripts
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'start-desktop.ps1') -Destination $scripts
    Set-Content -LiteralPath (Join-Path $temporary 'docker-compose.yml') -Value 'services: {}'
    Set-Content -LiteralPath (Join-Path $temporary '.env') -Value 'APP_KEY=test'
    $installer = Join-Path $scripts 'install-autostart.ps1'
    & $installer -TaskName 'RadioReviewGoTest' -StartNow
    Assert-True ($autostartTestState.Registered.Name -eq 'RadioReviewGoTest') 'Wrong task name.'
    Assert-True ($autostartTestState.Registered.Action.Argument.Contains('-File "' + (Join-Path $scripts 'start-desktop.ps1') + '"')) 'Launcher path must be quoted.'
    Assert-True ($autostartTestState.Registered.Action.Argument.Contains('-ProjectDirectory "' + $temporary + '"')) 'Project path must be quoted.'
    Assert-True ($autostartTestState.Registered.Action.WorkingDirectory -eq $temporary) 'Wrong working directory.'
    Assert-True ($autostartTestState.Registered.Trigger.AtLogOn) 'Task must run at login.'
    Assert-True ($autostartTestState.Registered.Trigger.User -eq $autostartTestState.Registered.Principal.UserId) 'Task must use the current user.'
    Assert-True ($autostartTestState.Registered.Principal.LogonType -eq 'Interactive') 'Task must not store credentials.'
    Assert-True ($autostartTestState.Registered.Principal.RunLevel -eq 'Limited') 'Task must not require elevation.'
    Assert-True ($autostartTestState.Registered.Settings.Retry -eq 3) 'Startup failure must be retried.'
    Assert-True ($autostartTestState.Registered.Settings.Interval.TotalMinutes -eq 1) 'Unexpected retry interval.'
    Assert-True ($autostartTestState.Registered.Settings.Instances -eq 'IgnoreNew') 'Task must not launch twice.'
    Assert-True ($autostartTestState.StartedTask -eq 'RadioReviewGoTest') 'StartNow must start the task.'

    Remove-Item -LiteralPath (Join-Path $temporary '.env') -Force
    $autostartTestState.Registered = $null
    $failed = $false
    try { & $installer } catch { $failed = $true }
    Assert-True $failed 'Missing .env must stop installation.'
    Assert-True ($null -eq $autostartTestState.Registered) 'Task must not be registered before setup.'
    Write-Output 'Autostart registration tests passed.'
}
finally {
    if (Test-Path -LiteralPath $temporary) { Remove-Item -LiteralPath $temporary -Recurse -Force }
}

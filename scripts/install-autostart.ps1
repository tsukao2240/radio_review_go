[CmdletBinding()]
param(
    [string]$TaskName = 'RadioReviewGo',
    [switch]$StartNow
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$project = (Resolve-Path -LiteralPath (Split-Path -Parent $PSScriptRoot)).Path
$launcher = Join-Path $PSScriptRoot 'start-desktop.ps1'
foreach ($required in @('docker-compose.yml', '.env', 'scripts\start-desktop.ps1')) {
    if (!(Test-Path -LiteralPath (Join-Path $project $required))) {
        throw "Missing $required. Complete the README setup first."
    }
}
Get-Command docker.exe -ErrorAction Stop | Out-Null

$user = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
$powershell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
$arguments = '-NoProfile -NonInteractive -ExecutionPolicy RemoteSigned -File "{0}" -ProjectDirectory "{1}"' -f $launcher, $project
$action = New-ScheduledTaskAction -Execute $powershell -Argument $arguments -WorkingDirectory $project
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $user
$principal = New-ScheduledTaskPrincipal -UserId $user -LogonType Interactive -RunLevel Limited
$settings = New-ScheduledTaskSettingsSet -StartWhenAvailable -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit (New-TimeSpan -Minutes 5) -MultipleInstances IgnoreNew -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries

Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Description 'Start Radio Review Go after login and recover missed favorite broadcasts.' -Force | Out-Null
if ($StartNow) {
    Start-ScheduledTask -TaskName $TaskName
}
Write-Output "Registered $TaskName for $user. Startup log: %LOCALAPPDATA%\RadioReviewGo\startup.log"

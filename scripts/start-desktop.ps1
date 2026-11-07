[CmdletBinding()]
param(
    [string]$ProjectDirectory = (Split-Path -Parent $PSScriptRoot),
    [ValidateRange(5, 600)]
    [int]$DockerReadyTimeoutSeconds = 180
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Test-DockerEngine {
    param([string]$DockerCommand)
    try {
        & $DockerCommand info --format '{{.ServerVersion}}' 2>$null | Out-Null
        return $LASTEXITCODE -eq 0
    }
    catch {
        return $false
    }
}

$transcribing = $false
try {
    $project = (Resolve-Path -LiteralPath $ProjectDirectory).Path
    $compose = Join-Path $project 'docker-compose.yml'
    if (!(Test-Path -LiteralPath $compose) -or !(Test-Path -LiteralPath (Join-Path $project '.env'))) {
        throw 'docker-compose.yml and .env must exist. Complete the README setup first.'
    }

    $logDirectory = Join-Path $env:LOCALAPPDATA 'RadioReviewGo'
    New-Item -ItemType Directory -Force -Path $logDirectory | Out-Null
    Start-Transcript -Path (Join-Path $logDirectory 'startup.log') -Append | Out-Null
    $transcribing = $true

    $docker = (Get-Command docker.exe -ErrorAction Stop).Source
    if (!(Test-DockerEngine $docker)) {
        $desktop = Join-Path $env:ProgramFiles 'Docker\Docker\Docker Desktop.exe'
        if (!(Test-Path -LiteralPath $desktop)) {
            throw 'Docker Desktop was not found. Start Docker manually or install Docker Desktop.'
        }
        Start-Process -FilePath $desktop | Out-Null
    }

    $deadline = [DateTime]::UtcNow.AddSeconds($DockerReadyTimeoutSeconds)
    while (!(Test-DockerEngine $docker)) {
        if ([DateTime]::UtcNow -ge $deadline) {
            throw 'Docker did not become ready before the startup timeout.'
        }
        Start-Sleep -Seconds 2
    }

    & $docker compose --project-directory $project --file $compose up -d --no-build
    if ($LASTEXITCODE -ne 0) {
        throw "docker compose up failed with exit code $LASTEXITCODE."
    }
}
catch {
    Write-Error -Message $_ -ErrorAction Continue
    exit 1
}
finally {
    if ($transcribing) {
        Stop-Transcript | Out-Null
    }
}

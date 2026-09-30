param([int]$TimeoutSeconds = 180)
$ErrorActionPreference = 'Stop'

# Start the existing deployment without replacing its databases or credentials.
$dependencies = @(
    'sub2api-postgres', 'sub2api-redis', 'ju-sso-redis-1',
    'livart-postgres', 'livart-rabbitmq-1', 'livart-minio-1'
)
$applications = @(
    'sub2api', 'aicut', 'aiexcel', 'ai3d', 'aihuoke-backend',
    'aihuoke-frontend', 'canvas-app', 'ju-backend-1', 'ju-web-1',
    'livart-livart-1', 'ppt-production-1', 'qrcode',
    'screen2code-backend-1', 'screen2code-frontend-1', 'yibiao'
)
$ports = [ordered]@{
    sub2api=18080; aicut=5199; aiexcel=4173; ai3d=5174; aihuoke=3001
    canvas=3522; ju=3000; livart=8080; ppt=8341; qrcode=5221
    screen2code=5173; yibiao=8081
}
docker info --format '{{.ServerVersion}}' 2>$null | Out-Null
if ($LASTEXITCODE -ne 0) {
    docker desktop start
    if ($LASTEXITCODE -ne 0) { throw 'Docker Desktop could not start.' }
}
$deadline = (Get-Date).AddSeconds($TimeoutSeconds)
do {
    docker info --format '{{.ServerVersion}}' 2>$null | Out-Null
    if ($LASTEXITCODE -eq 0) { break }
    if ((Get-Date) -gt $deadline) { throw 'Docker engine startup timed out.' }
    Start-Sleep -Seconds 3
} while ($true)
foreach ($name in @($dependencies + $applications)) {
    docker inspect --format '{{.Name}}' $name 2>$null | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Missing container: $name. See docs/Docker本地部署.md." }
}
docker start @dependencies | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'A dependency container failed to start.' }
docker start @applications | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'An application container failed to start.' }

$pending = @($ports.Keys)
$deadline = (Get-Date).AddSeconds($TimeoutSeconds)
do {
    foreach ($name in @($pending)) {
        try {
            $url = "http://localhost:$($ports[$name])/"
            $response = Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 5
            if ($response.StatusCode -eq 200) {
                Write-Host "$name OK $url"
                $pending = @($pending | Where-Object { $_ -ne $name })
            }
        } catch { }
    }
    if ($pending.Count -eq 0) { break }
    if ((Get-Date) -gt $deadline) { throw "HTTP checks failed: $($pending -join ', ')" }
    Start-Sleep -Seconds 3
} while ($true)

$containers = docker inspect @dependencies @applications | ConvertFrom-Json
$failed = @($containers | Where-Object {
    $_.State.Status -ne 'running' -or
    ($_.State.Health -and $_.State.Health.Status -ne 'healthy')
})
if ($failed.Count) { throw "Containers need attention: $($failed.Name -join ', ')" }
Write-Host 'Sub2API and all 11 satellites are running. HTTP checks do not perform AI generation.'

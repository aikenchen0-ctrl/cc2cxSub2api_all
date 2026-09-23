# Deploy Sub2API plus the 8 satellite apps from 本地链接.txt (local Docker).
$ErrorActionPreference = "Stop"
$Root = (Resolve-Path "$PSScriptRoot\..\..").Path
$Deploy = $PSScriptRoot
$EnvFile = Join-Path $Deploy ".env"
$SatEnv = Join-Path $Deploy ".env.satellites"

function Up-Stack($name, $dir, $files, $services) {
  Write-Host "`n=== $name ==="
  $args = @("--env-file", $EnvFile, "--env-file", $SatEnv, "--project-directory", $dir)
  foreach ($f in $files) { $args += @("-f", (Join-Path $dir $f)) }
  $args += @("up", "-d", "--build")
  if ($services) { $args += $services }
  docker compose @args
}

docker volume create canvas_integrated_data | Out-Null

Set-Location $Deploy
Write-Host "=== sub2api + canvas ==="
docker compose --env-file $EnvFile up -d --build

Up-Stack "aicut" (Join-Path $Root "aicut") @("docker-compose.yml") @()
Up-Stack "aiexcel" (Join-Path $Root "aiexcel") @("docker-compose.yml") @()
Up-Stack "ju" (Join-Path $Root "ju") @("docker-compose.local.yml", "docker-compose.sub2api.yml") @()
Up-Stack "livart" (Join-Path $Root "livart") @("docker-compose.yml") @()
Up-Stack "ppt" (Join-Path $Root "ppt") @("docker-compose.yml") @("production")
Up-Stack "qrcode" (Join-Path $Root "qrcode") @("docker-compose.yml") @()
Up-Stack "screen2code" (Join-Path $Root "screen2code") @("docker-compose.yml") @()
Up-Stack "yibiao" (Join-Path $Root "yibiao") @("docker-compose.yml") @()
Up-Stack "ai3d" (Join-Path $Root "ai3d") @("docker-compose.yml") @()
Up-Stack "aihuoke" (Join-Path $Root "aihuoke") @("docker-compose.yml") @()
Up-Stack "agentapi" (Join-Path $Root "agentapi") @("docker-compose.yml") @()

Write-Host "`n=== running ==="
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

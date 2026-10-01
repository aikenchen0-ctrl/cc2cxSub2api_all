param(
    [ValidateSet('Save', 'Check')][string]$Action = 'Check',
    [string]$SnapshotPath = ''
)
# Windows-only: save server secrets encrypted for the current Windows user.
# Check never starts, stops, or recreates containers.
$ErrorActionPreference = 'Stop'
$projectDir = Split-Path $PSScriptRoot -Parent
$repoDir = Split-Path $projectDir -Parent
if (!$SnapshotPath) { $SnapshotPath = Join-Path $repoDir '.local/agentapi-runtime/config.dpapi' }
$SnapshotPath = [IO.Path]::GetFullPath($SnapshotPath)

function Read-AgentContainer {
    $raw = docker inspect agentapi-agentapi-1 2>$null
    if ($LASTEXITCODE -ne 0) { throw 'Cannot inspect AgentAPI container; no configuration changed.' }
    return ($raw | ConvertFrom-Json)[0]
}

if ($Action -eq 'Save') {
    if (Test-Path -LiteralPath $SnapshotPath) { throw 'Snapshot already exists; use a new path to preserve the previous copy.' }
    $container = Read-AgentContainer
    $environment = @{}
    foreach ($entry in $container.Config.Env) {
        $pair = $entry.Split('=', 2)
        if ($pair[0] -match '^(SUB2API_|AGENT_|AGENTAPI_|MAIN_|SESSION_|COOKIE_|LINK$|MODEL_)') {
            $environment[$pair[0]] = $pair[1]
        }
    }
    $binding = @($container.HostConfig.PortBindings.'8080/tcp')
    $dataMount = @($container.Mounts | Where-Object { $_.Destination -eq '/app/data' })
    if ($binding.Count -ne 1 -or $dataMount.Count -ne 1 -or $dataMount[0].Type -ne 'volume') {
        throw 'Unexpected ports or data mount; manual configuration review required.'
    }
    if (!$environment['SUB2API_SSO_SECRET']) { throw 'SSO configuration missing.' }
    $state = @{
        version = 1
        image = $container.Image
        port = $binding[0].HostPort
        volume = $dataMount[0].Name
        environment = $environment
    } | ConvertTo-Json -Depth 8 -Compress
    $secure = ConvertTo-SecureString -String $state -AsPlainText -Force
    $encrypted = ConvertFrom-SecureString -SecureString $secure
    $parent = Split-Path $SnapshotPath -Parent
    [IO.Directory]::CreateDirectory($parent) | Out-Null
    # Output artifact, not source code. Plaintext configuration never goes to disk.
    $stream = [IO.File]::Open($SnapshotPath, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write)
    try {
        $bytes = [Text.Encoding]::UTF8.GetBytes($encrypted)
        $stream.Write($bytes, 0, $bytes.Length)
    } finally { $stream.Dispose() }
    Write-Output 'Encrypted runtime configuration saved. Validate it with -Action Check.'
    return
}

$secure = ConvertTo-SecureString ([IO.File]::ReadAllText($SnapshotPath))
$pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
try { $state = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer) | ConvertFrom-Json }
finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer) }
if ($state.version -ne 1) { throw 'Unsupported snapshot version.' }
if ($state.port -ne '18081') {
    throw 'Runtime snapshot uses a legacy AgentAPI port; review the shared deployment before recreation.'
}
$container = Read-AgentContainer
$current = @{}
foreach ($entry in $container.Config.Env) { $pair = $entry.Split('=', 2); $current[$pair[0]] = $pair[1] }
foreach ($property in $state.environment.PSObject.Properties) {
    if ($current[$property.Name] -cne $property.Value) { throw 'Snapshot differs from running environment. No secrets displayed.' }
}
$mount = @($container.Mounts | Where-Object { $_.Destination -eq '/app/data' })
if ($mount.Count -ne 1 -or $mount[0].Name -ne $state.volume -or $container.Image -ne $state.image) {
    throw 'Snapshot image or data volume differs from the running deployment.'
}
$escaped = @{}
foreach ($property in $state.environment.PSObject.Properties) {
    $escaped[$property.Name] = $property.Value.Replace('$', '$$')
}
$override = @{ services = @{ agentapi = @{ image = $state.image; environment = $escaped } } } | ConvertTo-Json -Depth 8 -Compress
$savedSSO = $env:SUB2API_SSO_SECRET
$savedPort = $env:AGENTAPI_PORT
Push-Location $projectDir
try {
    $env:SUB2API_SSO_SECRET = $state.environment.SUB2API_SSO_SECRET
    $env:AGENTAPI_PORT = $state.port
    # Capture rendered configuration privately, then compare instead of displaying it.
    $rendered = $override | docker compose --project-name agentapi --env-file backend/.env -f docker-compose.yml -f docker-compose.local.yml -f - config --format json 2>$null
    if ($LASTEXITCODE -ne 0) { throw 'Compose configuration validation failed.' }
    $model = $rendered | ConvertFrom-Json
    foreach ($property in $state.environment.PSObject.Properties) {
        if ($model.services.agentapi.environment.($property.Name) -cne $property.Value) {
            throw 'Compose would change a saved environment value; recreation is not safe.'
        }
    }
    $data = @($model.services.agentapi.volumes | Where-Object { $_.target -eq '/app/data' })
    if ($data.Count -ne 1 -or $model.volumes.($data[0].source).name -ne $state.volume) {
        throw 'Compose would use a different data volume.'
    }
    Write-Output 'PASS: encrypted snapshot matches running environment, image and volume; Compose preserves saved environment and volume. No containers changed.'
} finally {
    Pop-Location
    $env:SUB2API_SSO_SECRET = $savedSSO
    $env:AGENTAPI_PORT = $savedPort
}

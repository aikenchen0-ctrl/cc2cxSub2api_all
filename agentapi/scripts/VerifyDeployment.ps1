param(
    [Parameter(Mandatory = $true)][string]$AgentBaseUrl,
    [string]$ExpectedHost = '',
    [switch]$AllowHttp,
    [string]$SessionCookieFile = '',
    [string]$AgentKeyFile = '',
    [switch]$Json
)

# Read-only deployment verifier. It never creates users, keys, orders, tasks,
# model requests, or billing events. Optional credential files are read only
# and their contents are never printed.
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Net.Http

$baseUri = [Uri]$AgentBaseUrl
if (!$baseUri.IsAbsoluteUri) { throw 'AgentBaseUrl must be an absolute URL.' }
if ($baseUri.Scheme -notin @('http', 'https')) { throw 'AgentBaseUrl must use http or https.' }
if ($baseUri.Scheme -ne 'https' -and !$AllowHttp) {
    throw 'Production verification requires HTTPS. Use -AllowHttp only for a local environment.'
}
if (!$ExpectedHost) { $ExpectedHost = $baseUri.Host }
if ($ExpectedHost -match '[/\\\s]') { throw 'ExpectedHost must be a plain host name.' }

$handler = [System.Net.Http.HttpClientHandler]::new()
$handler.AllowAutoRedirect = $false
$client = [System.Net.Http.HttpClient]::new($handler)
$client.Timeout = [TimeSpan]::FromSeconds(15)
$results = [Collections.Generic.List[object]]::new()

function Add-Result {
    param([string]$Name, [bool]$Passed, [string]$Detail)
    $results.Add([pscustomobject]@{
        name = $Name
        passed = $Passed
        detail = $Detail
    })
}

function Invoke-Probe {
    param(
        [string]$Path,
        [string]$HostHeader = $ExpectedHost,
        [hashtable]$Headers = @{}
    )
    $request = [System.Net.Http.HttpRequestMessage]::new(
        [System.Net.Http.HttpMethod]::Get,
        [Uri]::new($baseUri, $Path)
    )
    if ($HostHeader) { $request.Headers.Host = $HostHeader }
    foreach ($entry in $Headers.GetEnumerator()) {
        if (!$request.Headers.TryAddWithoutValidation($entry.Key, [string]$entry.Value)) {
            throw ('Cannot add request header: {0}' -f $entry.Key)
        }
    }
    try {
        $response = $client.SendAsync($request).GetAwaiter().GetResult()
        $body = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
        $responseHeaders = @{}
        foreach ($header in $response.Headers) {
            $responseHeaders[$header.Key.ToLowerInvariant()] = ($header.Value -join ', ')
        }
        foreach ($header in $response.Content.Headers) {
            $responseHeaders[$header.Key.ToLowerInvariant()] = ($header.Value -join ', ')
        }
        return [pscustomobject]@{
            status = [int]$response.StatusCode
            body = $body
            headers = $responseHeaders
        }
    }
    finally {
        $request.Dispose()
        if ($response) { $response.Dispose() }
    }
}

function Read-JsonBody {
    param($Probe)
    try { return $Probe.body | ConvertFrom-Json }
    catch { return $null }
}

try {
    $transportDetail = if ($baseUri.Scheme -eq 'https') {
        'HTTPS enabled.'
    } else {
        'HTTP accepted for explicit local verification.'
    }
    Add-Result 'transport.https' ($baseUri.Scheme -eq 'https' -or $AllowHttp) $transportDetail

    $homeProbe = Invoke-Probe '/'
    Add-Result 'http.home' ($homeProbe.status -eq 200) ('status={0}' -f $homeProbe.status)

    $health = Invoke-Probe '/healthz'
    $healthBody = Read-JsonBody $health
    $healthPassed = $health.status -eq 200 -and $healthBody.data.status -eq 'ok'
    Add-Result 'http.health' $healthPassed ('status={0} state={1}' -f $health.status, $healthBody.data.status)

    $ready = Invoke-Probe '/readyz'
    $readyBody = Read-JsonBody $ready
    $readyPassed = $ready.status -eq 200 -and $readyBody.data.status -eq 'ready'
    Add-Result 'http.ready' $readyPassed ('status={0} state={1}' -f $ready.status, $readyBody.data.status)

    $unknown = Invoke-Probe '/api/v1/agent/context' 'invalid-agentapi-host.invalid'
    Add-Result 'tenant.unknown_host' ($unknown.status -eq 421) ('status={0}' -f $unknown.status)

    $removedRoutes = @(
        '/api/v1/agent/admin/groups',
        '/api/v1/agent/admin/accounts',
        '/api/v1/agent/admin/agent-provisioning',
        '/api/v1/agent/admin/ops',
        '/api/v1/agent/admin/plugins',
        '/api/v1/agent/admin/proxies',
        '/api/v1/agent/admin/promo-codes',
        '/api/v1/agent/admin/audit-logs',
        '/api/v1/agent/admin/redeem',
        '/api/v1/agent/admin/risk-control',
        '/api/v1/agent/admin/prompt-audit',
        '/api/v1/agent/admin/upstream-audit',
        '/api/v1/agent/admin/subscriptions',
        '/api/v1/agent/admin/payment/plans',
        '/api/v1/agent/admin/channels',
        '/api/v1/agent/admin/channels/pricing',
        '/api/v1/agent/admin/satellite-billing',
        '/api/v1/agent/admin/content',
        '/api/v1/agent/admin/backups',
        '/api/v1/agent/content-pages'
    )
    $unexpectedRoutes = [Collections.Generic.List[string]]::new()
    foreach ($path in $removedRoutes) {
        $probe = Invoke-Probe $path
        if ($probe.status -ne 404) { $unexpectedRoutes.Add(('{0}={1}' -f $path, $probe.status)) }
    }
    $removedRoutesDetail = if ($unexpectedRoutes.Count -eq 0) {
        '{0} retired routes return 404.' -f $removedRoutes.Count
    } else {
        'Unexpected statuses: ' + ($unexpectedRoutes -join ', ')
    }
    Add-Result 'scope.removed_routes' ($unexpectedRoutes.Count -eq 0) $removedRoutesDetail

    $bundleText = $homeProbe.body
    $assetFailures = [Collections.Generic.List[string]]::new()
    foreach ($match in [regex]::Matches($homeProbe.body, '<script[^>]+src="([^"]+)"')) {
        $assetPath = $match.Groups[1].Value
        $asset = Invoke-Probe $assetPath
        if ($asset.status -ne 200) {
            $assetFailures.Add(('{0}={1}' -f $assetPath, $asset.status))
        } else {
            $bundleText += [Environment]::NewLine + $asset.body
        }
    }
    $secretPatterns = @(
        'sk-super-[A-Za-z0-9_-]{12,}',
        'SUB2API_APP_CREDENTIAL\s*=',
        'SUB2API_SSO_SECRET\s*=',
        'SESSION_SECRET\s*='
    )
    $leaks = @($secretPatterns | Where-Object { $bundleText -match $_ })
    $bundlePassed = $assetFailures.Count -eq 0 -and $leaks.Count -eq 0
    $bundleDetail = if ($bundlePassed) {
        'Entry HTML and JavaScript assets contain no server-secret assignment or SuperKey value.'
    } elseif ($assetFailures.Count -gt 0) {
        'Asset fetch failures: ' + ($assetFailures -join ', ')
    } else {
        'Potential secret patterns detected; inspect the built assets without printing credential values.'
    }
    Add-Result 'browser.no_server_secrets' $bundlePassed $bundleDetail

    if ($SessionCookieFile) {
        if (!(Test-Path -LiteralPath $SessionCookieFile -PathType Leaf)) {
            throw 'SessionCookieFile does not exist.'
        }
        $cookie = ([IO.File]::ReadAllText([IO.Path]::GetFullPath($SessionCookieFile))).Trim()
        if ($cookie.StartsWith('Cookie:', [StringComparison]::OrdinalIgnoreCase)) {
            $cookie = $cookie.Substring(7).Trim()
        }
        if (!$cookie) { throw 'SessionCookieFile is empty.' }
        $context = Invoke-Probe '/api/v1/agent/context' $ExpectedHost @{ Cookie = $cookie }
        $contextBody = Read-JsonBody $context
        $cacheControl = [string]$context.headers['cache-control']
        $contextPassed = $context.status -eq 200 -and
            $contextBody.data.authenticated -eq $true -and
            ![string]::IsNullOrWhiteSpace([string]$contextBody.data.agent.agent_id) -and
            ![string]::IsNullOrWhiteSpace([string]$contextBody.data.main_user_id) -and
            $cacheControl -match 'no-store'
        Add-Result 'session.context' $contextPassed (
            'status={0} authenticated={1} tenant_present={2} user_present={3} no_store={4}' -f
                $context.status,
                ($contextBody.data.authenticated -eq $true),
                (![string]::IsNullOrWhiteSpace([string]$contextBody.data.agent.agent_id)),
                (![string]::IsNullOrWhiteSpace([string]$contextBody.data.main_user_id)),
                ($cacheControl -match 'no-store')
        )
    }

    if ($AgentKeyFile) {
        if (!(Test-Path -LiteralPath $AgentKeyFile -PathType Leaf)) {
            throw 'AgentKeyFile does not exist.'
        }
        $agentKey = ([IO.File]::ReadAllText([IO.Path]::GetFullPath($AgentKeyFile))).Trim()
        if (!$agentKey.StartsWith('sk-', [StringComparison]::Ordinal) -or $agentKey.StartsWith('sk-super-', [StringComparison]::Ordinal)) {
            throw 'AgentKeyFile must contain one AgentAPI key, not a SuperKey or main-site key.'
        }
        $usage = Invoke-Probe '/v1/usage' $ExpectedHost @{ Authorization = 'Bearer ' + $agentKey }
        Add-Result 'key.usage_read' ($usage.status -eq 200) ('status={0}' -f $usage.status)
    }

    $passed = @($results | Where-Object { !$_.passed }).Count -eq 0
    $summary = [pscustomobject]@{
        passed = $passed
        base_url = $baseUri.GetLeftPart([UriPartial]::Authority)
        expected_host = $ExpectedHost
        authenticated_session_checked = [bool]$SessionCookieFile
        agent_key_checked = [bool]$AgentKeyFile
        checks = $results
    }
    if ($Json) {
        $summary | ConvertTo-Json -Depth 6
    } else {
        foreach ($result in $results) {
            $prefix = if ($result.passed) { 'PASS' } else { 'FAIL' }
            Write-Output ('{0}: {1} - {2}' -f $prefix, $result.name, $result.detail)
        }
        Write-Output ('RESULT: {0}' -f $(if ($passed) { 'PASS' } else { 'FAIL' }))
    }
    if (!$passed) { exit 1 }
}
finally {
    $client.Dispose()
    $handler.Dispose()
}

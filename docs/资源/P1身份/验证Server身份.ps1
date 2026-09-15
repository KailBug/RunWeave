# 在仓库根目录运行；只创建独立测试状态并监听 loopback，不输出原始 token。
$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '../../..')).Path
$executable = Join-Path $repoRoot 'bin/runweave.exe'
if (-not (Test-Path -LiteralPath $executable)) { throw '请先构建 bin/runweave.exe' }
$runDir = Join-Path $repoRoot ('state/p1-identity-smoke-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $runDir | Out-Null
$portReservation = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
$portReservation.Start()
$port = $portReservation.LocalEndpoint.Port
$portReservation.Stop()
$serverConfig = Join-Path $runDir 'server.local.yaml'
@"
version: 1
role: server
state_dir: ./server-state
server:
  listen: 127.0.0.1:$port
"@ | Set-Content -LiteralPath $serverConfig -Encoding utf8
$stdoutPath = Join-Path $runDir 'server-stdout.txt'
$stderrPath = Join-Path $runDir 'server-stderr.txt'
$serverProcess = $null
$issuedRoles = @()
try {
    $serverProcess = Start-Process -FilePath $executable -ArgumentList @('server', 'serve', '--config', ('"' + $serverConfig + '"')) -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdoutPath -RedirectStandardError $stderrPath
    $healthy = $false
    $deadline = [DateTime]::UtcNow.AddSeconds(10)
    while ([DateTime]::UtcNow -lt $deadline) {
        $serverProcess.Refresh()
        if ($serverProcess.HasExited) { throw 'Server 在启动时退出，检查测试目录中的 stderr' }
        try {
            $health = Invoke-RestMethod -Uri "http://127.0.0.1:$port/healthz" -TimeoutSec 1
            if ($health.status -eq 'ok') { $healthy = $true; break }
        } catch { }
        Start-Sleep -Milliseconds 50
    }
    if (-not $healthy) { throw 'Server 健康检查超时' }
    & $executable server serve --config $serverConfig 2>$null
    if ($LASTEXITCODE -ne 1) { throw '第二个 daemon 未被拒绝' }
    foreach ($role in @('principal', 'node')) {
        $id = "$role-smoke"
        $tokenPath = Join-Path $runDir "$role.token"
        & $executable identity create --config $serverConfig --role $role --id $id --token-file $tokenPath
        if ($LASTEXITCODE -ne 0) { throw '身份创建失败' }
        $issuedRoles += $role
        $clientConfig = Join-Path $runDir "$role.local.yaml"
        if ($role -eq 'principal') {
            $clientBody = "version: 1`nrole: mcp`nmcp:`n  server_url: http://127.0.0.1:$port`n  token_file: ./principal.token`n"
        } else {
            $clientBody = "version: 1`nrole: node`nstate_dir: ./node-state`nnode:`n  node_id: $id`n  server_url: http://127.0.0.1:$port`n  token_file: ./node.token`n"
        }
        Set-Content -LiteralPath $clientConfig -Value $clientBody -Encoding utf8
        & $executable auth check --config $clientConfig
        if ($LASTEXITCODE -ne 0) { throw '身份接入失败' }
        & $executable identity revoke --config $serverConfig --role $role --id $id
        if ($LASTEXITCODE -ne 0) { throw '身份撤销失败' }
        & $executable auth check --config $clientConfig 2>$null
        if ($LASTEXITCODE -ne 1) { throw '已撤销身份仍可接入' }
    }
    if ((Get-Item -LiteralPath $stdoutPath).Length -ne 0) { throw 'daemon stdout 混入输出' }
    Write-Output 'PASS: 产品 Server、跨进程锁、两类身份接入、在线撤销、stdout 隔离'
} finally {
    foreach ($role in $issuedRoles) {
        & $executable identity revoke --config $serverConfig --role $role --id "$role-smoke" *> $null
    }
    if ($null -ne $serverProcess) {
        $serverProcess.Refresh()
        if (-not $serverProcess.HasExited) {
            # 只终止本脚本创建并持有的进程对象，不按名称杀进程。
            $serverProcess.Kill()
            $serverProcess.WaitForExit()
        }
        $serverProcess.Dispose()
    }
}
& $executable state init --config $serverConfig
if ($LASTEXITCODE -ne 0) { throw '产品进程退出后状态锁未释放' }

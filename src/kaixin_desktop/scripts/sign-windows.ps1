# Windows 代码签名钩子（由 tauri.conf.json 的 bundle.windows.signCommand 调用）
# 用法：powershell -NoProfile -ExecutionPolicy Bypass -File ../scripts/sign-windows.ps1 <待签名文件>
#
# 证书通过环境变量提供，证书本身绝不进仓库：
#   KAIXIN_SIGN_PFX             PFX 证书文件路径
#   KAIXIN_SIGN_PFX_PASSWORD    PFX 密码
#   KAIXIN_SIGN_THUMBPRINT      或者：证书存储中的指纹（已安装的证书）
# 未配置证书时只告警并返回 0，保证开发机构建不被卡住。

param(
    [Parameter(Mandatory = $true, Position = 0)]
    [string]$FilePath
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path -LiteralPath $FilePath)) {
    Write-Error "待签名文件不存在：$FilePath"
    exit 1
}

$signtool = $null
$roots = @(
    "${env:ProgramFiles(x86)}\Windows Kits\10\bin",
    "$env:ProgramFiles\Windows Kits\10\bin"
) | Where-Object { $_ -and (Test-Path $_) }

foreach ($root in $roots) {
    $signtool = Get-ChildItem -Path $root -Recurse -Filter signtool.exe -ErrorAction SilentlyContinue |
        Where-Object { $_.FullName -match '\\x64\\' } |
        Sort-Object FullName -Descending |
        Select-Object -First 1 -ExpandProperty FullName
    if ($signtool) { break }
}

if (-not $signtool) {
    Write-Warning "未找到 signtool.exe（需安装 Windows SDK），跳过代码签名：$FilePath"
    exit 0
}

$common = @("sign", "/fd", "SHA256", "/tr", "http://timestamp.digicert.com", "/td", "SHA256")

if ($env:KAIXIN_SIGN_PFX) {
    $signArgs = $common + @("/f", $env:KAIXIN_SIGN_PFX)
    if ($env:KAIXIN_SIGN_PFX_PASSWORD) {
        $signArgs += @("/p", $env:KAIXIN_SIGN_PFX_PASSWORD)
    }
    $signArgs += $FilePath
    & $signtool @signArgs
} elseif ($env:KAIXIN_SIGN_THUMBPRINT) {
    $signArgs = $common + @("/sha1", $env:KAIXIN_SIGN_THUMBPRINT) + $FilePath
    & $signtool @signArgs
} else {
    Write-Warning "未配置代码签名证书（KAIXIN_SIGN_PFX / KAIXIN_SIGN_THUMBPRINT），跳过签名：$FilePath"
    exit 0
}

if ($LASTEXITCODE -ne 0) {
    Write-Error "signtool 签名失败（exit $LASTEXITCODE）：$FilePath"
    exit $LASTEXITCODE
}

Write-Host "已签名：$FilePath"
exit 0

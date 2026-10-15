[CmdletBinding()]
param(
    [string]$Publisher
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

# Resolved in the body, as the publisher does: $PSScriptRoot can be empty in a
# parameter default under powershell -File from another working directory.
if ([string]::IsNullOrEmpty($Publisher)) {
    $Here = if (-not [string]::IsNullOrEmpty($PSScriptRoot)) { $PSScriptRoot } else { Split-Path -Parent $PSCommandPath }
    $Publisher = Join-Path $Here 'publish-download-portal.ps1'
}

$Tokens = $null
$ParseErrors = $null
$Ast = [Management.Automation.Language.Parser]::ParseFile(
    $Publisher,
    [ref]$Tokens,
    [ref]$ParseErrors
)
if ($ParseErrors.Count -ne 0) {
    throw "publisher has PowerShell parse errors: $($ParseErrors[0].Message)"
}
$PublisherSource = [IO.File]::ReadAllText($Publisher)
if ($PublisherSource -match '(?m)^\s*Write-Host \$SuccessMarker\s*$') {
    throw 'Publisher must not echo the promoter success marker a second time.'
}

foreach ($Name in @(
    'ConvertTo-NativeArgument',
    'ConvertTo-LfUtf8Bytes',
    'Invoke-BoundedChild',
    'Resolve-PublisherScriptDirectory'
)) {
    $Definition = @($Ast.FindAll({
        param($Node)
        $Node -is [Management.Automation.Language.FunctionDefinitionAst] -and
            $Node.Name -eq $Name
    }, $true))
    if ($Definition.Count -ne 1) {
        throw "expected exactly one production function named $Name"
    }
    Invoke-Expression $Definition[0].Extent.Text
}

$Probe = 'CELIKPANEL_PUBLISHER_STDIN_PROBE'
$Utf8 = [Text.UTF8Encoding]::new($false)
[byte[]]$WireBytes = ConvertTo-LfUtf8Bytes (
    $Utf8.GetBytes("$Probe`r`n")
)
$ChildPowerShell = Join-Path $env:WINDIR 'System32\WindowsPowerShell\v1.0\powershell.exe'
$ByteProbeCommand = '$s=[Console]::OpenStandardInput();$b=New-Object byte[] 128;$n=$s.Read($b,0,$b.Length);[BitConverter]::ToString($b,0,$n)'
$Result = Invoke-BoundedChild `
    -FilePath $ChildPowerShell `
    -Arguments @('-NoProfile', '-NonInteractive', '-Command', $ByteProbeCommand) `
    -TimeoutSeconds 10 `
    -InputBytes $WireBytes

if (@($Result).Count -ne 1) {
    throw 'Invoke-BoundedChild leaked an extra pipeline object.'
}
foreach ($Property in @('ExitCode', 'TimedOut', 'Stdout', 'Stderr')) {
    if ($Result.PSObject.Properties.Name -notcontains $Property) {
        throw "Invoke-BoundedChild result is missing $Property."
    }
}
if ($Result.TimedOut -or $Result.ExitCode -ne 0 -or
    $Result.Stdout.Trim() -ne [BitConverter]::ToString($WireBytes)) {
    throw 'Invoke-BoundedChild did not round-trip non-empty stdin.'
}

if ($Utf8.GetString($WireBytes) -ne "$Probe`n" -or $WireBytes -contains 13) {
    throw 'Publisher wire input was not normalized to LF-only UTF-8.'
}

# The helper-script defaults (2026-10-15): no parameter default may depend on
# the script location, and the body resolves the script's directory from the
# first automatic value that is set.
$ParamBlock = $Ast.ParamBlock.Extent.Text
if ($ParamBlock -match '=\s*\(?\s*Join-Path\s+\$PSScriptRoot|=\s*\$PSScriptRoot|=\s*\$PSCommandPath|=\s*\$MyInvocation') {
    throw 'A publisher parameter default depends on the script location; resolve it in the body.'
}
if ((Resolve-PublisherScriptDirectory 'C:\a\deploy' 'C:\b\x.ps1' 'C:\c\x.ps1') -ne 'C:\a\deploy') {
    throw 'Resolve-PublisherScriptDirectory must prefer PSScriptRoot.'
}
if ((Resolve-PublisherScriptDirectory '' 'C:\b\deploy\publish-download-portal.ps1' '') -ne 'C:\b\deploy') {
    throw 'Resolve-PublisherScriptDirectory must fall back to PSCommandPath.'
}
if ((Resolve-PublisherScriptDirectory '' '' 'C:\c\deploy\publish-download-portal.ps1') -ne 'C:\c\deploy') {
    throw 'Resolve-PublisherScriptDirectory must fall back to the invocation path.'
}
$Unresolved = $false
try { [void](Resolve-PublisherScriptDirectory '' '' '') } catch { $Unresolved = $true }
if (-not $Unresolved) { throw 'Resolve-PublisherScriptDirectory must refuse when no location is known.' }

# The way it failed on 2026-10-15: powershell -File from another working
# directory. A deliberately invalid version stops the publisher at its first
# validation, before any file, network or remote is touched; reaching that
# message proves that parameter binding and the default resolution passed.
$PublisherPath = (Resolve-Path -LiteralPath $Publisher).Path
$Elsewhere = [IO.Path]::GetTempPath()
$ProbeOut = Join-Path $Elsewhere 'celikpanel-publisher-binding-probe.out'
$ProbeErr = Join-Path $Elsewhere 'celikpanel-publisher-binding-probe.err'
$BindingProbe = Start-Process -FilePath $ChildPowerShell -WorkingDirectory $Elsewhere -NoNewWindow -Wait -PassThru `
    -RedirectStandardOutput $ProbeOut -RedirectStandardError $ProbeErr `
    -ArgumentList @('-NoProfile', '-NonInteractive', '-File', ('"' + $PublisherPath + '"'),
        '-IdentityFile', 'unused', '-Target', 'probe@example.invalid', '-Package', 'unused',
        '-PreviousVersion', 'not-a-version', '-TargetVersion', 'v0.0.1', '-PackageSize', '1',
        '-PackageSha256', ('0' * 64), '-PublicBaseUrl', 'https://example.invalid/', '-RemoteRoot', '/unused')
$ProbeText = ''
foreach ($File in @($ProbeOut, $ProbeErr)) {
    if (Test-Path -LiteralPath $File) {
        $ProbeText += [IO.File]::ReadAllText($File)
        Remove-Item -LiteralPath $File -Force
    }
}
if ($BindingProbe.ExitCode -eq 0 -or $ProbeText -notmatch 'canonical v-prefixed semantic versions' -or
    $ProbeText -match 'Cannot bind argument') {
    throw "publisher did not reach its first validation from another working directory: $ProbeText"
}

Write-Host 'download portal publisher PowerShell 5.1 runtime contract passed'

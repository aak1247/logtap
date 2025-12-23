param(
  [int]$Bytes = 32
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if ($Bytes -lt 32) { $Bytes = 32 }
if ($Bytes -gt 128) { $Bytes = 128 }

$b = New-Object byte[] $Bytes
[System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b)
$s = [Convert]::ToBase64String($b)

Write-Host $s
Write-Host ""
Write-Host "ENV:"
Write-Host "  AUTH_SECRET=$s"


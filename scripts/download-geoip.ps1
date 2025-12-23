param(
  [string]$LicenseKey = $env:MAXMIND_LICENSE_KEY,
  [string]$OutDir = "",
  [switch]$City,
  [switch]$ASN
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
try {
  [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
} catch {
}

function Get-RepoRoot {
  return Resolve-Path (Join-Path $PSScriptRoot "..")
}

function Ensure-Dir([string]$p) {
  New-Item -ItemType Directory -Force -Path $p | Out-Null
}

function Download-MaxMindEdition([string]$editionId, [string]$licenseKey, [string]$outDir, [string]$destName) {
  if ([string]::IsNullOrWhiteSpace($licenseKey)) {
    throw "MAXMIND_LICENSE_KEY not set (or pass -LicenseKey); required to download $editionId."
  }

  Ensure-Dir $outDir

  $url = "https://download.maxmind.com/app/geoip_download?edition_id=$editionId&license_key=$licenseKey&suffix=tar.gz"
  $tmpRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("logtap-geoip-" + [System.Guid]::NewGuid().ToString("N"))
  $archive = Join-Path $tmpRoot "$editionId.tar.gz"
  $extractDir = Join-Path $tmpRoot "extract"

  Ensure-Dir $tmpRoot
  Ensure-Dir $extractDir

  try {
    Write-Host "Downloading $editionId..."
    $iwr = Get-Command Invoke-WebRequest -ErrorAction Stop
    if ($iwr.Parameters.ContainsKey("UseBasicParsing")) {
      Invoke-WebRequest -Uri $url -OutFile $archive -UseBasicParsing | Out-Null
    } else {
      Invoke-WebRequest -Uri $url -OutFile $archive | Out-Null
    }

    if (-not (Test-Path $archive)) {
      throw "Download failed: $archive not found."
    }
    if (-not (Get-Command tar -ErrorAction SilentlyContinue)) {
      throw "'tar' not found in PATH; required to extract $archive (Windows 10/11 includes tar)."
    }

    & tar -xzf $archive -C $extractDir

    $mmdb = Get-ChildItem -Path $extractDir -Recurse -Filter "*.mmdb" | Select-Object -First 1
    if ($null -eq $mmdb) {
      throw "No .mmdb found after extracting $editionId."
    }

    $destPath = Join-Path $outDir $destName
    Copy-Item -Force -Path $mmdb.FullName -Destination $destPath
    Write-Host "Saved: $destPath"
    return $destPath
  } finally {
    if (Test-Path $tmpRoot) {
      Remove-Item -Recurse -Force $tmpRoot
    }
  }
}

$repoRoot = Get-RepoRoot
if ([string]::IsNullOrWhiteSpace($OutDir)) {
  $OutDir = Join-Path $repoRoot "data\\geoip"
}

if (-not $City -and -not $ASN) {
  $City = $true
  $ASN = $true
}

$cityPath = $null
$asnPath = $null
if ($City) {
  $cityPath = Download-MaxMindEdition "GeoLite2-City" $LicenseKey $OutDir "GeoLite2-City.mmdb"
}
if ($ASN) {
  $asnPath = Download-MaxMindEdition "GeoLite2-ASN" $LicenseKey $OutDir "GeoLite2-ASN.mmdb"
}

Write-Host ""
Write-Host "Gateway env vars:"
if ($cityPath) { Write-Host "  GEOIP_CITY_MMDB=$cityPath" }
if ($asnPath) { Write-Host "  GEOIP_ASN_MMDB=$asnPath" }

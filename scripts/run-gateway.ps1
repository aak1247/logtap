param(
  [string]$HTTPAddr = ":8080",
  [string]$NSQDAddress = "172.168.1.226:4150",
  [string]$PostgresURL = "postgres://logtap:password@127.0.0.1:5432/logtap?sslmode=disable",
  [string]$RedisAddr = "127.0.0.1:6379",
  [string]$RedisPassword = "",
  [string]$GeoIPCityMMDB = "",
  [string]$GeoIPASNMMDB = "",
  [string]$AuthSecret = "",
  [string]$AuthTokenTTL = "",
  [switch]$RunConsumers = $true,
  [string]$NSQEventChannel = "",
  [string]$NSQLogChannel = "",
  [switch]$Build
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Sanitize-NsqChannel([string]$s) {
  if ([string]::IsNullOrWhiteSpace($s)) { return "" }
  return ($s -replace "[^a-zA-Z0-9_\-\.]", "-")
}

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot

if ([string]::IsNullOrWhiteSpace($GeoIPCityMMDB)) {
  $candidate = Join-Path $repoRoot "data\\geoip\\GeoLite2-City.mmdb"
  if (Test-Path $candidate) {
    $GeoIPCityMMDB = $candidate
  }
}
if ([string]::IsNullOrWhiteSpace($GeoIPASNMMDB)) {
  $candidate = Join-Path $repoRoot "data\\geoip\\GeoLite2-ASN.mmdb"
  if (Test-Path $candidate) {
    $GeoIPASNMMDB = $candidate
  }
}

if ([string]::IsNullOrWhiteSpace($AuthSecret)) {
  if (-not [string]::IsNullOrWhiteSpace($env:AUTH_SECRET)) {
    $AuthSecret = $env:AUTH_SECRET
  }
}
if ([string]::IsNullOrWhiteSpace($AuthTokenTTL)) {
  if (-not [string]::IsNullOrWhiteSpace($env:AUTH_TOKEN_TTL)) {
    $AuthTokenTTL = $env:AUTH_TOKEN_TTL
  }
}

if ([string]::IsNullOrWhiteSpace($NSQEventChannel)) {
  $NSQEventChannel = "logtap-$($env:COMPUTERNAME)-events"
}
if ([string]::IsNullOrWhiteSpace($NSQLogChannel)) {
  $NSQLogChannel = "logtap-$($env:COMPUTERNAME)-logs"
}
$NSQEventChannel = Sanitize-NsqChannel $NSQEventChannel
$NSQLogChannel = Sanitize-NsqChannel $NSQLogChannel

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
  throw "go not found in PATH; please install Go 1.22+"
}

$env:HTTP_ADDR = $HTTPAddr
$env:NSQD_ADDRESS = $NSQDAddress
$env:POSTGRES_URL = $PostgresURL
$env:REDIS_ADDR = $RedisAddr
$env:REDIS_PASSWORD = $RedisPassword
$env:ENABLE_METRICS = $(if ([string]::IsNullOrWhiteSpace($RedisAddr)) { "false" } else { "true" })
$env:GEOIP_CITY_MMDB = $GeoIPCityMMDB
$env:GEOIP_ASN_MMDB = $GeoIPASNMMDB
$env:AUTH_SECRET = $AuthSecret
$env:AUTH_TOKEN_TTL = $AuthTokenTTL
$env:RUN_CONSUMERS = ($(if ($RunConsumers) { "true" } else { "false" }))
$env:NSQ_EVENT_CHANNEL = $NSQEventChannel
$env:NSQ_LOG_CHANNEL = $NSQLogChannel

Write-Host "HTTP_ADDR=$env:HTTP_ADDR"
Write-Host "NSQD_ADDRESS=$env:NSQD_ADDRESS"
Write-Host "REDIS_ADDR=$env:REDIS_ADDR"
Write-Host "ENABLE_METRICS=$env:ENABLE_METRICS"
Write-Host "GEOIP_CITY_MMDB=$env:GEOIP_CITY_MMDB"
Write-Host "GEOIP_ASN_MMDB=$env:GEOIP_ASN_MMDB"
Write-Host "AUTH_SECRET=$env:AUTH_SECRET"
Write-Host "AUTH_TOKEN_TTL=$env:AUTH_TOKEN_TTL"
Write-Host "RUN_CONSUMERS=$env:RUN_CONSUMERS"
Write-Host "NSQ_EVENT_CHANNEL=$env:NSQ_EVENT_CHANNEL"
Write-Host "NSQ_LOG_CHANNEL=$env:NSQ_LOG_CHANNEL"

if ($Build) {
  New-Item -ItemType Directory -Force -Path "bin" | Out-Null
  & go build -o "bin/gateway.exe" ./cmd/gateway
  & "bin/gateway.exe"
} else {
  & go run ./cmd/gateway
}

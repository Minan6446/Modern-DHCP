param(
  [int]$MinCoverage = 80
)

$ErrorActionPreference = 'Stop'

Write-Host '[1/3] Running DHCPv4 targeted tests...'
go test ./internal/dhcpv4 -run 'TestDHCPv4|TestMessage|TestLeaseFSM|TestACD' -coverprofile=coverage_dhcpv4.out -covermode=count

Write-Host '[2/3] Reading coverage...'
$coverOutput = & go tool cover "-func=coverage_dhcpv4.out"
$coverLine = $coverOutput | Select-String 'total:' | Select-Object -First 1 | ForEach-Object { $_.ToString() }
if (-not $coverLine) {
  throw 'Unable to read total coverage.'
}
$percentText = ($coverLine -split '\s+')[-1].TrimEnd('%')
$percent = [double]$percentText
Write-Host ("Total coverage: {0}%" -f $percent)

Write-Host '[3/3] Enforcing threshold...'
if ($percent -lt $MinCoverage) {
  throw ("Coverage {0}% is below threshold {1}%" -f $percent, $MinCoverage)
}
Write-Host 'DHCPv4 tests passed with required coverage.'

# set14b: process-level "system required" request for the duration of the run only (no setting changed).
# Ends when the stop file exists or after 8 hours, whichever comes first; the request ends with the process.
$stop = Join-Path $PSScriptRoot "keepawake.stop"
$log = Join-Path $PSScriptRoot "keepawake.log"
Add-Type -Namespace Set14b -Name Power -MemberDefinition '[DllImport("kernel32.dll")] public static extern uint SetThreadExecutionState(uint esFlags);'
$prev = [Set14b.Power]::SetThreadExecutionState([uint32]2147483649)
[IO.File]::AppendAllText($log, ("{0} started previous_state={1}`n" -f (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ"), $prev))
$deadline = (Get-Date).AddHours(8)
while (-not (Test-Path $stop) -and (Get-Date) -lt $deadline) { Start-Sleep -Seconds 30 }
[void][Set14b.Power]::SetThreadExecutionState([uint32]2147483648)
[IO.File]::AppendAllText($log, ("{0} ended (stop file present: {1})`n" -f (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ"), (Test-Path $stop)))

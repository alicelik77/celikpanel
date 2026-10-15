# set16: Get-PSDrive C every 60 s into c-drive-watch.txt (UTC stamp, free bytes, free GiB); ends when cwatch.stop exists
# or after 9 hours. Writes c-drive.below40 when a reading is under 40 GiB. Reads only.
$dir = $PSScriptRoot
$out = Join-Path $dir "c-drive-watch.txt"
$stop = Join-Path $dir "cwatch.stop"
$deadline = (Get-Date).AddHours(9)
while (-not (Test-Path $stop) -and (Get-Date) -lt $deadline) {
  $free = (Get-PSDrive C).Free
  $gib = [math]::Round($free / 1GB, 2)
  [IO.File]::AppendAllText($out, ("{0} free_bytes={1} free_GiB={2}`n" -f (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ"), $free, $gib))
  if ($free -lt 40GB) { [IO.File]::WriteAllText((Join-Path $dir "c-drive.below40"), "below 40 GiB`n") }
  Start-Sleep -Seconds 60
}

# v0.1.0-alpha.115

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.115.tr.md)

A small fix release after v0.1.0-alpha.114, for what the owner hit while
reinstalling the first server. **Only the owner updates installed panels, from
the panel's own update screen.**

## Fixes

- **Server setup: changing the main domain.** After reaching the
  existing-records step with one domain and going back to type another, the
  setup draft kept the first domain's imported records; the server refused
  it and the wizard said "check the connection". Changing the main domain now
  drops the old domain's record answers (the records step reads the new
  domain's own) and the mail2 name offered for it.
- **Server setup: a refused answer says why.** When the server does not
  accept a draft, the wizard shows the server's reason instead of the
  connection text.
- **Panel tab icon.** The browser tab showed Vite's starter icon; it now
  shows the CelikPanel mark.
- A test that could hit a database lock beside a background DKIM pass no
  longer starts that pass.

## Not covered

Only the automatic checks ran for this release (web, Go, release and
distribution checks); no new real-server trial was run, by the owner's choice.
The items listed as not tried in the alpha.114 notes stay not tried.

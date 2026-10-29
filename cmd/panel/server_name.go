package main

import hostnames "github.com/alicelik/celikpanel/internal/hostname"

// localServerName is this server's full name (boston.example.com), found the
// way `hostname -f` finds it, so a cloud image's short kernel name ("boston")
// does not reach the display, the license account or derived DNS defaults.
// localServerName sunucunun tam adıdır; bulut kalıbının kısa çekirdek adı
// gösterime, lisans hesabına ve türetilen DNS varsayılanlarına ulaşmaz.
var localServerName = hostnames.LocalFQDN

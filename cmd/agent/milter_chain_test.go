package main

import "testing"

// The milter chain decides whether outgoing mail is signed and incoming mail is
// filtered. Two live failures are pinned here so neither can come back:
//
//  1. DKIM used to write `smtpd_milters = <opendkim>` outright. A spam filter
//     wired afterwards replaced it, or was replaced by it — last writer wins,
//     no error, mail quietly unsigned or unfiltered.
//  2. Rspamd was installed and "Running" on Boston while `smtpd_milters` was
//     EMPTY (24 Jul): the daemon was up and not one message went through it.
//
// Milter zinciri, çıkan postanın imzalanıp gelen postanın süzülüp
// süzülmeyeceğine karar verir. İkisi de geri dönmesin diye iki canlı arıza
// buraya sabitlenmiştir:
//
//  1. DKIM doğrudan `smtpd_milters = <opendkim>` yazıyordu. Sonradan bağlanan
//     bir spam filtresi onu siliyor ya da onun tarafından siliniyordu — son
//     yazan kazanır, hata yok, posta sessizce imzasız ya da süzgeçsiz.
//  2. Boston'da Rspamd kurulu ve "Çalışıyor"ken `smtpd_milters` BOŞtu
//     (24 Tem): daemon ayaktaydı ve içinden tek bir ileti geçmiyordu.
func TestComposeMilterChain(t *testing.T) {
	cases := []struct {
		name         string
		dkim         bool
		spam         string
		wantIncoming string
		wantOutgoing string
	}{
		{
			// The regression: both present must COMPOSE, DKIM first.
			// Asıl gerileme: ikisi de varsa BİRLEŞMELİ, önce DKIM.
			name:         "dkim ve spam birlikte",
			dkim:         true,
			spam:         rspamdMilter,
			wantIncoming: opendkimMilter + ", " + rspamdMilter,
			wantOutgoing: opendkimMilter + ", " + rspamdMilter,
		},
		{
			// A spam filter alone must still reach incoming mail — installed
			// and unwired was the exact bug.
			// Tek başına spam filtresi de gelen postaya ulaşmalı — kurulu ama
			// bağlanmamış olması tam da o hataydı.
			name:         "yalniz spam filtresi",
			spam:         rspamdMilter,
			wantIncoming: rspamdMilter,
			wantOutgoing: rspamdMilter,
		},
		{
			// spamass-milter judges arriving mail only.
			// spamass-milter yalnız gelen postayı yargılar.
			name:         "dkim ve spamass",
			dkim:         true,
			spam:         "unix:/spamass/spamass.sock",
			wantIncoming: opendkimMilter + ", unix:/spamass/spamass.sock",
			wantOutgoing: opendkimMilter,
		},
		{
			name:         "yalniz dkim",
			dkim:         true,
			wantIncoming: opendkimMilter,
			wantOutgoing: opendkimMilter,
		},
		{
			// Nothing installed: both lists empty, never a stale endpoint.
			// Hiçbiri kurulu değil: iki liste de boş, asla bayat bir uç değil.
			name: "hicbiri yok",
		},
		{
			// spamassMilterEndpoint returns "" when it cannot determine the
			// socket. A missing line is honest; a guessed one is a mail server
			// talking to nothing.
			// Soketi saptayamayınca spamassMilterEndpoint "" döner. Eksik satır
			// dürüsttür; tahmin edilmişi, hiçliğe konuşan bir posta sunucusudur.
			name:         "spam ucu bilinmiyor",
			dkim:         true,
			spam:         "",
			wantIncoming: opendkimMilter,
			wantOutgoing: opendkimMilter,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in, out := composeMilterChain(c.dkim, c.spam)
			if in != c.wantIncoming {
				t.Errorf("smtpd_milters = %q, want %q", in, c.wantIncoming)
			}
			if out != c.wantOutgoing {
				t.Errorf("non_smtpd_milters = %q, want %q", out, c.wantOutgoing)
			}
		})
	}
}

// Mail made on this server without SMTP (a site's PHP mail(), cron) is
// scanned by Rspamd like mail sent through 587/465, after DKIM signing (final
// alpha.113 acceptance run 3: PHP mail() passed OpenDKIM only). Bounces do
// not pass non_smtpd_milters (Postfix's internal_mail_filter_classes, which
// CelikPanel never sets), and milter_default_action=accept keeps a dead
// Rspamd from holding mail.
// Sunucuda SMTP'siz üretilen posta (PHP mail(), cron), 587/465 postası gibi
// DKIM imzasından sonra Rspamd'den geçer.
func TestLocallyMadeMailIsSignedThenScannedByRspamd(t *testing.T) {
	_, outgoing := composeMilterChain(true, rspamdMilter)
	if outgoing != opendkimMilter+", "+rspamdMilter {
		t.Errorf("non_smtpd_milters = %q, want DKIM then Rspamd", outgoing)
	}
	// The list stays CelikPanel's own, so the Panel-start repair keeps
	// composing it (and an owner's list is still kept and named).
	if !milterListPanelComposed(outgoing, rspamdMilter) {
		t.Errorf("the new non_smtpd_milters list is not recognised as CelikPanel's")
	}
	if milterListPanelComposed(outgoing+", inet:127.0.0.1:9999", rspamdMilter) {
		t.Errorf("an owner-extended list was taken as CelikPanel's")
	}
}

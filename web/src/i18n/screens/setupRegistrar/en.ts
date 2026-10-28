// SCREEN catalogue part "setupRegistrar": the setup's live registrar card
// (v0.1.0-alpha.113, prefix setup.v4.).
// "setupRegistrar" ekran parcasi: kurulumun canli alan adi firmasi karti.

export const enSetupRegistrarScreens = {
    "setup.v4.state.ok": "correct",
    "setup.v4.state.notYet": "not yet",
    "setup.v4.state.waiting": "waiting",
    "setup.v4.state.unknown": "could not be checked",
    "setup.v4.item.glue4": "{name} → {ip} (IP record, IPv4)",
    "setup.v4.item.glue6": "{name} → {ip} (IP record, IPv6, optional)",
    "setup.v4.item.nameservers": "{domain} nameservers: {names}",
    "setup.v4.item.answersMain": "The main DNS server ({ip}) answers for the domain",
    "setup.v4.item.answersBackup": "The backup DNS server ({ip}) answers for the domain",
    "setup.v4.item.imported": "Your servers answer the imported and mail records as the zone publishes them ({matched}/{total})",
    "setup.v4.why.notRegistered": "The registry does not know {domain} yet: check the domain is registered and active.",
    "setup.v4.why.nameservers": "The registry still gives: {seen}. Enter the two names above at your registrar.",
    "setup.v4.why.glueMissing": "The registry has no IP record for this name yet; enter it in step 1.",
    "setup.v4.why.glueWrong": "The registry has {seen} for this name; correct it in step 1.",
    "setup.v4.why.glue6": "If your registrar has no IPv6 field, skip it; IPv4 is enough.",
    "setup.v4.why.backupWaiting": "Answers once the backup DNS server is set up and has received the list.",
    "setup.v4.why.imported": "Some imported records answer differently; check them on the Domains page before the change spreads.",
    "setup.v4.why.importedUnknown": "Some records could not be read from your servers just now; nothing to do, they are asked again in a minute.",
    "setup.v4.doneTitle": "Everything is done at your registrar",
    "setup.v4.doneWhy": "The registry hands out your nameservers and both your servers answer from the internet.",
    "setup.v4.doneNext": "Nothing; the change may still take a while to reach every cache.",
    "setup.v4.early": "Do it now anyway",
    "setup.v4.earlyRefused": "Not yet: the main DNS server does not answer for the domain from the internet, so a registrar change now would leave the domain without an answer. This page checks again by itself.",
    "setup.v4.earlyWarn": "If the registrar accepts, {domain} answers only from the main DNS server until the backup is set up; there is no backup meanwhile.",
    "setup.v4.earlyWarnTR": ".tr registries accept the change only when both nameservers answer, so {domain} will most likely be refused now: set up the backup DNS server first.",
    "setup.v4.reading": "Checking from the internet…",
    "setup.v4.readFailed": "The check could not be read just now; it is tried again shortly. The values above stay correct.",
    "setup.v4.glueHelp": "Your nameservers are inside {domain}, so their IP records come first; many registrars refuse step 2 without them. The setting is called “Child nameservers”, “Host records”, “Register nameservers” or “Glue” (in Turkish panels “Özel ad sunucusu” or “Host kaydı”). If there is no IPv6 field, IPv4 is enough.",
    "setup.v4.ipv6Optional": "IPv6 (optional)",
    "setup.v4.trNote": ".tr and .com.tr registries accept a nameserver change only when both nameservers already answer for {domain}.",
    "setup.v4.seenTitle": "Seen from the internet",
    "setup.v4.checkedAt": "Last checked at {time}; checked again about every minute.",
} as const;

export type SetupRegistrarScreenKey = keyof typeof enSetupRegistrarScreens;

import type { SetupDNSKey } from './setupDNS/en';
// English catalog. This file is the source of truth for translation keys:
// its shape defines the TranslationKey type, and every other locale must
// provide the same keys.
//
// İngilizce katalog. Bu dosya çeviri anahtarlarının doğruluk kaynağıdır:
// şekli TranslationKey tipini tanımlar ve diğer her yerel ayar aynı
// anahtarları sağlamalıdır.
// The SHELL catalogue: the copy the application shell needs before it can
// show anything - the login screen, the navigation rail, the profile menu, an
// in-flight operation, and the refusal vocabulary every one of those can hit.
//
// Everything else lives in ./screens, which is fetched alongside the screen
// that needs it. That split is the whole reason the critical-boot budget has
// room again (register R-060): this file used to carry all 2092 keys of the
// product's copy, and only 74 of them could be reached before a route loaded.
// A key belongs here when an eagerly loaded module can reach it - including by
// building the key at runtime, which is why the whole of err.* stays.
//
// KABUK katalogu: uygulama kabugunun bir sey gosterebilmek icin ihtiyac
// duydugu metin - giris ekrani, gezinme rayi, profil menusu, suren bir islem ve
// bunlarin hepsinin dusebilecegi ret sozlugu. Gerisi ./screens icindedir ve
// ihtiyaci olan ekranla birlikte getirilir (defter R-060).
import type { SetupScreenKey } from './screens/setup/en';
import type { SitesScreenKey } from './screens/sites/en';
import type { SiteConfigScreenKey } from './screens/siteConfig/en';
import type { DnsCertificatesScreenKey } from './screens/dnsCertificates/en';
import type { ComponentsScreenKey } from './screens/components/en';
import type { ComponentPagesScreenKey } from './screens/componentPages/en';
import type { MailDatabasesScreenKey } from './screens/mailDatabases/en';
import type { SecurityScreenKey } from './screens/security/en';
import type { DashboardScreenKey } from './screens/dashboard/en';
import type { SettingsScreenKey } from './screens/settings/en';
import type { ErrorsScreenKey } from './screens/errors/en';
import type { LifecycleScreenKey } from './screens/lifecycle/en';
import type { DatabaseClustersScreenKey } from './screens/databaseClusters/en';
import type { VendorLanesScreenKey } from './screens/vendorLanes/en';
import type { WebmailAddressScreenKey } from './screens/webmailAddress/en';
import type { SetupWizardScreenKey } from './screens/setupWizard/en';
import type { SetupRegistrarScreenKey } from './screens/setupRegistrar/en';
import type { SetupJoinScreenKey } from './screens/setupJoin/en';
import type { SetupLinkScreenKey } from './screens/setupLink/en';
import type { SetupInspectionScreenKey } from './screens/setupInspection/en';
import type { DnsOverviewScreenKey } from './screens/dnsOverview/en';
import type { NginxDefaultHttpsScreenKey } from './screens/nginxDefaultHttps/en';
import type { DnsPublicationScreenKey } from './screens/dnsPublication/en';
import type { ProblemsScreenKey } from './screens/problems/en';

export const en = {
    // ---- alpha.108 tools ----
    'services.phpToolProof.whyCheck': 'PHP-FPM {version} could not open CelikPanel’s check file (HTTP {status}); Check again in its Manage window repairs it',
    'services.phpToolProof.whySaid': 'it did not answer as itself; the server said: {detail}',
    'app.name': 'CelikPanel',
    'recovery.checkingTitle': 'Checking panel access',
    'recovery.checkingHelp': 'Confirming your session and panel readiness. This check does not start a server operation.',
    'recovery.authTitle': "Your session could not be checked",
    'recovery.authHelp': "CelikPanel cannot confirm your session right now. This page checks again by itself; you can also check now. Management stays closed until your session and panel access are verified.",
    'recovery.startingTitle': "The panel is starting",
    'recovery.startingHelp': "The server is preparing panel access. This page checks readiness automatically.",
    'recovery.availabilityTitle': "Panel readiness could not be checked",
    'recovery.availabilityHelp': "Your session was verified, but panel readiness is unknown. This page checks again by itself and opens once the server confirms readiness and access. You can also check now or reload the page.",
    'recovery.handoverTitle': "The Panel restarts once during setup",
    'recovery.handoverHelp': "Setup secured panel access for {host}, and the Panel restarts once to start using its new certificate. Setup continues on the server. This page checks by itself and opens when the Panel is ready; you do not need to do anything.",
    'recovery.handoverAddress': "You can also continue at the Panel’s secure address:",
    'recovery.licenseTitle': "License status could not be checked",
    'recovery.licenseHelp': "The license result is unavailable. This does not establish that your license is missing or expired. This page checks again by itself, and management opens once the server confirms access. You can also check now.",
    // D-039: a lapse whose cause the server named.
    'recovery.licenseOfflineEndedTitle': "License check overdue: management paused",
    'recovery.licenseOfflineEndedHelp': "This server has not reached the license service (celikpanel.net) for more than 7 days. This does not mean your license was removed or expired; sites, mail, DNS and scheduled tasks keep running. The server administrator should make sure this server can open https://celikpanel.net (outbound HTTPS on port 443, working DNS), then check again. Management opens as soon as a check succeeds; this page also checks again by itself.",
    'recovery.licenseClockBehindTitle': "This server's clock is wrong: management paused",
    'recovery.licenseClockBehindHelp': "This server's clock is earlier than its last license check, or was set back. Sites, mail, DNS and scheduled tasks keep running. The server administrator should turn on time synchronisation (for example: timedatectl set-ntp true) or correct the clock, then check again.",
    'recovery.bundleTitle': "The panel interface could not be loaded",
    'recovery.bundleHelp': "Reload CelikPanel to load its current interface. Your operation ID is kept. This recovery view can read the same operation without starting it again.",
    'recovery.retry': "Check panel access",
    'recovery.checking': "Checking…",
    'recovery.operationTitle': "Update and recovery status",
    'recovery.noOperation': "No update operation ID is saved in this browser. This page cannot associate an update result without its exact ID.",
    'recovery.operationId': "Operation ID",
    'recovery.observationUnavailable': "The latest result could not be read. Any result below is the last verified observation, not confirmation that work is still running. Check again; do not start a second update.",
    'recovery.observedAt': "Last verified observation",
    'recovery.previousFailure': "Previously recorded failure",
    'recovery.checkStatus': "Check this operation",
    'recovery.phase.accepted': "Update request recorded",
    'recovery.phase.running': "Last recorded step: update in progress",
    'recovery.phase.recovering': "Last recorded step: recovery in progress",
    'recovery.phase.recovered': "Rollback verified",
    'recovery.phase.succeeded': "Update verified",
    'recovery.phase.failed': "The update failed",
    'recovery.phase.recovery_required': "Recovery needs attention",
    'recovery.next.accepted': "The server recorded this request. This page checks the same operation automatically; it does not submit it again.",
    'recovery.next.running': "The server last reported work on this update. Wait for the next recorded result; checking this page does not restart it.",
    'recovery.wait.initializing': "Last recorded step: waiting for the operating system to initialize",
    'recovery.wait.starting': "Last recorded step: waiting for the server to finish starting",
    'recovery.wait.stopping': "Last recorded step: waiting for the operating system to finish stopping",
    'recovery.automatic.pausedTitle': "Last recorded state: automatic recovery paused",
    'recovery.automatic.pausedHelp': "All three automatic recovery attempts were used without a verified result. Automatic recovery will not start another attempt. The server owner must act.",
    'recovery.automatic.cause.panel_start_unverified': "The update was applied, but the new version's panel did not come up, and completing the update was retried to its limit. The server owner should read the panel log on the server with sudo journalctl -u celikpanel-panel -n 50. Retrying repeats the same start until that cause is fixed. There is no supported return to the previous version from this point.",
    'recovery.automatic.cause.update_finishing_scheduler': "The new version is installed and running, but turning automatic certificate renewal (Certbot) back on did not confirm after the automatic attempts, so renewal may be off until this finishes. The recovery log names what stopped it, for example a change made to certbot.timer while the update ran.",
    'recovery.automatic.cause.candidate_panel_startup_check_failed': "The new version's panel failed its start check before anything was switched on, and the automatic return to the previous version did not finish. The recovery log shows what stopped it; retrying continues the same return to the previous version.",
    'recovery.automatic.inspect': "On this server, open a terminal and inspect the recovery log (if you are signed in as root, leave out sudo):",
    'recovery.automatic.resume': "Resolve the reported cause, then use the one-time retry command shown in that log for this operation. It continues the existing recovery. This page checks for its result; checking or reloading does not restart recovery.",
    'recovery.automatic.retryTitle': "Last recorded state: automatic recovery will try again",
    'recovery.automatic.retryHelp': "The last automatic recovery attempt did not finish. The server tries this same operation again by itself, normally about 30 seconds after the previous attempt ended, up to three attempts in total. Nothing is needed now: the server owner has to act only if automatic recovery pauses. This page checks for the result; checking or reloading does not start anything.",
    'recovery.automatic.renewal': "Automatic certificate renewal (Certbot) was stopped for this update. The recovery log says whether it was returned to how it was before the update or stays stopped until this operation finishes.",
    'recovery.automatic.renewalOff': "Certbot renewal was already off before this update; the update did not stop it.",
    'recovery.automatic.pausingTitle': "Last recorded state: automatic recovery is finishing its last attempt",
    'recovery.automatic.pausingHelp': "No automatic attempt remains. Within about a minute the server records the pause with the next step and the retry command; nothing is needed before then.",
    'recovery.reason.update_preflight_refused': "Stopped by a read-only check before any change",
    'recovery.automatic.cause.recovery_runtime_preflight_failed': "The update stopped in its read-only check, before the installed version was changed; that stop needs no recovery.",
    'recovery.reason.recovery_runtime_preflight_failed': "The update stopped in its read-only check, before the installed version was changed",
    'recovery.failure.recovery_runtime_preflight_failed.stopped': "The update stopped in its read-only check, before the installed version was changed or any service was stopped. The server keeps running the version it had, and nothing more happens for this operation. The reason is shown with this update on the panel's update page; the server owner needs to act only if it names a condition on this server. Starting the update again is safe.",
    'recovery.wait.next': "No action is needed for this wait. The server's recovery timer will check the same operation again when the operating system is ready. Recovery is not yet complete; this page checks for its result automatically.",
    'recovery.next.recovering': "The server last reported automatic recovery. Wait for a verified recovery result before starting another operation.",
    'recovery.next.recovered': "The server verified restoration of the previous release. Reload CelikPanel to check panel access.",
    'recovery.next.succeeded': "The server verified this update. Reload CelikPanel to load the installed interface.",
    'recovery.next.failed': "The server owner must inspect the update log using this operation ID. Keep the ID and check the recovery result before retrying an update.",
    'recovery.next.recovery_required': "The server owner must inspect the recovery log for this operation. Keep the operation ID; do not start another update until recovery is verified.",
    'recovery.reason.update_failed': "Update failed",
    'recovery.reason.recovery_failed': "Recovery failed",
    'recovery.reason.recovery_incomplete': "Recovery is not verified",
    'recovery.reason.candidate_panel_startup_check_failed': "The new version's panel failed its start check",
    'recovery.reason.panel_start_unverified': "The new version's panel did not start after the update",
    'recovery.reason.update_finishing_scheduler': "Turning automatic certificate renewal back on did not confirm on the first try",
    'recovery.failure.candidate_panel_startup_check_failed.returning': "The new version's panel failed its start check before anything was switched on. The server is returning to the previous version automatically; nothing needs to be done on the server. This page checks the same operation for the verified result.",
    'recovery.failure.candidate_panel_startup_check_failed.recovered': "The new version's panel failed its start check before anything was switched on, so the server was returned to the previous version automatically. The previous version keeps running and nothing needs to be done on the server. When you report this, include the reason line shown for this update on the update page.",
    'recovery.failure.panel_start_unverified.pending': "The update was applied, but the new version's panel did not come up. The server owner should read the panel log on the server with sudo journalctl -u celikpanel-panel -n 50. Completion is retried automatically up to its limit; after that, the recovery log shows a one-time retry command. There is no supported return to the previous version from this point.",
    'recovery.failure.update_finishing_scheduler.pending': "The new version is installed and running. One final step, turning automatic certificate renewal back on, did not confirm on the first try, and the server is repeating it by itself. Nothing needs to be done.",
    'app.tagline': 'Control Panel',
    'app.pageLoadFailed': 'This section could not be loaded. CelikPanel may have been updated while this tab was open.',
    'app.reload': 'Reload CelikPanel',

    // First page load (2026-10-10, seventh native record, cell 5): drawn only
    // after the 1.5 s quiet time, while the first read has still not answered.
    // Ilk sayfa yuklemesi: yalnizca 1,5 sn sessiz sureden sonra cizilir.
    'recovery.waitingHelp': "The Panel has not answered yet. CelikPanel opens as soon as it does; you do not need to do anything.",
    'recovery.waitingProlonged': "This has taken longer than half a minute. You can also reload CelikPanel; if the Panel still does not answer, the server administrator checks that the CelikPanel service is running.",
    'recovery.loadingTitle': "Opening CelikPanel",
    'recovery.loadingHelp': "Your session is confirmed and the Panel is ready. The interface is still loading and opens by itself.",
    // End of the first page load block.

    'nav.dashboard': 'Dashboard',
    'nav.domains': 'Domains',
    'nav.databases': 'Databases',
    // "Components": the page holds every installed piece of server software
    // typed by kind (service/runtime/tool) — naming it Services became false the
    // day PHP, phpMyAdmin and nftables joined the list (D-016; consultant panel
    // 23 Jul). "Service" lives on as a kind, so support language keeps working.
    // "Bileşenler": sayfa, kurulu her sunucu yazılımını cinsiyle tutar —
    // PHP, phpMyAdmin ve nftables listeye girdiği gün Services adı yalanlaştı
    // (D-016). "Servis" bir cins olarak yaşıyor; destek dili bozulmaz.
    'nav.services': 'Components',
    'nav.users': 'Users',
    'nav.audit': 'Audit log',
    'nav.addons': 'Store',
    'login.totpLabel': 'Authentication code',
    'login.totpHint': 'Enter the 6-digit code from your authenticator app.',
    'login.invalidCode': 'Invalid code. Try again.',
    'login.verify': 'Verify',
    'panelUpdate.title': 'CelikPanel updates',
    // The sidebar's quiet link when the Panel found a newer signed release (D-035).
    'panelUpdate.notice.sidebar': 'Update available · {version}',
    'panelUpdate.targetVersion': 'Target version',
    'panelUpdate.sessionExpired': 'The session ended. After you sign in again, the same operation will continue to be tracked.',
    'panelUpdate.sessionExpiredBeforeStart': 'The session ended before the update was accepted. Sign in again and check for updates.',
    'panelUpdate.adminOnly': 'This operation is available only to the primary administrator account.',
    'panelUpdate.timeout': 'The response timed out; the same request identity is still being tracked.',
    'panelUpdate.busy': 'Another server operation is running.',
    'err.PANEL_UPDATE_UNAVAILABLE': 'Signed-update trust is unavailable for this installation. Open Server → Security and review Signed-update trust.',
    'err.PANEL_UPDATE_BUSY': 'Another server operation is running. Wait for it to finish, then check again.',
    'err.PANEL_UPDATE_TARGET_CHANGED': 'The signed update target changed. Check for updates again before confirming.',
    'panelUpdate.rateLimited': 'Too many requests were sent. Tracking will retry shortly.',
    'panelUpdate.restarting': 'The panel or agent may be restarting; the same operation is still being tracked.',
    'panelUpdate.invalidResponse': 'The update request could not be verified.',
    'panelUpdate.notFoundYet': 'The request record is not visible yet; the same identity is being tracked automatically.',
    'panelUpdate.notAccepted': 'No durable request appeared within the safe verification window. The update was not accepted; check again.',
    'panelUpdate.identityMismatchCleared': 'The different operation identity is terminal. The local pending marker was cleared; check again.',
    'panelUpdate.succeeded': 'The update completed. The panel and agent restarted on the new version.',
    'panelUpdate.reloading': 'The update completed. Reloading this page with {version}…',
    'panelUpdate.failed': 'The update could not be completed.',
    'panelUpdate.dismissFailure': 'Close notification',
    'panelUpdate.failureAcknowledgement': 'Closing this notification lets you review updates again. It does not start another update.',
    'panelUpdate.packageManagerBusy': 'The server was running a package operation. CelikPanel stopped before changing the installed version or its data. Wait for that operation to finish, then retry from Settings → CelikPanel updates.',
    'panelUpdate.running': 'The update is being applied; the panel may be unavailable briefly.',
    'panelUpdate.queued': 'The update is queued.',
    'panelUpdate.connectionLost': 'The connection was interrupted; the panel may be restarting. The same operation will be tracked.',
    'panelUpdate.markerFailed': 'The request identity could not be stored in the browser; the update was not started.',
    'panelUpdate.sending': 'Sending the update request…',
    'panelUpdate.accepted': 'The request was accepted; secure status tracking started.',
    'panelUpdate.watch': 'Navigate; updates locked.',
    // --- update notice: what the last read of the followed update established (2026-10-10) ---
    'panelUpdate.tracking.reading': 'Reading the state of this update from the server…',
    'panelUpdate.tracking.verifyingTitle': 'Update installed, being verified',
    'panelUpdate.tracking.verifying': '{version} is installed and this panel is running it. The update is being verified and is not finished yet; this notice follows it by itself. You do not need to do anything.',
    'panelUpdate.tracking.unknown': 'The state of this update could not be read just now, so whether it is still running or has finished is not known. This notice reads it again by itself; do not start another update.',
    'panelUpdate.tracking.restarting': 'The panel is restarting as part of the update. This page reconnects by itself; nothing needs to be pressed.',
    'panelUpdate.tracking.restartOverdue': "The panel has not answered for longer than an update normally needs to restart it. This does not mean the update failed: the server keeps the update's record, and this notice keeps reading it by itself. If the panel still does not answer, check on the server whether it is running: sudo systemctl status celikpanel-panel.",
    'panelUpdate.lastRead': 'Last read',
    // --- end of the update notice block ---
    // --- update steps: one bar per step (owner design 2026-10-16) ---
    'panelUpdate.steps.label': 'Update steps',
    'panelUpdate.steps.title': 'Updating to {version}',
    'panelUpdate.steps.download': 'Download the update package',
    'panelUpdate.steps.verify': 'Check the signature and checksums',
    'panelUpdate.steps.prepare': 'Check the server before installing',
    'panelUpdate.steps.restart': 'Install the new version and restart the panel',
    'panelUpdate.steps.confirm': 'Verify the new version',
    'panelUpdate.steps.restarting': 'The panel is restarting; this step can take a few minutes. This page reconnects by itself.',
    'panelUpdate.steps.locked': 'Other pages stay locked while the update runs.',
    'panelUpdate.steps.updated': 'Updated to {version}',
    'panelUpdate.steps.state.done': 'done',
    'panelUpdate.steps.state.running': 'in progress',
    'panelUpdate.steps.state.pending': 'not started yet',
    'panelUpdate.steps.state.retrying': 'repeating automatically',
    'panelUpdate.steps.retrying': 'Repeating automatically (attempt {attempt} of {max})',
    'panelUpdate.steps.retryingPlain': 'Repeating automatically',
    'panelUpdate.steps.repeated': 'One step was repeated automatically; nothing needs to be done.',
    // --- end of the update steps block ---
    'panelUpdate.canonicalChecking': 'Checking unfinished updates...',
    'panelUpdate.interactionLocked': 'Navigation is temporarily locked while this update is checked. It unlocks by itself after two minutes; if the panel is still restarting then, once it answers again, at the latest six minutes after the start. Tracking continues.',
    'panelUpdate.ambiguous': 'The start response is uncertain. This request will not be repeated; its exact server status will be tracked.',
    'panelUpdate.outcome.rolledBackTitle': 'Update not completed; previous version restored',
    'panelUpdate.outcome.rolledBack': 'The update to {target} did not complete. The server was returned to {previous} automatically and is running it now.',
    'panelUpdate.outcome.cause.candidate_panel_startup_check_failed': "Cause: the new version's panel failed its start check before anything was switched on.",
    'panelUpdate.outcome.cause.panel_start_unverified': "Cause: the new version's panel did not start and answer after the update was applied.",
    'panelUpdate.outcome.cause.update_finishing_scheduler': "Cause: turning automatic certificate renewal back on did not confirm after the new version was installed.",
    'panelUpdate.finishing.title': 'Finishing the update',
    'panelUpdate.finishing.body': '{version} is installed and running. One final step, turning automatic certificate renewal back on, did not confirm on the first try. The server is repeating it by itself. Nothing needs to be done; this page updates by itself.',
    'panelUpdate.outcome.checkingTitle': "Checking the update's result",
    'panelUpdate.details': 'Details',
    'panelUpdate.outcome.cause.generic': 'Cause: the update failed before it completed; the server recorded no more specific cause.',
    'panelUpdate.outcome.rolledBackNext': "Nothing needs to be done on the server. Do not start the update to {target} again until a corrected version is published. If the server's message names a problem on this server, fix it first.",
    'panelUpdate.outcome.rolledBackResume': 'Nothing resumes by itself: the server keeps running {previous}. When a newer version is published, “Check for updates” offers it.',
    'panelUpdate.outcome.followsRecovery': "This notice follows the server's recovery record for this update and refreshes by itself. It does not start anything.",
    'panelUpdate.outcome.unknownResult': 'The server has not reported a recovery result for this update yet, so the version it runs now is not known. This notice checks again by itself; do not start another update until a result appears.',
    'panelUpdate.outcome.checking': "Reading the server's recovery record for this update…",
    'panelUpdate.outcome.serverMessage': 'The server reported: {message}',
    'panelUpdate.outcome.cause.recovery_runtime_preflight_failed': "Cause: the update stopped in its read-only check, before the installed version was changed.",
    'panelUpdate.outcome.stoppedTitle': "Update stopped before changing the installed version",
    'panelUpdate.outcome.stopped': "The update to {target} stopped in its read-only check, before the installed version was changed or any service was stopped. The server is running {previous} as before.",
    // The recovery tool's first two checks (cmd/recovery/compatibility.go):
    // mode, the kind of check the updater asked for; owner, that it runs as root.
    'panelUpdate.outcome.preflightStep.mode': "Reason: the recovery tool was asked for a kind of check it does not know, so it did not check the installation.",
    'panelUpdate.outcome.preflightStep.owner': "Reason: the read-only check was not running with the server's administrator (root) rights, so it could not read the installation.",
    'panelUpdate.outcome.preflightStep.panel_database_check': "Reason: the read-only check of the panel database did not pass.",
    'panelUpdate.outcome.preflightStep.agent_ledger_check': "Reason: the read-only check of the Agent's operation record did not pass; another operation may still be in progress.",
    'panelUpdate.outcome.preflightStep.owner_metadata_probe': "Reason: the ownership or permissions of installed CelikPanel files could not be verified.",
    'panelUpdate.outcome.preflightStep.runtime_selection': "Reason: the selected recovery runtime could not be verified.",
    'panelUpdate.outcome.preflightStep.runtime_revalidation': "Reason: the selected recovery runtime could not be verified again during the check.",
    'panelUpdate.outcome.preflightStep.release_boundary': "Reason: the update lock, or that no other update is pending, could not be verified.",
    'panelUpdate.outcome.preflightStep.material_support': "Reason: the selected recovery runtime does not support this update's recovery data.",
    'panelUpdate.outcome.preflightStep.database_support': "Reason: the selected recovery runtime does not support this update's database migration.",
    'panelUpdate.outcome.preflightStep.database_metadata': "Reason: the panel database's metadata is not supported for this update's migration.",
    // alpha.97: the reason line while a step's words load (the step texts of alpha.97 are in the settings screen part).
    'panelUpdate.outcome.reasonLoading': "Reading the reason…",
    'panelUpdate.outcome.preflightStep.generic': "Reason: a read-only check could not verify the current installation.",
    'panelUpdate.outcome.stoppedNext': "Nothing needs to be done on the server, unless the server's message below names a condition on this server, such as another operation still running or the package manager busy: then let it finish or fix it first.",
    'panelUpdate.outcome.stoppedResume': "Nothing resumes by itself. Starting the update to {target} again is safe.",
    'nav.settings': 'Settings',
    'nav.menu': 'Menu',
    'nav.collapse': 'Collapse menu',
    'nav.expand': 'Expand menu',
    'common.sudoNote': 'If you are signed in as root, leave out sudo.',
    'nav.monitoring': 'Monitoring',
    'nav.serverSetup': 'Server setup',
    'nav.dnsPeers': 'DNS and peer servers',
    'nav.security': 'Security',
    'nav.maintenance': 'Maintenance',
    'nav.account': 'My account',
    'nav.group.hosting': 'Hosting',
    'nav.group.server': 'Server',

    'login.subtitle': 'Sign in to your control panel',
    'login.username': 'Username',
    'login.password': 'Password',
    'login.showPassword': 'Show password',
    'login.hidePassword': 'Hide password',
    'login.signIn': 'Sign in',
    'login.signingIn': 'Signing in…',
    'login.invalid': 'Invalid username or password.',
    'login.failed': 'Sign-in failed. Please try again.',
    'login.tooMany': 'Too many attempts. Please wait a moment and try again.',
    'login.demoTitle': 'Demo accounts',
    'login.demoHint': 'Development mode — click a role to fill the form.',

    'theme.label': 'Theme',
    'theme.light': 'Light',
    'theme.dark': 'Dark',
    'theme.system': 'System',
    'theme.skin': 'Style',

    'lang.label': 'Language',

    'user.logout': 'Sign out',
    'role.admin': 'Administrator',
    'role.reseller': 'Reseller',
    'role.customer': 'Customer',
    'role.additional_user': 'User',
    'dashboard.serverInfo': 'Server information',
    'dashboard.hostname': 'Hostname',
    'dashboard.ipv4': 'IPv4 address',
    'common.days': 'd',
    'common.hours': 'h',
    'common.minutes': 'm',

    'common.loading': 'Loading…',
    'common.error': 'Something went wrong.',
    'common.goFix': 'Fix it',
    'common.andMore': '+{n} more',
    // The API error contract (B1): localized texts for coded refusals.
    // err.<CODE> overrides the server message; err.<CODE>.action labels the
    // fix-it button when the refusal carries an in-panel path.
    'err.license_required': "An active server license is required to use the panel. Existing services and scheduled tasks keep running. Your server administrator must activate or renew the license.",
    'err.AUTH_REQUIRED': 'Your session has expired — sign in again.',
    'err.ADMIN_ONLY': 'This action requires an administrator.',
    'err.ACCOUNT_SUSPENDED': 'This account is suspended.',
    'err.INTERNAL': 'The server hit an internal error. Try again; if it persists, check the panel logs.',
    'err.AGENT_UNREACHABLE': 'The CelikPanel Agent on this server is not reachable, so this request was not carried out and nothing was changed. The server owner checks the Agent on the server with sudo systemctl status celikpanel-agent (its log: sudo journalctl -u celikpanel-agent) and, if it is stopped, starts it with sudo systemctl start celikpanel-agent. Once the Agent is running, try again.',
    'err.PLATFORM_CAPABILITY_UNAVAILABLE': 'This operation is not available for the connected server platform.',
    'err.PLATFORM_IDENTITY_UNAVAILABLE': 'The connected server\'s platform identity could not be verified. Try again; if it persists, check the agent connection.',
    // Scheduled tasks need the server's native cron (upd1, 1 Oct 2026). The
    // base sentence answers a refused change; .read answers the list.
    // A directory above the hosting root that CelikPanel did not create blocks
    // the web server or the site users (native finding P3, 1 Oct 2026).
    'err.SERVICE_STATE_UNVERIFIED': 'The stored record of this server’s components could not be verified, so it is not shown and nothing was changed. CelikPanel checks the components again by itself while the Dashboard, the Components page or a component’s page is open; open this page again after that check.',
    'err.HOST_MUTATION_BUSY': 'Another server change or operating-system package task is still running. Wait for it to finish, then try again.',
    'err.HOST_MUTATION_BUSY.package_manager_active': 'This server\'s package manager is busy — a package task is still running on this server. Wait for it to finish, then try again.',
    'err.HOST_MUTATION_BUSY.agent_mutation_active': 'Another CelikPanel change is still running on this server. Wait for it to finish, then try again.',
    'err.HOST_MUTATION_BUSY.panel_operation_active': 'Another CelikPanel operation is still running. Wait for it to finish, then try again.',
    'err.HOST_MUTATION_BUSY.host_lock_busy': 'A change that did not finish is still holding this server, and it will not clear by waiting. Restarting the server releases the hold; if it comes back, this needs looking at.',
    'err.HOST_MUTATION_UNAVAILABLE': 'CelikPanel could not safely verify that the server is ready for changes. Refresh the page and try again.',
    'err.service_operation_busy': 'Another package operation is already in progress. Wait for it to finish.',
    'err.service_operation_request_conflict': 'This request identifier already belongs to a different package operation. Reload and try again.',
    'err.service_operation_runner_panicked': 'The package operation stopped unexpectedly. Its final machine state must be verified before another action.',
    'err.operation_state_persist_failed': 'The package operation state could not be saved. Verify the machine state before another action.',
    'err.operation_start_failed': 'The package operation could not be started.',
    'err.service_install_failed': 'The service could not be installed and verified.',
    'err.node_runtime_install_failed': 'The Node.js runtime could not be installed and verified.',
    'err.mail_profile_install_failed': 'The mail profile could not be installed and verified.',
    'common.buildHint': 'The git revision this panel is running. It comes from the running server, so it changes whenever a new build is actually live.',
    'common.fullCommit': 'full commit {commit}',
    'common.agentMismatch': 'agent mismatch',
    'common.agentMismatchHint': 'The background service is running a different build ({commit}) from the panel. Re-run the update so both sides match — until then they may disagree about what is allowed.',
    'common.retry': 'Retry',
    'common.staleNotice': 'This could not be read again from the server just now, so what is shown below is as it was at {time}. Nothing was changed. Controls that remove or change something are off until it has been read again.',
    'common.resultUnknown': 'The connection dropped before the answer arrived, so it is not known whether the change was made. Nothing is sent a second time. What is shown is being read again; check it before repeating the action.',
    'common.resultUnknownRead': 'The connection dropped before the answer arrived, so it is not known whether the change was made. Nothing was sent a second time. What is shown here was read again at {time}: check it before repeating the action.',
    'common.resultUnknownMade': 'The connection dropped before the answer arrived, and nothing was sent a second time. What was read again at {time} shows the change, so it was saved. Nothing needs to be sent again.',
    'common.resultUnknownNotMade': 'The connection dropped before the answer arrived, and nothing was sent a second time. What was read again at {time} does not show the change, so it is not known to have been saved. What you entered is still here. If the server was still working when the connection dropped, the change can appear later: check again before sending it a second time.',
    'common.resultUnknownUnread': 'The connection dropped before the answer arrived, so it is not known whether the change was made. Nothing was sent a second time. The current state could not be read again either, so controls that change or remove something stay off. Check again.',
    'common.checkAgain': 'Check again',
    'common.cancel': 'Cancel',
    'common.close': 'Close',
    'common.back': 'Back',
    // Plural by the count: '1 item', '{n} items' (alpha.92 Domains: "1 items total").
    'common.itemsTotalOne': '1 item',
    'common.itemsTotal': '{n} items',
    'common.perPage': 'Entries per page',
    'common.search': 'Search…',
    'common.remove': 'Remove',
    'common.home': 'Home',
    // "Not checked yet" is not "not installed". The panel says which of the
    // two it means, because a restored or freshly installed server has a real
    // catalogue and no observation of it at all.
    // "Henüz bakılmadı", "kurulu değil" demek değildir.
    'services.notChecked': 'Not checked yet',
    'common.copy': 'Copy',

    'nav.vpn': 'VPN',
    'dnssrv.continue': 'Continue',
    'services.install': 'Install',
    'services.installed': '{name} installed.',
    'services.phpToolProblem.versionGone': '{tool} is set to PHP {version}, which is no longer installed, so {tool} cannot answer. Open {tool} in Components and choose an installed PHP version in Manage.',
    'services.phpToolProof.moved': '{tool} runs on PHP {version}: it did not work on PHP {first} ({why}), so the next installed PHP version that has its extensions was used. You can change it in {tool}’s Manage window.',
    'services.phpToolProof.notReady': '{tool} was installed, but it did not answer on PHP {version} ({why}). It reads Not ready: open its Manage window in Components; the reason is shown there, and Check again asks it again without moving it.',
    'services.phpToolProof.whyMissing': 'PHP {version} lacks {extensions}',
    'services.phpToolProof.whyOther': 'it did not answer as itself',
    // alpha.109: a refused install lists what it would have brought, one package a line, grouped by reason.
    'services.installRefused.notable': 'Nothing was installed: it would also have brought the packages below, which were not confirmed — the package manager’s answer changed after the install dialog was shown. Press Install again: the dialog now lists them for you to confirm.',
    // alpha.109: why a package comes with an install, said once for the dialog and the refusal.
    'services.notable.why.new_php_version': 'part of another PHP version ({version})',
    'services.notable.why.apache': 'the Apache web server',
    'services.notable.why.component': 'another component: {component}',
    'services.notable.why.database_server': 'a database server',
    'services.notable.why.upgrade': 'already installed; would be upgraded although the install did not ask for it',
    'services.notable.why.vendor_shared': 'the {repo} repository’s version instead of the distribution’s',
    'services.notable.repoUnknown': 'vendor',
    'services.notable.repoName.sury': 'Sury PHP',
    'services.notable.repoName.pgdg': 'PostgreSQL (PGDG)',
    'services.notable.repoName.netdata': 'Netdata',
    'services.phpToolProblem.extensionsFailed': 'The PHP extensions {tool} needs could not be installed for PHP {version}, so {tool} may not work. Open {tool} in Components and choose PHP {version} again in Manage, or another version.',
    'services.mailProfiles.fallbackWarning': '{name} installed with a self-signed fallback certificate. Mail clients will show a trust warning until a matching trusted certificate is configured.',
    'services.operation.title': 'Installing {name}',
    'services.operation.activeCheckInProgress': 'The panel is still checking for an active package operation. Try again in a moment.',
    'services.operation.actionLocked': 'Another package operation or safety check is already in progress.',
    'services.operation.starting': 'Starting the server operation…',
    'services.operation.running': 'Installation is running…',
    'services.operation.recoveringRequest': 'Checking whether the previous installation request reached the server…',
    'common.reloadPage': 'Reload page',
    'login.panelAddressHint': 'Configured HTTPS address for this panel:',
    'license.connectionRetry': 'The latest access check failed. Retrying automatically; the current page is preserved while the last verified access remains valid.',
    'services.operation.reloadHelp': 'Reloading reconnects to the recorded operation. It does not start another installation.',
    'services.operation.reconnecting': 'Connection interrupted. Reconnecting without unlocking the panel…',
    // Reconnecting is active and bounded (owner rule 2026-10-14): the page reads by itself every 3 s.
    // Yeniden bağlanma etkin ve sınırlıdır: sayfa her 3 sn'de kendisi okur.
    'services.operation.reconnectingAuto': 'This page asks the server again every few seconds by itself; you do not need to press anything.',
    'services.operation.reconnectSlow': "This page has not had a usable answer from the panel for more than a minute, so the result of this operation is not known yet. This does not mean it failed: the server keeps the operation's record, and this page keeps asking by itself. Do not start the operation again. If the panel still does not answer, check on the server whether it is running: sudo systemctl status celikpanel-panel. If it is stopped, start it with sudo systemctl start celikpanel-panel, then reload this page.",
    'services.operation.refreshing': "Server operation finished. Verifying the current service state…",
    'services.operation.backgroundHint': 'The job continues on the server. Reloading this page is safe; the panel will reattach with the operation id or its pre-request recovery marker. Other panel actions stay locked until the result is known.',
    'services.operation.id': 'Operation: {id}',
    'services.operation.dismiss': 'Dismiss installation error',
    'services.operation.startFailed': 'The installation could not be started. The server did not return an active operation.',
    'services.operation.recoveryStorageUnavailable': 'The installation was not started because this browser could not preserve a recovery marker. Enable session storage and try again.',
    'services.operation.failedTitle': "{name}: installation failed",
    'services.operation.failedChecking': "Installation failed. Checking the resulting service state.",
    'services.operation.failedCheckingHint': "The error is recorded. CelikPanel is checking what changed before allowing another operation. The installation will not restart automatically.",
    'services.operation.failedConnectionHint': "The installation failed, and the resulting service state cannot currently be checked. Check the panel and agent connection. CelikPanel will retry the state check, not the installation.",
    'services.operation.uncertainHint': "The latest server state is not confirmed. CelikPanel is checking the same request; do not start a second installation. If this persists, check the server connection and use the operation ID to inspect its logs.",
    'services.operation.verifyingHint': "The server operation ended. CelikPanel is verifying the current service state before allowing another change. Waiting for this check does not run the installation again.",
    'services.operation.failureNextStep': "Follow the recovery instructions above. Before starting another change, review the component state and any reported details. Closing this message does not retry the operation.",
    'services.operation.phase.scanning': "Checking the installed services…",
    'services.operation.phase.syncing': "Applying the related service configuration…",
    'services.operation.phase.syncing_dns': "Publishing the required DNS configuration…",
    'services.operation.failed': 'The installation failed on the server.',
    'services.operation.phase.queued': 'Queued on the server…',
    'services.operation.phase.running': 'Installation is running…',
    'services.operation.phase.preparing': 'Preparing packages…',
    'services.operation.phase.downloading': 'Downloading packages…',
    'services.operation.phase.installing': 'Installing packages…',
    'services.operation.phase.configuring': 'Configuring the service…',
    'services.operation.phase.starting': 'Starting the service…',
    'services.operation.phase.verifying': 'Verifying the service…',
    'services.operation.phase.finalizing': 'Finishing the installation…',
    'users.cancel': 'Cancel',

    'imp.banner': 'You are browsing as {name}.',
    'imp.return': 'Return to my account',

    'profile.changePassword': 'Change password',
    'profile.changeEmail': 'Change email',
    'profile.current': 'Current password',
    'profile.new': 'New password',
    'profile.new2': 'New password (again)',
    'profile.mismatch': 'The new passwords do not match.',
    'profile.changed': 'Password changed.',
    'profile.wrongCurrent': 'Current password is incorrect.',

    'nav.import': 'Import',
    // --- batch 2b (9 Oct 2026): database and mail configuration screens: checking,
    // could not be read, known; saves that carry the version of what was read ---
    // --- end of batch 2b ---
    // --- set1 corrections (10 Oct 2026): a reload is verified, never inferred; no cause is
    // named that was not verified; what the server holds after a failed reload ---
    // --- end of set1 corrections ---

    // A change is sent once and answered once (D-029). Order: what happened,
    // what was and was not changed, the next action.
    'err.REQUEST_ID_REQUIRED': 'This page was opened before CelikPanel was updated, so the server did not accept the change and nothing was changed. Reload the page, then make the change again.',
    'err.REQUEST_ID_REUSED': 'This change was sent with an identifier the server already used for a different change, so it was not carried out. Reload the page, then make the change again.',
    'err.REQUEST_IN_PROGRESS': 'This change is still running on the server. It was not started a second time. Wait a little, then reload the page to see the result; do not send it again.',
    'err.REQUEST_OUTCOME_UNKNOWN': 'CelikPanel restarted or failed while this change was running, so it is not known whether the change was completed. It will not be run again by itself. Reload the page and check the current state; make the change again only if it is missing.',
    'err.REQUEST_COMPLETED_RESULT_NOT_RETAINED': 'This change was already made; it was not made a second time. Its result was shown only once and is not kept. Reload the page to see the current state; if you still need what was shown once (a password or a configuration file), create a new one.',
    'err.REQUEST_COMPLETED_RESULT_NOT_RETAINED.failed': 'This change already ended with an error, and that answer is not kept; it was not tried a second time. Reload the page and check the current state; make the change again only if it is missing.',

    // A change on a route that carries an identity whose result is not known
    // (D-029 with lib/lostAnswer.ts): the first sentence says what happened, the
    // second what the state that was read again shows.
    'common.lostAsked': "No answer arrived for this change, and asking the server once more for the same answer brought none either, so it is not known whether the change was made. Asking again never makes the change a second time.",
    'common.lostInterrupted': "CelikPanel restarted or failed while this change was running, so it is not known whether the change was completed. It will not be run again by itself.",
    'common.lostRunning': "This change is still running on the server, so its result is not known yet. It was not started a second time.",
    'common.lostStateReading': "What is shown here is being read again. Controls that change or remove something stay off until it has been read.",
    'common.lostStateRead': "What is shown here was read again at {time}. Check it before making the change again; if the change may still be running, check again in a little while.",
    'common.lostStateUnread': "The current state could not be read again, so controls that change or remove something stay off. Check again.",
    'common.lostStateMade': "The answer to this change did not reach this page, but what was read again at {time} shows the change, so it was made. Nothing needs to be sent again.",
    'common.lostStateNotMade': "What was read again at {time} does not show the change, so it is not known to have been made. What you entered is still here. If the server is still working on it, the change can appear later: check again before sending it a second time.",

    // Start, Stop, Restart and Reload on the Services page (10 Oct 2026): what the
    // service showed, the one command the server owner runs, how the work resumes.
    // An unknown result is not a failure.

    // --- first page load: the interface is still loading after half a minute (2026-10-10) ---
    'recovery.waitingProlongedLoading': "CelikPanel is still loading; it opens by itself. If this page stays like this, reload it.",
    // --- end of the first page load block ---
    // --- a site configuration file the owner changed: refusals (D-031, 2026-10-10) ---
    // --- end of the site configuration refusals block ---
    // --- an access read that has not answered, and one that answered with a failure (ninth native record, 2026-10-10) ---
    'recovery.waitingLong': "The Panel has not answered for a while. CelikPanel keeps checking by itself; you can also check now.",
    'recovery.checkNow': "Check now",
    'recovery.failure.network': "The connection to the Panel failed before it answered (refused, closed or unreachable). If this continues, the server administrator checks that the CelikPanel service is running.",
    'recovery.failure.status': "The server answered with HTTP error {status}. If this continues, the server administrator checks the CelikPanel service and its log.",
    'recovery.lastKnown': "Last known, before this wait: {cause}",
    'recovery.failure.invalid': "The Panel answered, but this page could not read the answer. Reloading CelikPanel loads the interface that matches the Panel.",
    // --- end of the unanswered access read block ---
} as const;

export type ShellKey = keyof typeof en;
// Every screen part's keys; a part missing here would make its keys untyped.
export type ScreenKey =
    | SetupScreenKey
    | SitesScreenKey
    | SiteConfigScreenKey
    | DnsCertificatesScreenKey
    | ComponentsScreenKey
    | ComponentPagesScreenKey
    | MailDatabasesScreenKey
    | SecurityScreenKey
    | DashboardScreenKey
    | SettingsScreenKey
    | ErrorsScreenKey
    | LifecycleScreenKey
    | DatabaseClustersScreenKey
    | VendorLanesScreenKey
    | WebmailAddressScreenKey
    | SetupWizardScreenKey
    | SetupRegistrarScreenKey
    | SetupJoinScreenKey
    | SetupLinkScreenKey
    | SetupInspectionScreenKey
    | DnsOverviewScreenKey
    | NginxDefaultHttpsScreenKey
    | DnsPublicationScreenKey
    | ProblemsScreenKey;
export type TranslationKey = ShellKey | ScreenKey | SetupDNSKey;

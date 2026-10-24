package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/hostplatform"
	"github.com/alicelik/celikpanel/internal/transport"
)

// One-click service installation — the flip side of "what isn't installed is
// invisible": nothing is preinstalled, and the admin adds exactly the
// services they want. The service ID is validated against the fixed
// catalogue, so only known packages are ever installed; the distro family
// decides the real package names.
//
// Tek tıkla servis kurulumu — "kurulu olmayan görünmez"in öteki yüzü: hiçbir
// şey önceden kurulmaz ve yönetici tam istediği servisleri ekler. Servis
// ID'si sabit kataloğa karşı doğrulanır; böylece yalnız bilinen paketler
// kurulur; gerçek paket adlarına dağıtım ailesi karar verir.

type InstallServiceRequest = transport.InstallServiceRequest

type InstallServiceResponse = transport.InstallServiceResponse

const genericDNSEngineWorkflowRequired = "BIND and PowerDNS must be installed, activated, switched, removed, or controlled through the dedicated DNS infrastructure workflow"

func genericDNSEngineMutationRefusal(service *core.ManagedService) string {
	if service != nil && service.ConflictGroup == "dns-server" {
		return genericDNSEngineWorkflowRequired
	}
	return ""
}

// validateRepoPackageSelection accepts a caller-selected package only when the
// service catalogue explicitly declares a non-empty PackagePattern and that
// pattern matches the entire package name. This validation is intentionally
// pure so it runs before any package-manager probe.
//
// validateRepoPackageSelection, çağıranın seçtiği paketi yalnızca servis
// kataloğu boş olmayan bir PackagePattern tanımlıyorsa ve desen paket adının
// tamamıyla eşleşiyorsa kabul eder. Bu doğrulama bilerek saftır; paket
// yöneticisine herhangi bir sorgu yapılmadan önce çalışır.
func validateRepoPackageSelection(svc *core.ManagedService, packageName string) ([]string, error) {
	if svc == nil || svc.Repo == nil {
		return nil, fmt.Errorf("this service does not offer version selection")
	}
	pattern := strings.TrimSpace(svc.Repo.PackagePattern)
	if pattern == "" {
		return nil, fmt.Errorf("this service does not offer version selection")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid package pattern in catalogue")
	}
	match := re.FindStringSubmatch(packageName)
	if match == nil || match[0] != packageName {
		return nil, fmt.Errorf("%q is not a valid version package for %s", packageName, svc.Name)
	}
	return match, nil
}

// InstallService installs a managed service by its catalog ID, then enables +
// starts its systemd unit so it is actually running (and survives reboot)
// right after. Already-present services take the same idempotent preparation,
// configuration and readiness path so a partial install can be repaired.
// InstallService, katalog kimliğiyle yönetilen bir servisi kurar, sonra
// systemd unit'ini etkinleştirip başlatır; böylece hemen ardından gerçekten
// çalışır (ve reboot'tan sağ çıkar). Zaten kurulu servisler dürüstçe
// bildirilen bir no-op'tur.
func (a *Agent) InstallService(req *InstallServiceRequest, resp *InstallServiceResponse) error {
	*resp = InstallServiceResponse{}
	if req == nil {
		resp.Error = "missing request"
		return nil
	}
	serviceID := req.ID
	packageName := req.Package
	svc := core.GetManagedServiceByID(serviceID)
	if svc == nil {
		resp.Error = "unknown service"
		return nil
	}
	if refusal := genericDNSEngineMutationRefusal(svc); refusal != "" {
		resp.Error = refusal
		return nil
	}

	// Reject an arbitrary or non-versioned package before detectPkgFamily or
	// packageInstalled can invoke a package-manager command.
	// Keyfi ya da sürümlü olmayan paketi detectPkgFamily veya packageInstalled
	// bir paket-yöneticisi komutu çalıştırmadan önce reddet.
	var selectedPackageMatch []string
	if packageName != "" {
		var err error
		selectedPackageMatch, err = validateRepoPackageSelection(svc, packageName)
		if err != nil {
			resp.Error = err.Error()
			return nil
		}
	}

	// A PHP version is chosen only for a database tool (alpha.105).
	phpTool := phpToolForInstall(svc.ID)
	phpVersion := strings.TrimSpace(req.PHPVersion)
	if phpVersion != "" && (phpTool == "" || !phpVersionText.MatchString(phpVersion)) {
		resp.Error = "a PHP version is chosen only for phpMyAdmin and phpPgAdmin"
		return nil
	}

	authorizedReq := *req
	authorizedReq.ID = svc.ID
	authorizedReq.Package = packageName
	req = &authorizedReq
	ctx, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepInstallService, svc.ID, packageName, "install"),
	)
	if err != nil {
		*resp = InstallServiceResponse{Error: err.Error()}
		return nil
	}
	defer finishStep()
	return a.installServiceWith(ctx, req, svc, selectedPackageMatch, phpTool, phpVersion, resp)
}

// firstPHPToolInstall: the tool was not on the host before this install and
// is now. Only then may its proof move it to another PHP version; a tool
// already installed (a repair, a second install) is never moved
// (alpha.104 rule).
func firstPHPToolInstall(tool string, before bool) bool {
	return tool != "" && !before && phpToolInstalledCheck(tool)
}

// installServiceWith is InstallService's work under its held mutation step
// (alpha.105: the disposable-VM evidence calls it directly).
// installServiceWith, InstallService'in tutulan adım altındaki işidir.
func (a *Agent) installServiceWith(ctx context.Context, req *InstallServiceRequest, svc *core.ManagedService, selectedPackageMatch []string, phpTool, phpVersion string, resp *InstallServiceResponse) error {
	profile, err := detectHostPlatform()
	if err != nil {
		resp.Error = fmt.Sprintf("detect host platform: %v", err)
		return nil
	}
	family := string(profile.PackageManager)
	systemctl, err := executableForProfile(profile, family, "systemctl")
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	if reason := core.ManagedServiceInstallDisabledReason(svc, family); reason != "" {
		resp.Error = reason
		return nil
	}

	// A version pick is validated before anything else, because it changes what
	// "already installed" even means.
	// Sürüm seçimi her şeyden önce doğrulanır, çünkü "zaten kurulu"nun ne
	// demek olduğunu değiştiren şey odur.
	var versionPrefix string
	if req.Package != "" {
		// Version selection comes from a managed vendor repo, which is apt-only
		// today. Without this the pick sails through validation and dies inside
		// pacman as "target not found: php8.3-fpm" — a package-manager error
		// shown to someone who asked the panel a product question.
		// Sürüm seçimi yönetilen bir vendor deposundan gelir ve bugün yalnız
		// apt'te vardır. Bu olmadan seçim doğrulamayı geçip pacman'in içinde
		// "target not found: php8.3-fpm" diye ölür — panele ürün sorusu soran
		// birine gösterilen bir paket-yöneticisi hatası.
		if family != "apt" {
			resp.Error = "choosing a version needs a managed repository, which currently requires a verified APT package ecosystem"
			return nil
		}
		if len(selectedPackageMatch) > 1 {
			versionPrefix = selectedPackageMatch[1]
		}
	}

	// For a plain install the question is "is this service here?"; for a version
	// pick it is "is THIS VERSION here?". Asking the first question for both is
	// what made a second PHP impossible: with 8.4 present, a request for 8.3 was
	// answered "PHP-FPM is already installed" and refused — the admin could not
	// give a customer the version they needed even with Sury enabled, which is
	// the whole point of D-014's chain.
	// Düz kurulumda soru "bu servis burada mı?"dır; sürüm seçiminde "BU SÜRÜM
	// burada mı?". İkisi için de ilk soruyu sormak, ikinci bir PHP'yi imkânsız
	// kılan şeydi: 8.4 varken 8.3 isteği "PHP-FPM zaten kurulu" diye
	// reddediliyordu — Sury açık olsa bile admin, müşterinin ihtiyacı olan
	// sürümü veremiyordu; D-014'ün zincirinin tüm amacı buydu.
	alreadyPresent := a.serviceInstalledWithProfile(svc, profile)
	if req.Package != "" {
		alreadyPresent = packageInstalledForProfile(profile, req.Package)
	}

	// Requirements first: a dependent tool without its parent service would
	// install broken (phpMyAdmin with no MariaDB, no web server, no PHP). The
	// UI blocks this too; the agent is the enforcement that cannot be skipped.
	// Önce gereksinimler: üst servisi olmayan bağımlı araç bozuk kurulur
	// (MariaDB'siz, web sunucususuz, PHP'siz phpMyAdmin). UI da engeller;
	// atlatılamayan uygulayıcı agent'tır.
	installed := a.installedServiceSetWithProfile(profile)
	if missing := core.RequirementsMissing(svc, installed); len(missing) > 0 {
		resp.Error = fmt.Sprintf("%s requires %s — install that first from Services",
			svc.Name, strings.Join(missing, ", "))
		return nil
	}

	// The seat, enforced HERE and not only in the browser. Two members of a
	// seat bind the same port, so the second one installs and immediately dies
	// — proven live on Boston: with Redis holding 6379, a panel call installed
	// valkey-server and systemd gave up after five failed starts. The row said
	// "conflicts with Redis"; the API did it anyway.
	// Koltuk, yalnız tarayıcıda değil BURADA uygulanır. Bir koltuğun iki üyesi
	// aynı portu tutar; ikincisi kurulur ve anında ölür — Boston'da canlı
	// kanıtlandı: Redis 6379'u tutarken bir panel çağrısı valkey-server kurdu ve
	// systemd beş başarısız başlatmadan sonra pes etti. Satır "Redis ile
	// çakışıyor" diyordu; API yine de yaptı.
	if taken := core.SeatTakenBy(svc, installed); taken != "" {
		resp.Error = fmt.Sprintf("%s cannot run alongside %s — they do the same job and would fight over the same port. Remove %s first.",
			svc.Name, taken, taken)
		return nil
	}

	// Native cron is installed only where the host has none. Any existing
	// cron is the owner's: no package, unit or configuration change (D-022).
	// Yerel cron yalnız sunucuda hiç yokken kurulur. Var olan her cron
	// sahibinindir: paket, unit ya da yapılandırma değişmez (D-022).
	if svc.ID == core.NativeCronServiceID && nativeCronPresent(alreadyPresent, hostNativeCronProbe(systemctl)) {
		resp.PreservedExisting = true
		resp.Detail = "an existing cron service was found and left unchanged"
		return nil
	}

	pkgs := svc.Packages[family]
	if len(pkgs) == 0 {
		resp.Error = fmt.Sprintf("%s cannot be installed automatically on this system yet", svc.Name)
		return nil
	}

	// A version pick from the service's managed repo replaces the distro
	// default — but only if the service actually has a repo and the requested
	// name matches its pattern, so this can never install an arbitrary package.
	// Servisin yönetilen deposundan bir sürüm seçimi dağıtım varsayılanının
	// yerini alır — ama yalnız servisin gerçekten bir deposu varsa ve istenen ad
	// desenine uyuyorsa; böylece bu asla keyfi bir paket kuramaz.
	var skipped []string
	// alpha.107 (D-038 §1/§7): with the component's vendor repository
	// enabled, the exact version the distribution's default package brings
	// is installed in its place, never the moving default package.
	effectivePackage := req.Package
	if req.Package == "" && family == "apt" && svc.Repo != nil {
		aptCache, err := executableForProfile(profile, family, "apt-cache")
		if err != nil {
			resp.Error = err.Error()
			return nil
		}
		exact, err := exactDefaultPackage(ctx, svc, family, hostExactDefaultOps(aptCache))
		if err != nil {
			resp.Error = err.Error()
			return nil
		}
		if exact != "" {
			pkgs = []string{exact}
			effectivePackage = exact
		}
	}
	if req.Package != "" {
		pkgs = []string{req.Package}

		// A picked version installs bare without its companions — `php8.3-fpm`
		// alone has no database driver, mbstring or curl, so the panel would
		// report success for a runtime that cannot serve WordPress or Laravel.
		// Companions are filtered to what apt actually offers for THIS version
		// and the rest is reported: Sury publishes no php8.5-opcache, and a
		// strict install would refuse PHP 8.5 entirely over one extension.
		// Seçilen bir sürüm companion'ları olmadan çıplak kurulur — tek başına
		// `php8.3-fpm`in veritabanı sürücüsü, mbstring'i, curl'ü yoktur; panel,
		// WordPress ya da Laravel sunamayacak bir runtime için başarı
		// bildirirdi. Companion'lar apt'ın BU sürüm için gerçekten sunduklarına
		// süzülür, kalanı raporlanır: Sury php8.5-opcache yayınlamıyor ve katı
		// bir kurulum tek bir uzantı yüzünden PHP 8.5'i tümüyle reddederdi.
		if versionPrefix != "" && len(svc.Repo.VersionCompanions) > 0 {
			aptCache, err := executableForProfile(profile, family, "apt-cache")
			if err != nil {
				resp.Error = err.Error()
				return nil
			}
			offered, notOffered := versionCompanionPackages(svc, versionPrefix, func(name string) bool {
				return packageAvailableWithExecutable(name, aptCache)
			})
			pkgs = append(pkgs, offered...)
			skipped = notOffered
		}
	}

	// A database tool installed for a chosen PHP version (alpha.105, a
	// bundle's choice) brings its extension packages for that version; a
	// version that cannot serve the tool is refused before anything
	// changes, with the reason.
	toolBefore := phpTool != "" && phpToolInstalledCheck(phpTool)
	if phpVersion != "" {
		refuse := func(refusal *transport.InstallPreviewRefusal) {
			resp.ErrorCode = errCodeInstallPHPVersionRefused
			switch refusal.Code {
			case "php_extensions_unavailable":
				resp.Error = fmt.Sprintf("%s cannot run on PHP %s here: %s has no installation candidate; choose another PHP version. Nothing was installed",
					svc.Name, phpVersion, strings.Join(refusal.Packages, ", "))
			case "php_version_unsupported":
				resp.Error = "choosing a PHP version for a tool is available on Debian and Ubuntu; this system runs one PHP. Nothing was installed"
			default:
				resp.Error = fmt.Sprintf("PHP %s is not installed; install PHP-FPM %s first, or choose another PHP version. Nothing was installed", phpVersion, phpVersion)
			}
		}
		if family != "apt" {
			refuse(&transport.InstallPreviewRefusal{Code: "php_version_unsupported"})
			return nil
		}
		if !phpVersionInstalled(phpVersion) {
			refuse(&transport.InstallPreviewRefusal{Code: "php_version_unavailable"})
			return nil
		}
		aptCache, err := executableForProfile(profile, family, "apt-cache")
		if err != nil {
			resp.Error = err.Error()
			return nil
		}
		extensions, refusal := phpToolVersionPackages(phpTool, phpVersion, family,
			func(name string) bool { return packageInstalledForProfile(profile, name) },
			func(name string) bool { return packageAvailableWithExecutable(name, aptCache) },
			nil)
		if refusal != nil {
			refuse(refusal)
			return nil
		}
		pkgs = append(pkgs, extensions...)
	}
	requiresRepo, err := core.InstallRequiresManagedRepository(svc, req.Package)
	if err != nil {
		resp.Error = fmt.Sprintf("repository policy failed: %v", err)
		return nil
	}
	if family == "apt" && requiresRepo {
		var repoStatus RepoStatusResponse
		if err := a.RepoStatus(&EnableRepoRequest{RepoID: svc.Repo.ID}, &repoStatus); err != nil {
			resp.Error = fmt.Sprintf("required repository status failed: %v", err)
			return nil
		}
		if repoStatus.Error != "" || !repoStatus.Enabled {
			if req.Package != "" {
				resp.Error = fmt.Sprintf("selected package %s requires the %s repository; enable it from Services first", req.Package, svc.Repo.Name)
			} else {
				resp.Error = fmt.Sprintf("%s requires the %s repository; enable it from Services first", svc.Name, svc.Repo.Name)
			}
			return nil
		}
	}

	missingPackages := make([]string, 0, len(pkgs))
	for _, pkg := range pkgs {
		if !packageInstalledForProfile(profile, pkg) {
			missingPackages = append(missingPackages, pkg)
		}
	}
	if len(missingPackages) > 0 {
		// alpha.105: what the simulation would install beyond what this
		// install names is judged right before the real install; a notable
		// package the owner did not confirm refuses it.
		var guard func(aptSimulation) error
		if req.CheckInstalls {
			classification := newInstallClassification(family, missingPackages, []string{svc.ID}, []string{phpVersion}, installed)
			if family == "apt" {
				classification.vendorOf = hostVendorVersionLookup()
			}
			guard = notableInstallGuard(classification, missingPackages, req.ConfirmedInstalls)
		}
		installMissingPackages := func() (string, error) {
			return installPackagesGuardedContext(ctx, family, missingPackages, strings.TrimSpace(req.Package), guard)
		}
		var installErr error
		if serviceUsesBINDPackageInstallGuard(req.ID) {
			_, installErr = installBINDPackagesWithGuard(ctx, systemctl, installMissingPackages)
		} else {
			_, installErr = installMissingPackages()
		}
		if installErr != nil {
			var notable *packageInstallNotableRefusal
			var vendor *packageInstallVendorRefusal
			if errors.As(installErr, &notable) {
				resp.ErrorCode = errCodeInstallNeedsConfirmation
				resp.Notable = append([]transport.NotableInstall{}, notable.Notable...)
			} else if errors.As(installErr, &vendor) {
				resp.ErrorCode = errCodeInstallVendorReplacement
				resp.Notable = append([]transport.NotableInstall{}, vendor.Notable...)
			}
			resp.Error = fmt.Sprintf("package install failed: %v", installErr)
			return nil
		}
	}
	// A tool installed for the first time for a chosen PHP version is bound
	// to it before its configuration is written (alpha.105).
	if firstPHPToolInstall(phpTool, toolBefore) {
		resp.FirstInstall = true
		if phpVersion != "" {
			if err := writePHPToolBinding(phpTool, phpVersion); err != nil {
				resp.Error = fmt.Sprintf("record %s PHP version: %v", svc.Name, err)
				return nil
			}
		}
	}
	if err := prepareInstalledServiceContext(ctx, req.ID, family); err != nil {
		resp.Error = fmt.Sprintf("service preparation failed: %v", err)
		return nil
	}

	// After a version pick, enable the unit that version actually brought —
	// firstPresentUnit returns the NEWEST present unit, which after installing
	// 8.3 next to an existing 8.4 would start 8.4 and leave the version the
	// operator just asked for stopped.
	// Sürüm seçiminden sonra, o sürümün gerçekten getirdiği unit etkinleştirilir
	// — firstPresentUnit mevcut EN YENİ unit'i döndürür; var olan 8.4'ün yanına
	// 8.3 kurulduktan sonra bu, 8.4'ü başlatıp operatörün az önce istediği
	// sürümü durmuş bırakırdı.
	unit := ""
	if exactUnit, exact := exactInstallUnit(req.ID, family, effectivePackage); exact {
		// A selected package has one exact service target. Never fall back to
		// another installed PHP major, postgresql.service or another PG
		// cluster: that would report success while starting a different
		// version than the operator selected.
		//
		// Seçilen paketin tek bir tam servis hedefi vardır. Başka kurulu PHP
		// major'una, postgresql.service'e veya başka PG cluster'ına asla düşme;
		// aksi hâlde operatörün seçtiğinden farklı sürüm başlatılıp başarı
		// bildirilirdi.
		if exactUnit == "" || !unitExistsWithExecutable(systemctl, exactUnit) {
			resp.Error = fmt.Sprintf("selected package %s did not provide its exact service unit", effectivePackage)
			return nil
		}
		unit = exactUnit
	} else {
		unit = a.firstPresentUnitWithExecutable(svc, systemctl)
	}
	if unit != "" {
		var err error
		if serviceUsesBINDPackageInstallGuard(req.ID) {
			// BIND's package-maintainer auto-start guard deliberately leaves both
			// distro unit names masked. The explicit panel configuration/switch
			// operation owns unmask + enable + start after publishing valid zones.
			err = nil
		} else if serviceStartsAfterPanelSetup(req.ID) {
			err = enableServiceForMutationWithExecutable(ctx, systemctl, unit, false)
		} else {
			err = enableServiceForMutationWithExecutable(ctx, systemctl, unit, true)
		}
		if err != nil {
			resp.Error = fmt.Sprintf("service did not become ready: %v", err)
			return nil
		}
		resp.Unit = unit
	}
	// Bridge daemons come in their own packages with their own units, and
	// nothing else starts them: SpamAssassin's spamd scores a message handed
	// to it, but spamass-milter is what hands Postfix's mail over. Leaving it
	// stopped produced "installed", "Running", zero mail filtered.
	// Köprü daemon'ları kendi paketlerinde, kendi unit'leriyle gelir ve onları
	// başka hiçbir şey başlatmaz: SpamAssassin'in spamd'si kendisine verilen
	// iletiyi puanlar ama Postfix'in postasını uzatan spamass-milter'dır. Onu
	// durmuş bırakmak "kurulu", "Çalışıyor", süzülen posta sıfır üretiyordu.
	for _, h := range svc.HelperUnits {
		if unitExistsWithExecutable(systemctl, h) {
			if err := enableServiceForMutationWithExecutable(ctx, systemctl, h, true); err != nil {
				resp.Error = fmt.Sprintf("helper service did not become ready: %v", err)
				return nil
			}
		}
	}

	resp.Installed = !alreadyPresent || len(missingPackages) > 0
	if resp.Installed {
		resp.Detail = fmt.Sprintf("installed %s", strings.Join(pkgs, ", "))
	} else {
		resp.Detail = fmt.Sprintf("verified and repaired %s", svc.Name)
	}
	if len(skipped) > 0 {
		resp.Detail += fmt.Sprintf(" — not offered for this version: %s", strings.Join(skipped, ", "))
	}
	return nil
}

func serviceStartsAfterPanelSetup(serviceID string) bool {
	switch serviceID {
	case "pdns", "bind", "postfix", "dovecot", "wireguard":
		return true
	default:
		return false
	}
}

func serviceUsesBINDPackageInstallGuard(serviceID string) bool {
	return serviceID == "bind"
}

// packageAvailable reports whether apt knows a package name at all, so an
// optional companion that a vendor does not publish for one version is skipped
// instead of failing the whole install.
// packageAvailable, apt'ın bir paket adını hiç tanıyıp tanımadığını bildirir;
// böylece vendor'ın bir sürüm için yayınlamadığı isteğe bağlı bir companion,
// tüm kurulumu düşürmek yerine atlanır.
func packageAvailable(name string) bool {
	profile, err := verifiedHostProfile("apt")
	if err != nil {
		return false
	}
	aptCache, err := executableForProfile(profile, "apt", "apt-cache")
	if err != nil {
		return false
	}
	return packageAvailableWithExecutable(name, aptCache)
}

func packageAvailableWithExecutable(name, aptCache string) bool {
	out, err := serviceMutationCommand(context.Background(), aptCache, "policy", name).Output()
	if err != nil {
		return false
	}
	// An unknown package prints nothing; a known one prints a stanza whose
	// Candidate is a real version rather than "(none)".
	// Bilinmeyen paket hiçbir şey basmaz; bilinen paket, Candidate'i "(none)"
	// değil gerçek bir sürüm olan bir kayıt basar.
	text := strings.TrimSpace(string(out))
	if text == "" {
		return false
	}
	for _, line := range strings.Split(text, "\n") {
		if s := strings.TrimSpace(line); strings.HasPrefix(s, "Candidate:") {
			return strings.TrimSpace(strings.TrimPrefix(s, "Candidate:")) != "(none)"
		}
	}
	return false
}

// unitCanonicalName asks systemd what a unit name really resolves to. For a
// plain unit the answer is itself; for an alias it is the target
// ("redis.service" -> "valkey.service"). Returns "" when systemd cannot say.
// unitCanonicalName, bir unit adının gerçekte neye çözüldüğünü systemd'ye
// sorar. Düz bir unit için cevap kendisidir; takma ad için hedeftir
// ("redis.service" -> "valkey.service"). systemd söyleyemezse "" döner.
func unitCanonicalName(unit string) string {
	profile, err := verifiedHostProfileForAnyFamily()
	if err != nil {
		return ""
	}
	systemctl, err := executableForProfile(profile, string(profile.PackageManager), "systemctl")
	if err != nil {
		return ""
	}
	return unitCanonicalNameWithExecutable(systemctl, unit)
}

func unitCanonicalNameWithExecutable(systemctl, unit string) string {
	out, err := serviceMutationCommand(context.Background(), systemctl, "show", unit+".service", "-p", "Id", "--value").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(strings.TrimSpace(string(out)), ".service")
}

// unitProvesInstalled decides whether the presence of `unit` really proves
// this component is installed, given what systemd says the name resolves to.
// Separated from the exec call so the RULE can be tested without systemd.
//
// - canonical == "" : systemd could not say; trust the name (a plain unit).
// - canonical == unit : a real unit of its own. Installed.
// - canonical is one of OUR names : still ours, just a second name for it.
// - otherwise : the name is an alias for somebody else's unit. Not ours.
//
// unitProvesInstalled, `unit`in varlığının bu bileşenin kurulu olduğunu
// gerçekten kanıtlayıp kanıtlamadığına, systemd'nin adı neye çözdüğüne bakarak
// karar verir. exec çağrısından ayrıdır ki KURAL systemd olmadan test
// edilebilsin.
func unitProvesInstalled(unit, canonical string, systemNames []string) bool {
	if canonical == "" || canonical == unit {
		return true
	}
	return containsString(systemNames, canonical)
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// unitExists reports whether systemd knows a unit by exactly this name.
// unitExists, systemd'nin tam bu adda bir unit tanıyıp tanımadığını bildirir.
func unitExists(name string) bool {
	profile, err := verifiedHostProfileForAnyFamily()
	if err != nil {
		return false
	}
	systemctl, err := executableForProfile(profile, string(profile.PackageManager), "systemctl")
	if err != nil {
		return false
	}
	return unitExistsWithExecutable(systemctl, name)
}

func unitExistsWithExecutable(systemctl, name string) bool {
	out, err := serviceMutationCommand(context.Background(), systemctl, "list-unit-files", name+".service", "--no-legend").Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

type UninstallServiceResponse = transport.UninstallServiceResponse

// serviceUninstallOps keeps privileged host operations behind one narrow
// boundary. Production supplies fixed argv commands; tests supply recorders so
// partial-mutation and package-selection contracts are exercised without
// touching the test host.
// serviceUninstallOps, yetkili makine işlemlerini tek ve dar bir sınırın
// arkasında tutar. Üretim sabit argv komutları, testler ise test makinesine
// dokunmadan kısmi değişiklik ve paket seçimi sözleşmelerini sınayan
// kaydediciler sağlar.
type serviceUninstallOps struct {
	detectPackageFamily func() string
	packageInstalled    func(string) bool
	unitExists          func(string) bool
	unitsMatching       func(string) []string
	disableUnit         func(string) error
	// unitState reads a unit's state as the owner left it, before the
	// removal stops it; restoreUnit puts back exactly that when the removal
	// is refused after the stop (alpha.104): a unit the owner had disabled
	// or stopped is not enabled or started.
	unitState   func(string) (unitRuntimeState, error)
	restoreUnit func(unitRuntimeState) error
	// removePackages plans the removal with the package manager's own
	// simulation, refuses collateral before anything changes, then runs
	// beforeRemoval (stopping units) and the real removal under one package
	// lock (alpha.104, guardedRemovePackagesContext).
	removePackages        func(string, packageRemovalRequest, removalHooks) (packageRemovalPlan, string, error)
	installedRepoPackages func(*core.ManagedService) ([]string, error)
	// boundPHPTools lists the tools (webmail, phpMyAdmin, phpPgAdmin) served
	// from a PHP version ("" = from any PHP-FPM socket).
	boundPHPTools func(string) []string
}

func defaultServiceUninstallOps(ctx context.Context) (serviceUninstallOps, error) {
	profile, err := verifiedHostProfileForAnyFamily()
	if err != nil {
		return serviceUninstallOps{}, err
	}
	family := string(profile.PackageManager)
	systemctl, err := executableForProfile(profile, family, "systemctl")
	if err != nil {
		return serviceUninstallOps{}, err
	}
	return serviceUninstallOps{
		detectPackageFamily: func() string { return family },
		packageInstalled:    func(pkg string) bool { return packageInstalledForProfile(profile, pkg) },
		unitExists:          func(unit string) bool { return unitExistsWithExecutable(systemctl, unit) },
		unitsMatching:       func(pattern string) []string { return unitsMatchingWithExecutable(systemctl, pattern) },
		disableUnit: func(unit string) error {
			_, err := runServiceMutationCombinedOutput(ctx, systemctl, "disable", "--now", unit)
			return err
		},
		unitState: func(unit string) (unitRuntimeState, error) {
			out, err := serviceMutationCommand(ctx, systemctl, "show", "--property=Id", "--property=UnitFileState", "--property=ActiveState", unit).Output()
			if err != nil {
				return unitRuntimeState{}, err
			}
			return parseUnitRuntimeState(unit, string(out))
		},
		restoreUnit: func(state unitRuntimeState) error {
			for _, args := range unitRestoreCommands(state) {
				out, err := runServiceMutationCombinedOutput(ctx, systemctl, args...)
				if err != nil {
					return errors.New(commandFailureDetail("systemctl "+strings.Join(args, " "), out, err))
				}
			}
			return nil
		},
		removePackages: func(family string, request packageRemovalRequest, hooks removalHooks) (packageRemovalPlan, string, error) {
			return guardedRemovePackagesContext(ctx, family, request, hooks)
		},
		installedRepoPackages: func(service *core.ManagedService) ([]string, error) {
			if family != "apt" {
				return nil, fmt.Errorf("installed repo package discovery requires apt")
			}
			dpkgQuery, err := executableForProfile(profile, family, "dpkg-query")
			if err != nil {
				return nil, err
			}
			return installedRepoPackagesForServiceWithExecutable(ctx, service, dpkgQuery)
		},
		boundPHPTools: phpToolsBoundToVersion,
	}, nil
}

// unitRuntimeState is a unit as the owner left it before a removal stopped
// it. ID is the unit's own name: an alias resolves to it (systemctl show
// reports the real unit's properties).
// Bir kaldırmanın durdurmasından önce sahibin bıraktığı biçimiyle unit.
type unitRuntimeState struct {
	ID      string
	Enabled bool
	Active  bool
}

func parseUnitRuntimeState(name, out string) (unitRuntimeState, error) {
	state := unitRuntimeState{}
	seen := 0
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "Id":
			state.ID = value
			seen++
		case "UnitFileState":
			state.Enabled = value == "enabled"
			seen++
		case "ActiveState":
			state.Active = value == "active" || value == "activating" || value == "reloading"
			seen++
		}
	}
	if seen != 3 || state.ID == "" {
		return unitRuntimeState{}, fmt.Errorf("systemctl show %s did not report its state", name)
	}
	return state, nil
}

// unitRestoreCommands puts back exactly the recorded state: enabled again
// only if it was enabled, started again only if it was running.
func unitRestoreCommands(state unitRuntimeState) [][]string {
	var commands [][]string
	if state.Enabled {
		commands = append(commands, []string{"enable", state.ID})
	}
	if state.Active {
		commands = append(commands, []string{"start", state.ID})
	}
	return commands
}

// failUninstallRemoval preserves the critical distinction between a refusal
// before mutation and a package purge that failed after units were stopped.
// failUninstallRemoval, değişiklik öncesi ret ile unit'ler durdurulduktan sonra
// başarısız olan paket kaldırmayı birbirinden ayıran kritik bilgiyi korur.
func failUninstallRemoval(resp *UninstallServiceResponse, err error, mutationApplied bool) {
	resp.Error = fmt.Sprintf("package removal failed: %v", err)
	resp.PartialSuccess = mutationApplied
	resp.MutationApplied = mutationApplied
}

// failUninstallDisable is deliberately fail-closed. `systemctl disable --now`
// may stop a unit before a later disable step fails, so every attempted
// stop/disable error means the host state may already have changed. Package
// removal must not continue from that uncertain state.
// failUninstallDisable bilerek güvenli biçimde kapalı davranır. `systemctl
// disable --now`, daha sonraki devre dışı bırakma adımı başarısız olmadan önce
// unit'i durdurmuş olabilir. Bu nedenle denenen her stop/disable hatası makine
// durumunun değişmiş olabileceği anlamına gelir; paket kaldırma sürdürülmez.
func failUninstallDisable(resp *UninstallServiceResponse, unit string, err error) {
	resp.Error = fmt.Sprintf(
		"failed to stop and disable %s; service state may have changed: %v",
		unit,
		err,
	)
	resp.PartialSuccess = true
	resp.MutationApplied = true
}

func uniquePackageNames(packages []string) []string {
	seen := make(map[string]struct{}, len(packages))
	out := make([]string, 0, len(packages))
	for _, raw := range packages {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

// UninstallService stops and disables a managed service, then purges its
// packages — the mirror of InstallService, for shrinking the attack surface.
// Every installed service is code that can be exploited; the operator must be
// able to take one back off, not only add it. Refuses services we never
// install (protects nothing) and reports a not-installed service honestly.
// UninstallService, yönetilen bir servisi durdurur, devre dışı bırakır ve
// paketlerini purge eder — InstallService'in aynası, saldırı yüzeyini
// küçültmek için. Kurulu her servis sömürülebilir koddur; operatör bir
// servisi yalnız ekleyebilmemeli, geri de alabilmeli.
func (a *Agent) UninstallService(req *InstallServiceRequest, resp *UninstallServiceResponse) error {
	*resp = UninstallServiceResponse{}
	if req == nil {
		resp.Error = "missing request"
		return nil
	}
	serviceID := req.ID
	packageName := req.Package
	svc := core.GetManagedServiceByID(serviceID)
	if svc == nil {
		resp.Error = "unknown service"
		return nil
	}
	if refusal := genericDNSEngineMutationRefusal(svc); refusal != "" {
		resp.Error = refusal
		return nil
	}
	if refusal := nativeCronRemovalRefusal(svc); refusal != "" {
		resp.Error = refusal
		return nil
	}
	// Reject an invalid catalogue/package selection before asking for a
	// privileged lease. This is pure input validation and mirrors InstallService.
	// Geçersiz katalog/paket seçimini ayrıcalıklı lease istemeden önce reddet.
	// Bu yalnızca girdi doğrulamasıdır ve InstallService akışını yansıtır.
	if packageName != "" {
		if _, err := validateRepoPackageSelection(svc, packageName); err != nil {
			resp.Error = err.Error()
			return nil
		}
	}
	authorizedReq := *req
	authorizedReq.ID = svc.ID
	authorizedReq.Package = packageName
	req = &authorizedReq
	ctx, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepUninstallService, svc.ID, packageName, "uninstall"),
	)
	if err != nil {
		*resp = UninstallServiceResponse{Error: err.Error()}
		return nil
	}
	defer finishStep()
	ops, err := defaultServiceUninstallOps(ctx)
	if err != nil {
		resp.Error = fmt.Sprintf("detect host platform: %v", err)
		return nil
	}
	return a.uninstallServiceWithOps(req, resp, ops)
}

// serviceRemovalWork is everything a removal will do, computed read-only
// before anything changes; the uninstall and the read-only preview share it.
// serviceRemovalWork, bir kaldırmanın yapacağı her şeydir; hiçbir şey
// değişmeden salt-okur hesaplanır. Kaldırma ve önizleme bunu paylaşır.
type serviceRemovalWork struct {
	family  string
	request packageRemovalRequest
	// units are stopped and disabled, in order, before the packages go.
	units []string
	// notInstalled: the selected version is not installed; nothing to do.
	notInstalled string
	// boundTools are PHP tools served from the PHP being removed.
	boundTools []string
}

// planServiceRemovalWork resolves the packages, units and safeguards of a
// removal without changing the host. A non-empty refusal is the answer.
func planServiceRemovalWork(svc *core.ManagedService, packageName string, ops serviceUninstallOps) (serviceRemovalWork, string) {
	// Apply the same catalogue boundary as install before any package-manager
	// detection or installed-package query.
	// Paket-yöneticisi algılama veya kurulu-paket sorgusundan önce kurulumla aynı
	// katalog sınırını uygula.
	var selectedPackageMatch []string
	if packageName != "" {
		var err error
		selectedPackageMatch, err = validateRepoPackageSelection(svc, packageName)
		if err != nil {
			return serviceRemovalWork{}, err.Error()
		}
	}

	family := ops.detectPackageFamily()
	work := serviceRemovalWork{family: family}

	// Version pick (B3d): remove ONE version of a runtime, the mirror of the
	// install pick. Same gates as install: the service must have a Repo, the
	// package must match its PackagePattern, apt only. Companions go with it
	// ({v}-cli, {v}-mysql… — orphaned config-only leftovers are what made
	// "did I really remove 8.3?" unanswerable on classic panels).
	// Sürüm seçimi (B3d): bir runtime'ın TEK sürümünü kaldır — kurulum
	// seçiminin aynası. Kapılar aynı: serviste Repo olmalı, paket
	// PackagePattern'e uymalı, yalnız apt. Yol arkadaşları da gider.
	if packageName != "" {
		if family != "apt" {
			return work, "per-version removal is only supported on apt-based systems"
		}
		if !ops.packageInstalled(packageName) {
			work.notInstalled = fmt.Sprintf("%s is not installed", packageName)
			return work, ""
		}
		versionPrefix := ""
		if len(selectedPackageMatch) > 1 {
			versionPrefix = selectedPackageMatch[1]
		}
		pkgs := []string{packageName}
		for _, tpl := range svc.Repo.VersionCompanions {
			name := strings.ReplaceAll(tpl, "{v}", versionPrefix)
			if ops.packageInstalled(name) {
				pkgs = append(pkgs, name)
			}
		}
		// Resolve the exact daemon target independently from the package name;
		// PostgreSQL's postgresql-17 package owns postgresql@17-main.
		// Tam daemon hedefini paket adından bağımsız çöz; PostgreSQL'in
		// postgresql-17 paketi postgresql@17-main unit'ine sahiptir.
		if exactUnit, exact := exactInstallUnit(svc.ID, family, packageName); exact {
			if exactUnit == "" {
				return work, "selected package has no valid exact service unit"
			}
			if ops.unitExists(exactUnit) {
				work.units = append(work.units, exactUnit)
			}
		}
		prefix := versionPrefix
		if prefix == "" {
			prefix = packageName
		}
		pkgs = uniquePackageNames(pkgs)
		work.request = packageRemovalRequest{
			Packages:           pkgs,
			VersionPrefixes:    []string{prefix},
			ProtectedComponent: catalogueProtectedComponent(family, packageNameSet(pkgs)),
		}
		if svc.ID == "php-fpm" && ops.boundPHPTools != nil {
			work.boundTools = ops.boundPHPTools(strings.TrimPrefix(versionPrefix, "php"))
		}
		return work, ""
	}

	pkgs := append([]string(nil), svc.Packages[family]...)
	if len(pkgs) == 0 {
		return work, fmt.Sprintf("%s cannot be removed automatically on this system yet", svc.Name)
	}

	// Discover catalogue-matching apt packages before stopping anything. The
	// query is package-manager truth, not a systemd-name guess: in particular,
	// postgresql@17-main must add postgresql-17, never the unit string itself.
	// Herhangi bir şeyi durdurmadan önce katalogla eşleşen apt paketlerini bul.
	// Sorgu systemd adı tahmini değil, paket yöneticisi gerçeğidir; özellikle
	// postgresql@17-main, unit dizgesini değil postgresql-17 paketini eklemelidir.
	var prefixes []string
	if core.UninstallPurgesAllVersions(svc, family) {
		repoPackages, err := ops.installedRepoPackages(svc)
		if err != nil {
			return work, fmt.Sprintf("installed package discovery failed: %v", err)
		}
		pkgs = append(pkgs, repoPackages...)
		re, err := regexp.Compile(svc.Repo.PackagePattern)
		if err != nil {
			return work, "invalid catalogue package pattern"
		}
		for _, pkg := range repoPackages {
			match := re.FindStringSubmatch(pkg)
			if match == nil {
				continue
			}
			if len(match) > 1 {
				prefixes = append(prefixes, match[1])
				for _, tpl := range svc.Repo.VersionCompanions {
					companion := strings.ReplaceAll(tpl, "{v}", match[1])
					if ops.packageInstalled(companion) {
						pkgs = append(pkgs, companion)
					}
				}
			} else {
				prefixes = append(prefixes, match[0])
			}
		}
	}
	pkgs = uniquePackageNames(pkgs)

	// Stop + disable every present unit first, so purge does not fight a
	// running process. Template instances ("wg-quick@wg0") are handled by
	// their exact SystemNames entry; pattern-matched units (php8.3-fpm from
	// Sury) are stopped AND their packages added to the purge — before B3d,
	// uninstalling php-fpm removed only the meta package and left every
	// versioned daemon installed, running and serving. Since alpha.104 the
	// units are stopped only after the package manager's simulation has
	// accepted the removal.
	// Önce mevcut her unit'i durdur + devre dışı bırak ki purge çalışan bir
	// süreçle boğuşmasın; alpha.104'ten beri yalnız benzetim kaldırmayı kabul
	// ettikten sonra.
	for _, unit := range append(append([]string{}, svc.SystemNames...), svc.HelperUnits...) {
		// Catalogue entries include distribution-specific alternatives (for
		// example redis-server/redis). Only an existing unit is an attempted
		// mutation; once attempted, any error is state-uncertain and aborts.
		// Katalog girdileri dağıtıma özgü alternatifler içerir (örneğin
		// redis-server/redis). Yalnız var olan bir unit için değişiklik denenir;
		// denendikten sonra her hata durumu belirsiz sayılır ve işlem kesilir.
		if ops.unitExists(unit) {
			work.units = append(work.units, unit)
		}
	}
	if svc.SystemNamePattern != "" {
		work.units = append(work.units, ops.unitsMatching(svc.SystemNamePattern)...)
	}
	work.request = packageRemovalRequest{
		Packages:           pkgs,
		VersionPrefixes:    prefixes,
		ProtectedComponent: catalogueProtectedComponent(family, packageNameSet(pkgs)),
	}
	if svc.ID == "php-fpm" && ops.boundPHPTools != nil {
		work.boundTools = ops.boundPHPTools("")
	}
	return work, ""
}

func packageNameSet(packages []string) map[string]bool {
	set := make(map[string]bool, len(packages))
	for _, p := range packages {
		set[p] = true
	}
	return set
}

func boundToolsRefusal(tools []string, version string) string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, phpToolDisplayName(tool))
	}
	if version == "" {
		return fmt.Sprintf("%s run on PHP-FPM; remove them first or keep PHP-FPM; nothing was changed", strings.Join(names, ", "))
	}
	return fmt.Sprintf("%s run on PHP %s; move them to another PHP version first or keep this version; nothing was changed", strings.Join(names, ", "), version)
}

func (a *Agent) uninstallServiceWithOps(
	req *InstallServiceRequest,
	resp *UninstallServiceResponse,
	ops serviceUninstallOps,
) error {
	if req == nil {
		resp.Error = "missing request"
		return nil
	}
	svc := core.GetManagedServiceByID(req.ID)
	if svc == nil {
		resp.Error = "unknown service"
		return nil
	}
	if refusal := genericDNSEngineMutationRefusal(svc); refusal != "" {
		resp.Error = refusal
		return nil
	}
	if refusal := nativeCronRemovalRefusal(svc); refusal != "" {
		resp.Error = refusal
		return nil
	}

	work, refusal := planServiceRemovalWork(svc, req.Package, ops)
	if refusal != "" {
		resp.Error = refusal
		return nil
	}
	if work.notInstalled != "" {
		resp.Removed = true
		resp.Detail = work.notInstalled
		return nil
	}
	// A PHP version webmail or a database tool runs on is not removed: they
	// would stop working and nothing would say so (alpha.104).
	// Web postası ya da bir veritabanı aracının koştuğu PHP sürümü kaldırılmaz.
	if len(work.boundTools) > 0 {
		version := ""
		if req.Package != "" {
			version = strings.TrimSuffix(strings.TrimPrefix(req.Package, "php"), "-fpm")
		}
		resp.Error = boundToolsRefusal(work.boundTools, version)
		resp.ErrorCode = errCodeRemovalToolsBound
		resp.BoundTools = work.boundTools
		return nil
	}

	// Each unit's state as the owner left it, read before anything stops,
	// so a refusal after the stop puts back exactly that (alpha.104).
	// Her unit'in sahibin bıraktığı durumu; ret sonrası tam o geri konur.
	states := map[string]unitRuntimeState{}
	if ops.unitState != nil {
		for _, unit := range work.units {
			state, err := ops.unitState(unit)
			if err != nil {
				resp.Error = fmt.Sprintf("the state of %s could not be read, so nothing was changed: %v", unit, err)
				return nil
			}
			states[unit] = state
		}
	}

	var disableErr error
	disableFailedUnit := ""
	var stopped []string
	plan, _, err := ops.removePackages(work.family, work.request, removalHooks{
		stop: func() error {
			for _, unit := range work.units {
				if err := ops.disableUnit(unit); err != nil {
					disableErr, disableFailedUnit = err, unit
					return err
				}
				stopped = append(stopped, unit)
			}
			return nil
		},
		restart: func() error {
			if ops.restoreUnit == nil {
				return nil
			}
			var failures []string
			for _, unit := range stopped {
				state, ok := states[unit]
				if !ok {
					failures = append(failures, unit+": its earlier state was not read")
					continue
				}
				if err := ops.restoreUnit(state); err != nil {
					failures = append(failures, err.Error())
				}
			}
			if len(failures) > 0 {
				return errors.New(strings.Join(failures, "; "))
			}
			return nil
		},
	})
	mutationApplied := len(stopped) > 0
	if disableErr != nil {
		failUninstallDisable(resp, disableFailedUnit, disableErr)
		return nil
	}
	if err != nil {
		var refusal *packageRemovalRefusal
		if errors.As(err, &refusal) {
			resp.Error = refusal.Message
			resp.ErrorCode = refusal.Code
			resp.Collateral = refusal.Collateral
			// Refused after the stop: the host changed only if the stopped
			// services could not be started again.
			changed := refusal.AfterStop && refusal.RestartFailed != ""
			resp.PartialSuccess = changed
			resp.MutationApplied = changed
			return nil
		}
		failUninstallRemoval(resp, err, mutationApplied)
		return nil
	}

	removed := plan.Remove
	if len(removed) == 0 {
		removed = work.request.Packages
	}
	resp.Removed = true
	resp.Detail = fmt.Sprintf("removed %s", strings.Join(removed, ", "))
	return nil
}

// serviceInstalled reports whether a catalogue service is present: by its
// systemd unit when it has one, by its package otherwise (daemonless tools
// like phpMyAdmin are files, not units).
// serviceInstalled, bir katalog servisinin var olup olmadığını bildirir:
// unit'i varsa unit'inden, yoksa paketinden (phpMyAdmin gibi daemon'suz
// araçlar unit değil dosyadır).
func (a *Agent) serviceInstalled(svc *core.ManagedService) bool {
	profile, err := detectHostPlatform()
	return err == nil && a.serviceInstalledWithProfile(svc, profile)
}

func (a *Agent) serviceInstalledWithProfile(svc *core.ManagedService, profile hostplatform.Profile) bool {
	// Roundcube is a tarball tree, not a unit or a package (D-004): presence
	// is the tree on disk, the same way Node's is. Without this it has no
	// SystemNames and no Packages, so the loops below both say "not
	// installed" even right after a successful install.
	// Roundcube bir tarball ağacıdır, unit ya da paket değil (D-004): varlık
	// diskteki ağaçtır, Node'unki gibi. Bu olmadan SystemNames'i de Packages'ı
	// da yoktur; aşağıdaki iki döngü de başarılı bir kurulumdan hemen sonra
	// bile "kurulu değil" der.
	if svc.ID == "roundcube" {
		return roundcubeInstalled()
	}
	if len(svc.SystemNames) > 0 {
		return a.firstPresentUnitWithProfile(svc, profile) != ""
	}
	for _, pkg := range svc.Packages[string(profile.PackageManager)] {
		if packageInstalledForProfile(profile, pkg) {
			return true
		}
	}
	return false
}

// installedServiceSet is the catalogue-wide installed map, for requirement
// and conflict decisions.
// installedServiceSet, gereksinim ve çakışma kararları için katalog çapında
// kurulu haritasıdır.
func (a *Agent) installedServiceSet() map[string]bool {
	profile, err := detectHostPlatform()
	if err != nil {
		return map[string]bool{}
	}
	return a.installedServiceSetWithProfile(profile)
}

func (a *Agent) installedServiceSetWithProfile(profile hostplatform.Profile) map[string]bool {
	set := map[string]bool{}
	for i := range core.ManagedServices {
		if a.serviceInstalledWithProfile(&core.ManagedServices[i], profile) {
			set[core.ManagedServices[i].ID] = true
		}
	}
	return set
}

// firstPresentUnit returns the first of a service's candidate systemd unit
// names that the system recognises, or "" if none is installed.
// firstPresentUnit, bir servisin aday systemd unit adlarından sistemin
// tanıdığı ilkini döndürür, hiçbiri kurulu değilse "".
// unitsMatching returns the installed unit names matching a catalogue pattern,
// newest version first. Listing the unit files and filtering here is what keeps
// versioned runtimes out of the catalogue's hardcoded name list — the panel
// learns which PHP versions exist from the machine, not from this file.
// unitsMatching, bir katalog desenine uyan kurulu unit adlarını en yeni sürüm
// başta döndürür. Unit dosyalarını listeleyip burada süzmek, sürümlü
// runtime'ları katalogdaki elle yazılmış ad listesinden kurtaran şeydir —
// panel hangi PHP sürümlerinin var olduğunu bu dosyadan değil makineden öğrenir.
func unitsMatching(pattern string) []string {
	profile, err := verifiedHostProfileForAnyFamily()
	if err != nil {
		return nil
	}
	systemctl, err := executableForProfile(profile, string(profile.PackageManager), "systemctl")
	if err != nil {
		return nil
	}
	return unitsMatchingWithExecutable(systemctl, pattern)
}

func unitsMatchingWithExecutable(systemctl, pattern string) []string {
	if pattern == "" {
		return nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	out, err := serviceMutationCommand(context.Background(), systemctl, "list-unit-files", "--type=service", "--no-legend", "--no-pager").Output()
	if err != nil {
		return nil
	}
	var units []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := strings.TrimSuffix(fields[0], ".service")
		if re.MatchString(name) {
			units = append(units, name)
		}
	}
	sort.Slice(units, func(i, j int) bool {
		ai, an := versionOf(units[i])
		bi, bn := versionOf(units[j])
		if ai != bi {
			return ai > bi
		}
		return an > bn
	})
	return units
}

func (a *Agent) firstPresentUnit(svc *core.ManagedService) string {
	profile, err := verifiedHostProfileForAnyFamily()
	if err != nil {
		return ""
	}
	return a.firstPresentUnitWithProfile(svc, profile)
}

func (a *Agent) firstPresentUnitWithProfile(svc *core.ManagedService, profile hostplatform.Profile) string {
	systemctl, err := executableForProfile(profile, string(profile.PackageManager), "systemctl")
	if err != nil {
		return ""
	}
	return a.firstPresentUnitWithExecutable(svc, systemctl)
}

func (a *Agent) firstPresentUnitWithExecutable(svc *core.ManagedService, systemctl string) string {
	// Pattern matches are consulted after the explicit names but count just as
	// much: with only php8.3-fpm installed there is no plain `php-fpm` unit, and
	// without this the runtime would read as not installed at all.
	// Desen eşleşmeleri açık adlardan sonra bakılır ama aynı ölçüde geçerlidir:
	// yalnız php8.3-fpm kuruluyken düz bir `php-fpm` unit'i yoktur ve bu olmadan
	// runtime hiç kurulu değilmiş gibi okunurdu.
	for _, unit := range svc.SystemNames {
		// Instances of template units ("wg-quick@wg0") never appear in
		// list-unit-files — look their template ("wg-quick@") up instead.
		// Şablon unit örnekleri ("wg-quick@wg0") list-unit-files'ta hiç
		// görünmez — onun yerine şablonunu ("wg-quick@") ara.
		lookup := unit
		if at := strings.IndexByte(unit, '@'); at >= 0 {
			lookup = unit[:at+1]
		}
		out, err := serviceMutationCommand(context.Background(), systemctl, "list-unit-files", lookup+".service", "--no-legend").Output()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			// A unit name that is an ALIAS of some other component's unit does
			// not prove this component is installed. Arch's valkey package
			// ships /usr/lib/systemd/system/redis.service as a symlink to
			// valkey.service (it declares Provides: redis), so installing
			// Valkey made the panel report "Redis: installed, dead" — a
			// component that was never installed, wearing someone else's name.
			// Ask systemd for the canonical name instead of trusting the link.
			// Başka bir bileşenin unit'ine TAKMA AD olan bir unit adı, bu
			// bileşenin kurulu olduğunu kanıtlamaz. Arch'ın valkey paketi
			// /usr/lib/systemd/system/redis.service'i valkey.service'e sembolik
			// bağ olarak koyar (Provides: redis der); bu yüzden Valkey kurmak
			// paneli "Redis: kurulu, ölü" demeye itiyordu — hiç kurulmamış bir
			// bileşen, başkasının adını taşıyarak. Bağa güvenmek yerine asıl
			// adı systemd'ye sor.
			if !unitProvesInstalled(unit, unitCanonicalNameWithExecutable(systemctl, unit), svc.SystemNames) {
				continue
			}
			return unit
		}
	}
	if units := unitsMatchingWithExecutable(systemctl, svc.SystemNamePattern); len(units) > 0 {
		return units[0] // newest version present
	}
	return ""
}

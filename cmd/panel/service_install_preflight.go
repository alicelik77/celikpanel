package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// preflightManagedServiceInstall is the backend gate shared by package,
// portable-tool and runtime installs. The browser may disable an Install
// button, but every API path must independently prove that this host supports
// the component, its prerequisites exist, its exclusive seat is free and a
// required repository is enabled before the agent mutates the machine.
// preflightManagedServiceInstall; paket, taşınabilir araç ve runtime
// kurulumlarının ortak backend kapısıdır. Tarayıcı düğmeyi kapatsa da her API
// yolu; agent makineyi değiştirmeden önce host desteğini, önkoşulları, özel
// servis yuvasının boş olduğunu ve zorunlu deponun etkinliğini bağımsız olarak
// doğrular.
func (p *Panel) preflightManagedServiceInstall(ctx context.Context, serviceID, selectedPackage string) error {
	managed := core.GetManagedServiceByID(serviceID)
	if managed == nil {
		return errors.New("unknown managed service")
	}
	if managedDNSEngineService(managed) {
		return errDNSEngineWorkflowRequired
	}

	host := p.managedServiceHostProfile()
	family := host.PackageFamily
	if _, reason := core.ManagedServiceInstallBlockForHost(managed, host); reason != "" {
		return fmt.Errorf("%s: %s", managed.Name, reason)
	}

	services, err := p.scanManagedServices(ctx)
	if err != nil {
		return fmt.Errorf("service install preflight scan: %w", err)
	}
	installed := make(map[string]bool, len(services))
	for _, service := range services {
		if service.Installed() {
			installed[service.ID] = true
		}
	}
	if missing := core.RequirementsMissing(managed, installed); len(missing) > 0 {
		return fmt.Errorf("%s requires %s; install the missing component first",
			managed.Name, strings.Join(missing, ", "))
	}
	if taken := core.SeatTakenBy(managed, installed); taken != "" {
		return fmt.Errorf("%s cannot be installed while %s occupies the same service role",
			managed.Name, taken)
	}

	// alpha.118 (lab): the distribution's own PHP version needs no vendor
	// repository (the Agent applies the same rule).
	if family == "apt" && managed.Repo != nil && !managed.Repo.Required &&
		core.DistributionShipsSelectedPackage(managed, host.DistroID, host.VersionID, selectedPackage) {
		return nil
	}
	return p.preflightManagedServiceRepository(ctx, managed, family, selectedPackage)
}

func (p *Panel) preflightManagedServiceRepository(
	ctx context.Context,
	managed *core.ManagedService,
	family string,
	selectedPackage string,
) error {
	requiresRepo, err := core.InstallRequiresManagedRepository(managed, selectedPackage)
	if err != nil {
		return fmt.Errorf("%s repository policy: %w", managed.Name, err)
	}
	// alpha.108: a PostgreSQL major the distribution itself ships installs by
	// its exact name without the vendor repository.
	if family == "apt" && requiresRepo && managed.ID == "postgresql" && strings.TrimSpace(selectedPackage) != "" &&
		!managed.Repo.Required && p.pgDistributionOffers(ctx, strings.TrimSpace(selectedPackage)) {
		requiresRepo = false
	}
	if family == "apt" && requiresRepo {
		var status transport.RepoStatusResponse
		if err := p.callAgentContext(ctx, "Agent.RepoStatus", &transport.EnableRepoRequest{RepoID: managed.Repo.ID}, &status); err != nil {
			log.Printf("[repo][%s][install-preflight][transport] %v", managed.Repo.ID, err)
			return errors.New("required repository status could not be verified")
		}
		if status.Error != "" {
			code := normalizeRepoErrorCode(status.ErrorCode, errCodeRepoStatusFailed)
			if status.Repairable {
				code = normalizeRepoErrorCode(status.ErrorCode, errCodeRepoConfigurationInvalid)
			}
			log.Printf("[repo][%s][install-preflight][agent][%s] %s", managed.Repo.ID, code, status.Error)
			if status.Repairable {
				return errors.New("required repository configuration needs repair")
			}
			return errors.New("required repository status could not be verified")
		}
		if !status.Enabled {
			if selectedPackage = strings.TrimSpace(selectedPackage); selectedPackage != "" {
				return fmt.Errorf("selected package %s requires the %s repository; enable it from Components first",
					selectedPackage, managed.Repo.Name)
			}
			return fmt.Errorf("%s requires the %s repository; enable it from Components first",
				managed.Name, managed.Repo.Name)
		}
	}
	return nil
}

// serviceInstallPreflightBusyAttempts bounds how often the install preflight
// is asked again while the Panel's own database answers busy (2+4+8+16 s, then
// 20 s each: about two minutes).
var serviceInstallPreflightBusyAttempts = 9

// serviceInstallPreflightPause waits between those attempts; tests replace it.
var serviceInstallPreflightPause = func(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	return nil
}

// preflightWhilePanelBusy runs an install preflight and asks it again while
// the Panel's own database answers busy (alpha.123 lab, Ubuntu 24.04: setup
// step 02 failed "service install preflight scan: database is locked" while
// another panel writer held the lock). The preflight's scan records what it
// read in the Panel's database; nothing has reached the Agent yet, so asking
// again changes nothing on the server. Any other answer, or a database that
// stays busy past the bound, is returned as before.
// Panelin kendi veritabanı meşgulse kurulum ön denetimi yeniden sorulur;
// sunucuda henüz hiçbir şey değişmemiştir.
func preflightWhilePanelBusy(ctx context.Context, preflight func() error) error {
	for attempt := 1; ; attempt++ {
		err := preflight()
		if err == nil || !panelDatabaseBusy(err) || attempt >= serviceInstallPreflightBusyAttempts {
			return err
		}
		log.Printf("service install preflight: the panel database was busy; asked again (%d): %v", attempt, err)
		if waitErr := serviceInstallPreflightPause(ctx, serverSetupPanelBusyDelay(attempt)); waitErr != nil {
			return err
		}
	}
}

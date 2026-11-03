package main

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The usage worker (alpha.118, D-044): every few minutes it collects website
// traffic, applies the traffic limits, measures the disks that are due,
// applies the disk rules, keeps Dovecot's mailbox quotas enforced (hourly)
// and hands pending notices to the mail system. One goroutine, so two
// measurements never compete; polling a screen never starts any of it.
//
// Kullanım işçisi: trafiği toplar, sınırları uygular, sırası gelen diskleri
// ölçer, posta kotasını saatte bir denetler ve bildirimleri postalar.

const (
	usageWorkerTick       = 5 * time.Minute
	mailQuotaInterval     = time.Hour
	usageWorkerRoundLimit = 20 * time.Minute
)

type usageWorkerState struct {
	once sync.Once
	kick chan struct{}
	mu   sync.Mutex

	stateMu      sync.Mutex
	low          bool
	lastQuota    time.Time
	quotaResult  transport.MailQuotaReconcileResponse
	quotaError   string
	quotaChecked time.Time
}

func (s *usageWorkerState) lowSpace() bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.low
}

func (s *usageWorkerState) setLowSpace(low bool) {
	s.stateMu.Lock()
	s.low = low
	s.stateMu.Unlock()
}

// mailQuotaStatus is the last mailbox quota reconciliation, for the screens.
func (s *usageWorkerState) mailQuotaStatus() (transport.MailQuotaReconcileResponse, string, time.Time) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.quotaResult, s.quotaError, s.quotaChecked
}

func (p *Panel) kickUsageWorker() {
	if p.usageWorker.kick == nil {
		return
	}
	select {
	case p.usageWorker.kick <- struct{}{}:
	default:
	}
}

// reconcileMailQuota asks the Agent to keep Dovecot's quota enforced.
func (p *Panel) reconcileMailQuota(ctx context.Context) {
	var resp transport.MailQuotaReconcileResponse
	var err error
	if p.mailQuota != nil {
		resp, err = p.mailQuota(ctx)
	} else {
		req := transport.MailQuotaReconcileRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit)}
		err = p.callAgentContext(ctx, "Agent.ReconcileMailQuotaV1", &req, &resp)
	}
	s := &p.usageWorker
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.quotaChecked = usageNow()
	if err != nil {
		s.quotaError = boundedAgentDiagnostic(err.Error())
		return
	}
	s.quotaError = ""
	s.quotaResult = resp
	if resp.Error != "" || (resp.State != transport.MailQuotaEnforced && resp.State != transport.MailQuotaNoMail) {
		log.Printf("[mail quota] %s: %s%s", resp.State, resp.Detail, resp.Error)
	}
}

// runUsageRound is one round of the worker.
func (p *Panel) runUsageRound(ctx context.Context) {
	p.usageWorker.mu.Lock()
	defer p.usageWorker.mu.Unlock()
	if err := p.collectTraffic(ctx); err != nil {
		log.Printf("[traffic] %v", err)
	}
	if err := p.evaluateTrafficLimits(ctx); err != nil {
		log.Printf("[traffic] limits: %v", err)
	}
	if _, err := p.measureDueDisks(ctx); err != nil {
		log.Printf("[disk] %v", err)
	}
	if err := p.evaluateDiskLimits(ctx); err != nil {
		log.Printf("[disk] limits: %v", err)
	}
	if err := p.applyUploadBlocks(ctx); err != nil {
		log.Printf("[disk] uploads: %v", err)
	}
	s := &p.usageWorker
	s.stateMu.Lock()
	due := s.lastQuota.IsZero() || usageNow().Sub(s.lastQuota) >= mailQuotaInterval
	if due {
		s.lastQuota = usageNow()
	}
	s.stateMu.Unlock()
	if due {
		p.reconcileMailQuota(ctx)
	}
	if err := p.sendPendingUsageNotices(ctx); err != nil {
		log.Printf("[usage] notices: %v", err)
	}
}

// startUsageWorker runs a round at start, every few minutes and on a kick.
func (p *Panel) startUsageWorker() {
	p.usageWorker.once.Do(func() {
		p.usageWorker.kick = make(chan struct{}, 1)
		go func() {
			ticker := time.NewTicker(usageWorkerTick)
			defer ticker.Stop()
			for {
				ctx, cancel := context.WithTimeout(context.Background(), usageWorkerRoundLimit)
				p.runUsageRound(ctx)
				cancel()
				select {
				case <-ticker.C:
				case <-p.usageWorker.kick:
				}
			}
		}()
	})
}

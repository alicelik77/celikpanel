package main

import (
	"context"

	"github.com/alicelik/celikpanel/internal/licensing"
)

// The count check's own licence refusal (D-047, design §b, second point).
// Every creation of a domain, a database or a mailbox already passes
// checkSubscriptionQuota (D-044). While the licence does not allow full use
// that check refuses those kinds too, independently of the route table: a
// creation reached through a route nobody classified, or classified wrongly,
// is still refused. The disk kind is never affected, so backups and restores
// keep working. It reads the receipt on disk only (Status), never the
// licence service.
//
// Sayım denetiminin kendi lisans reddi: lisans tam kullanıma izin vermezken
// alan adı, veritabanı ve posta kutusu oluşturma rota tablosundan bağımsız
// olarak burada da reddedilir. Disk türü etkilenmez.

// licenceGrowthRefusal carries the licence status and whom the sentence
// speaks to; writeQuotaRefusal answers it as the route gate would.
type licenceGrowthRefusal struct {
	status   licensing.Status
	audience string
}

func (e *licenceGrowthRefusal) Error() string {
	_, _, message := licenceRefusal(e.status, e.audience)
	return message
}

// countCheckLicenceStatus is the licence status the count check reads. A
// Panel without a licence manager cannot run (main.go stops at start); it is
// read as unknown, exactly as the route gate reads it. Tests that build a
// Panel without a manager and call creation handlers directly replace it in
// TestMain.
var countCheckLicenceStatus = func(p *Panel) licensing.Status {
	if p.license == nil {
		return licensing.Status{State: "status_unavailable", Observation: licensing.ObservationUnavailable}
	}
	return p.license.Status()
}

// licenceGrowthCheck: nil when the licence allows full use.
func (p *Panel) licenceGrowthCheck(ctx context.Context) error {
	status := countCheckLicenceStatus(p)
	if status.CanProvision {
		return nil
	}
	caller, _ := ctx.Value(callerKey).(*Caller)
	return &licenceGrowthRefusal{status: status, audience: licenceAudienceOf(caller)}
}

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Finishing a deletion whose DNS removal is confirmed, when it keeps failing
// (alpha.129 follow-up).
//
// After the DNS removal is confirmed, the rest of a deletion on this server
// (its managed certificates, then its row) is finished by the deletion
// reconciler (reconcileLocalDNSDeletionsOnce). alpha.128 retried a failing
// finish every 5 minutes or sooner for as long as the panel ran, and kept the
// failure in memory only. Now each failure is kept with the deletion marker
// (a versioned panel_settings record; no schema change) with the stage that
// failed, its bounded first error line and the next attempt:
//
//	failure 1: next attempt after 5 minutes; 2: 15 minutes; 3: 60 minutes;
//	from 4 on: every 6 hours.
//
// From the third failure the deletion is "clearly failing": its status says
// deletion_finish_failed with the stage and the error line, who acts and
// "Retry this deletion" (the panel user and a token's status_url read the
// same answer), and the domain's owner, its reseller and the administrators
// get one notice for that run of failures, not one per attempt. A deletion
// started again (the panel's "Retry this deletion" or a token's DELETE)
// clears the record, so its backoff starts again. A status read only reads
// the record (D-024).
//
// DNS kaldırması doğrulanmış bir silmenin geri kalanı sürekli başarısız
// olursa: 5 dk, 15 dk, 60 dk, sonra 6 saatte bir denenir; üçüncü başarısızlıktan
// sonra durum ve tek bir bildirim nedeni ve "Bu silmeyi yeniden dene" eylemini
// söyler. Elle yeniden deneme sayacı sıfırlar.

const (
	domainDeletionFinishKeyPrefix = "domain_deletion_finish_v1:"
	domainDeletionFinishVersion   = 1
	// domainDeletionFinishClearlyFailing: from this failure on the status and
	// the one notice name the failure and the person's retry.
	domainDeletionFinishClearlyFailing = 3

	domainDeletionFinishFailedReason = "deletion_finish_failed"
	noticeDomainDeletionStuck        = "domain_deletion_stuck"
)

// domainDeletionFinishNow is replaced by tests.
var domainDeletionFinishNow = func() time.Time { return time.Now().UTC() }

// domainDeletionFinishDelay is the pause after the given number of failures.
func domainDeletionFinishDelay(failures int) time.Duration {
	switch {
	case failures <= 1:
		return 5 * time.Minute
	case failures == 2:
		return 15 * time.Minute
	case failures == 3:
		return time.Hour
	}
	return 6 * time.Hour
}

type domainDeletionFinishRecord struct {
	Version      int    `json:"version"`
	DomainID     int    `json:"domain_id"`
	Domain       string `json:"domain"`
	Failures     int    `json:"failures"`
	FirstAt      string `json:"first_at"`
	LastAt       string `json:"last_at"`
	NextAt       string `json:"next_at"`
	Stage        string `json:"stage"`
	ErrorLine    string `json:"error_line"`
	NoticeRaised bool   `json:"notice_raised"`
}

func domainDeletionFinishKey(domainID int) string {
	return domainDeletionFinishKeyPrefix + strconv.Itoa(domainID)
}

// domainDeletionFinishStageError names the stage after DNS that failed.
type domainDeletionFinishStageError struct {
	Stage string
	Err   error
}

func (e *domainDeletionFinishStageError) Error() string { return e.Stage + ": " + e.Err.Error() }
func (e *domainDeletionFinishStageError) Unwrap() error { return e.Err }

func domainDeletionFinishStage(err error) string {
	var staged *domainDeletionFinishStageError
	if errors.As(err, &staged) {
		return staged.Stage
	}
	return "certificate_cleanup"
}

// readDomainDeletionFinishRecord is read only. A missing, malformed or
// foreign record is absent.
func readDomainDeletionFinishRecord(ctx context.Context, q rowQueryer, domainID int, domain string) (domainDeletionFinishRecord, bool) {
	var raw string
	if err := q.QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`, domainDeletionFinishKey(domainID)).Scan(&raw); err != nil {
		return domainDeletionFinishRecord{}, false
	}
	var record domainDeletionFinishRecord
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil || record.Version != domainDeletionFinishVersion || record.DomainID != domainID ||
		record.Domain != strings.ToLower(strings.TrimSpace(domain)) || record.Failures < 1 ||
		(record.Stage != "certificate_cleanup" && record.Stage != "ledger_delete") {
		return domainDeletionFinishRecord{}, false
	}
	return record, true
}

// domainDeletionFinishDue: no failure kept, or its next attempt is due.
func domainDeletionFinishDue(record domainDeletionFinishRecord, present bool, now time.Time) bool {
	if !present {
		return true
	}
	next, err := time.Parse(time.RFC3339, record.NextAt)
	return err != nil || !now.Before(next)
}

// clearDomainDeletionFinish ends a run of failures: the deletion finished,
// or a person started it again (its backoff starts again).
func (p *Panel) clearDomainDeletionFinish(ctx context.Context, domainID int) {
	if _, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key = ?`, domainDeletionFinishKey(domainID)); err != nil {
		log.Printf("domain deletion %d: clear its finishing record: %v", domainID, err)
	}
}

// recordDomainDeletionFinishFailure keeps one more failure of finishing the
// deletion and schedules the next attempt; at the third failure of a run it
// raises the one notice. Stored only while the deletion marker exists.
func (p *Panel) recordDomainDeletionFinishFailure(ctx context.Context, domainID int, domain string, failure error) domainDeletionFinishRecord {
	db := p.db.GetDB()
	now := domainDeletionFinishNow()
	record, present := readDomainDeletionFinishRecord(ctx, db, domainID, domain)
	if !present {
		record = domainDeletionFinishRecord{
			Version: domainDeletionFinishVersion, DomainID: domainID,
			Domain: strings.ToLower(strings.TrimSpace(domain)), FirstAt: now.Format(time.RFC3339),
		}
	}
	record.Failures++
	record.LastAt = now.Format(time.RFC3339)
	record.NextAt = now.Add(domainDeletionFinishDelay(record.Failures)).Format(time.RFC3339)
	record.Stage = domainDeletionFinishStage(failure)
	record.ErrorLine = boundedFinishErrorLine(failure)
	raiseNotice := record.Failures >= domainDeletionFinishClearlyFailing && !record.NoticeRaised
	if raiseNotice {
		record.NoticeRaised = true
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return record
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO panel_settings (key, value, updated_at)
		SELECT ?, ?, CURRENT_TIMESTAMP
		WHERE EXISTS (SELECT 1 FROM domain_deletion_operations WHERE domain_id = ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		domainDeletionFinishKey(domainID), string(raw), domainID); err != nil {
		log.Printf("deletion of %s: keep its finishing failure: %v", domain, err)
		return record
	}
	if raiseNotice {
		p.raiseDomainDeletionStuckNotice(ctx, domainID, record)
	}
	return record
}

func boundedFinishErrorLine(err error) string {
	text := ""
	if err != nil {
		text = err.Error()
		var staged *domainDeletionFinishStageError
		if errors.As(err, &staged) && staged.Err != nil {
			text = staged.Err.Error()
		}
	}
	line := boundedDeletionErrorLine(text)
	if strings.TrimSpace(text) == "" || utf8.RuneCountInString(line) == 0 {
		return "the server reported no further detail"
	}
	return line
}

// raiseDomainDeletionStuckNotice tells the domain's owner, its reseller and
// the administrators once per run of failures (period: the run's first
// failure). The owner is told whom to ask; the administrator acts.
func (p *Panel) raiseDomainDeletionStuckNotice(ctx context.Context, domainID int, record domainDeletionFinishRecord) {
	db := p.db.GetDB()
	var subscriptionID int
	if err := db.QueryRowContext(ctx, `SELECT subscription_id FROM domains WHERE id = ?`, domainID).Scan(&subscriptionID); err != nil {
		log.Printf("deletion of %s: notice recipients: %v", record.Domain, err)
		return
	}
	recipients, err := usageRecipientsFor(ctx, db, subscriptionID)
	if err != nil {
		log.Printf("deletion of %s: notice recipients: %v", record.Domain, err)
		return
	}
	var people, admins []usageRecipient
	for _, r := range recipients {
		if r.Audience == audienceAdministrator {
			admins = append(admins, r)
		} else {
			people = append(people, r)
		}
	}
	body := map[string]any{"domain": record.Domain, "stage": record.Stage, "error_line": record.ErrorLine}
	subject := "domain_deletion:" + strconv.Itoa(domainID)
	p.raiseAccountNotice(ctx, db, people, noticeDomainDeletionStuck, false, subject, record.FirstAt, body, "/domains")
	p.raiseAccountNotice(ctx, db, admins, noticeDomainDeletionStuck, true, subject, record.FirstAt, body, "/domains")
}

func init() {
	registerAccountNoticeKind(noticeDomainDeletionStuck, accountNoticeKind{
		Group: notifyGroupAccount,
		Words: domainDeletionStuckNoticeWords,
	})
}

func domainDeletionStuckNoticeWords(audience string, body map[string]any) accountNoticeWords {
	domain := bodyString(body, "domain")
	stepEN, stepTR := "removing its certificates", "sertifikalarını kaldırma"
	if bodyString(body, "stage") == "ledger_delete" {
		stepEN, stepTR = "removing it from the panel's list", "panelin listesinden kaldırma"
	}
	w := accountNoticeWords{
		TitleEN: "The deletion of " + domain + " cannot finish",
		TitleTR: domain + " silmesi bitemiyor",
	}
	if audience == audienceAdministrator {
		w.TextEN = fmt.Sprintf("The domain %s is removed from DNS, but %s failed three times in a row, so its deletion is not finished. Its website, mail and DNS are already gone. CelikPanel now tries again only every 6 hours. Who acts: you. Open Domains, read the error shown for %s, fix its cause on the server, then use “Retry this deletion”.", domain, stepEN, domain)
		w.TextTR = fmt.Sprintf("%s alan adı DNS'ten kaldırıldı, ancak %s adımı üst üste üç kez başarısız oldu; bu yüzden silme bitmedi. Web sitesi, postası ve DNS kayıtları zaten kaldırıldı. CelikPanel artık yalnız 6 saatte bir yeniden dener. Kim yapar: siz. Alan adları sayfasında %s için gösterilen hatayı okuyun, nedenini sunucuda giderin, sonra “Bu silme işlemini tekrar dene” düğmesini kullanın.", domain, stepTR, domain)
		return w
	}
	w.TextEN = fmt.Sprintf("The domain %s is removed from DNS and its website and mail are gone, but the last step of its deletion keeps failing on the server, so it is still listed. Nothing of it is served any more. Who acts: your provider (the administrator of this server) was told and fixes it; you do not need to do anything.", domain)
	w.TextTR = fmt.Sprintf("%s alan adı DNS'ten kaldırıldı, web sitesi ve postası da kaldırıldı; ancak silmenin son adımı sunucuda başarısız olmayı sürdürüyor, bu yüzden hâlâ listede görünüyor. Artık hiçbir şeyi sunulmuyor. Kim yapar: sağlayıcınız (bu sunucunun yöneticisi) bilgilendirildi ve sorunu giderir; sizin bir şey yapmanız gerekmiyor.", domain)
	return w
}

// domainDeletionFinishStatus is the status of a deletion whose DNS removal
// is confirmed, from the kept finishing record. Read only.
func domainDeletionFinishStatus(record domainDeletionFinishRecord, present bool) domainDeletionStatusResponse {
	if !present {
		return domainDeletionStatusResponse{
			Status: domainDeletionPendingStatus, Stage: domainDeletionDNSStageDone,
			Reason: domainDeletionDNSRemovalConfirmedReason, Message: domainDeletionDNSRemovedEnglish,
		}
	}
	next := record.NextAt
	if at, err := time.Parse(time.RFC3339, next); err == nil {
		next = at.UTC().Format("2006-01-02 15:04") + " UTC"
	}
	if record.Failures < domainDeletionFinishClearlyFailing {
		return domainDeletionStatusResponse{
			Status: domainDeletionPendingStatus, Stage: domainDeletionDNSStageDone,
			Reason:  domainDeletionDNSRemovalConfirmedReason,
			Message: domainDeletionDNSFinishRetryEnglish + " Next attempt: " + next + ".",
		}
	}
	return domainDeletionStatusResponse{
		Status: domainDeletionPendingStatus, Stage: record.Stage,
		Reason: domainDeletionFinishFailedReason, ErrorLine: record.ErrorLine,
		Message: fmt.Sprintf("This domain is removed from DNS, but finishing its deletion (%s) failed %d times in a row; error_line is the server's first line. CelikPanel tries again by itself only every 6 hours (next: %s). The server administrator fixes the cause on the server, then uses “Retry this deletion” (a token sends the same DELETE again); a retry starts the attempts afresh.",
			strings.ReplaceAll(record.Stage, "_", " "), record.Failures, next),
	}
}

var _ rowQueryer = (*sql.DB)(nil)

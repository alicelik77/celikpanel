package membership

// The receipt mail (S2): sent once when an order becomes paid, to the member's verified
// address only, through the existing mail code. It names what was paid, when, for which
// licence and term, says it is not a tax invoice, and carries no secret and no provider
// identifier beyond our order number. There is no refund mail: there are no refunds.

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// VerifiedEmail is a member's address if it is verified ("" otherwise).
func (s *Service) VerifiedEmail(member int64) (string, error) {
	var email string
	err := s.db.QueryRowContext(context.Background(), "SELECT email FROM members WHERE id=? AND verified=1", member).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return email, err
}

// ReceiptMail is the subject and body of an order's receipt (bilingual, ASCII like the
// service's other mails).
func ReceiptMail(o Order, paidAt int64) (string, string) {
	what := "First year / Ilk yil"
	if o.Kind == KindRenewal {
		what = "Renewal / Yenileme"
	}
	var b strings.Builder
	b.WriteString("CelikPanel\n\nPayment received / Odeme alindi\n\n")
	b.WriteString("Order / Siparis: " + o.ID + "\n")
	b.WriteString("Amount / Tutar: " + money(o.AmountMinor, o.Currency) + "\n")
	b.WriteString("License / Lisans: ..." + o.LicenseID[len(o.LicenseID)-8:] + "\n")
	b.WriteString("For / Ne icin: " + what + ", " + day(o.GrantedFrom) + " - " + day(o.GrantedTo) + " (UTC)\n")
	b.WriteString("Paid on / Odeme tarihi (UTC): " + day(paidAt) + "\n\n")
	if !o.Live {
		b.WriteString("Test mode: no real money was taken. / Deneme kipi: gercek para alinmadi.\n")
	}
	b.WriteString("This receipt is not a tax invoice. / Bu makbuz bir vergi faturasi degildir.\n")
	return "CelikPanel receipt / Makbuz " + o.ID, b.String()
}

// sendReceipt mails the receipt of an order that has just become paid. A mail that cannot
// be sent changes nothing on the order; the receipt stays on the member's page.
func (s *Service) sendReceipt(o Order, send func(to, subject, body string) error) error {
	if o.State != OrderPaid || send == nil {
		return nil
	}
	email, err := s.VerifiedEmail(o.Member)
	if err != nil || email == "" {
		return err
	}
	paidAt, err := s.PaidAt(o.ID)
	if err != nil {
		return err
	}
	subject, body := ReceiptMail(o, paidAt)
	return send(email, subject, body)
}

// mail is the service's way to send a store mail: the test folder in the loopback
// development configuration, otherwise the configured transport (as the token mails).
func (h *Handler) mail(config Config) func(to, subject, body string) error {
	if h.sendMail != nil {
		return h.sendMail
	}
	return func(to, subject, body string) error {
		if config.SMTPPresent && config.SMTP == nil {
			return ErrMailUnavailable
		}
		mailer := Mailer{Sender: config.Sender, SMTP: config.SMTP}
		if h.Mailer != nil {
			mailer = h.Mailer(config)
		}
		return mailer.Send(to, subject, body)
	}
}

// ConfigMailer is the transport a command uses for store mails (reconciliation's receipts).
func ConfigMailer(config Config) func(to, subject, body string) error {
	return (&Handler{}).mail(config)
}

// SendReceipt mails the receipt of a paid order (a command after reconciliation).
func (s *Service) SendReceipt(o Order, send func(to, subject, body string) error) error {
	return s.sendReceipt(o, send)
}

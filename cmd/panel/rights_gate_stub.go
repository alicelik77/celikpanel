package main

import "context"

// subscriptionHasRight reports whether a subscription's rights set grants a
// right ("file_access", "ssh", ...). Temporary: replaced by rights_gate.go at
// integration (docs/PLANS-AND-RIGHTS.md, the rights set). Until then every
// right is granted, which is today's behavior (the Unlimited plan).
func (p *Panel) subscriptionHasRight(ctx context.Context, subscriptionID int64, right string) (bool, error) {
	_ = ctx
	_ = subscriptionID
	_ = right
	return true, nil
}

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// alpha.147 review, finding 1: a tool's move whose step context ends half-way
// (the Agent's lease ended: the Panel stalled or restarted, or its renewals
// went unanswered) must leave the tool working. The fakes model the server:
// a command run with an ended context does not run (exec.CommandContext),
// nginx's loaded configuration changes only on a reload that runs, and the
// tool's own pool stops only when PHP-FPM reloads after its file is removed.
// For every point at which the step's context can end, the socket the running
// nginx hands PHP to must have a running pool: the stock socket (always
// running here) or the own socket with the own pool running. Fails on
// 2321e9cd7: a cut after nginx loaded the new file left nginx on the own
// socket while the undo removed the own pool (502), and the message said
// nginx rejected the configuration.
func TestAMoveCutAtAnyPointLeavesNginxOnARunningPool(t *testing.T) {
	const stock = "/run/php/php8.4-fpm.sock"
	own := phpToolPoolSocket(phpToolPHPPgAdmin, "8.4")
	for point := 1; point <= 8; point++ {
		t.Run(fmt.Sprintf("cut after call %d", point), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			call := func(name string) {
				calls++
				if calls == point {
					cancel()
				}
			}
			bind := newFakeBindHost()
			bind.packages["php8.4-pgsql"], bind.packages["php8.4-mbstring"], bind.packages["php8.4-xml"] = true, true, true
			const path = "/etc/nginx/conf.d/celikpanel-dbtools.conf"
			bind.conf[path] = []byte("old:" + stock)
			loaded := "old:" + stock
			ownRunning := false
			ops := bind.ops()
			ops.cliProbe = func(ctx context.Context, _, _ string) error {
				defer call("cli")
				return ctx.Err()
			}
			ops.ownPool = func(ctx context.Context, _, _, _ string) (string, func(context.Context) error, error) {
				defer call("own")
				if err := ctx.Err(); err != nil {
					return "", nil, err
				}
				ownRunning = true
				return own, func(ctx context.Context) error {
					// The pool file goes; PHP-FPM stops serving it only on a
					// reload, which needs a live context.
					if err := ctx.Err(); err != nil {
						return err
					}
					ownRunning = false
					return nil
				}, nil
			}
			ops.render = func(_, socket string) (string, []byte, error) {
				defer call("render")
				return path, []byte("new:" + socket), nil
			}
			ops.nginxTest = func(ctx context.Context) error {
				defer call("nginx-t")
				return ctx.Err()
			}
			ops.nginxReload = func(ctx context.Context) error {
				defer call("reload")
				if err := ctx.Err(); err != nil {
					return err
				}
				loaded = string(bind.conf[path])
				return nil
			}
			ops.httpProbe = func(ctx context.Context, _, _ string) error {
				defer call("probe")
				if err := ctx.Err(); err != nil {
					return err
				}
				if strings.HasSuffix(loaded, own) && !ownRunning {
					return errors.New("502")
				}
				return nil
			}
			ops.writeRecord = func(string, string) error { return nil }
			_, err := bindPHPToolWith(ctx, ops, phpToolPHPPgAdmin, "8.4")
			socket := strings.SplitN(loaded, ":", 2)[1]
			switch {
			case socket == stock:
			case socket == own && ownRunning:
			default:
				t.Fatalf("nginx hands PHP to %s, own pool running %v (err %v)", socket, ownRunning, err)
			}
			if err != nil && ctx.Err() != nil && strings.Contains(err.Error(), "nginx rejected") {
				t.Fatalf("a cut move blames nginx: %v", err)
			}
			if err != nil && ctx.Err() != nil && calls >= 5 && !strings.Contains(err.Error(), "interrupted") {
				t.Fatalf("a cut move does not say it was interrupted: %v", err)
			}
		})
	}
}

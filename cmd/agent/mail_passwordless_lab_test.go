//go:build linux

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Lab-only: runs the real passwordless-mail steps against the real Postfix of
// a throwaway VM (the phase 2 lab check, D-046). Skipped unless
// CELIKPANEL_LAB_PASSWORDLESS names the step; never set it on a server that
// matters. Steps: "report" (read only), "close" (the WireMailFilters step),
// "send" (the panel's account mail through Agent.SendAccountMailV1, to
// CELIKPANEL_LAB_TO).
//
// Yalnız laboratuvar: gerçek Postfix'e karşı; ortam değişkeni yoksa atlanır.
func TestLabPasswordlessMail(t *testing.T) {
	step := os.Getenv("CELIKPANEL_LAB_PASSWORDLESS")
	if step == "" {
		t.Skip("lab only: set CELIKPANEL_LAB_PASSWORDLESS=report|close|send on a throwaway VM")
	}
	run := func(name string, args ...string) ([]byte, error) { return exec.Command(name, args...).CombinedOutput() }
	switch step {
	case "report":
		var resp transport.PasswordlessMailReportResponse
		if err := (&Agent{}).PasswordlessMailReportV1(&transport.Empty{}, &resp); err != nil {
			t.Fatal(err)
		}
		out, _ := json.MarshalIndent(resp, "", "  ")
		fmt.Println(string(out))
	case "close":
		note, err := reconcilePasswordlessMail(run, func() error {
			_, err := applyPostfixVerified(func(name string, args ...string) ([]byte, error) {
				return exec.Command(name, args...).CombinedOutput()
			}, mailServiceReload)
			return err
		}, time.Now())
		fmt.Printf("note: %s\nerror: %v\n", note, err)
		out, _ := json.MarshalIndent(passwordlessEnforcement(run), "", "  ")
		fmt.Println(string(out))
	case "send":
		message := []byte("From: no-reply@panel.test\r\nTo: " + os.Getenv("CELIKPANEL_LAB_TO") +
			"\r\nSubject: CelikPanel account mail (lab)\r\nAuto-Submitted: auto-generated\r\n\r\nlab account mail\r\n")
		var resp transport.SendAccountMailResponse
		_ = (&Agent{}).SendAccountMailV1(&transport.SendAccountMailRequest{
			From: "no-reply@panel.test", To: os.Getenv("CELIKPANEL_LAB_TO"), Message: message}, &resp)
		fmt.Printf("sent=%v error=%q\n", resp.Sent, resp.Error)
	default:
		t.Fatalf("unknown step %q", step)
	}
}

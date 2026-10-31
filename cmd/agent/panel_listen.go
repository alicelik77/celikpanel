package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/alicelik/celikpanel/internal/panellisten"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The panel's listen address, read from the same root-only panel.env systemd
// gives the panel (single source of truth, alpha.114). The Agent dials the
// panel there to verify its served certificate, and changes that one line
// only after the owner confirmed signing in on a new port.
//
// Panelin dinleme adresi, systemd'nin panele verdiği panel.env'den okunur
// (tek doğruluk kaynağı). Agent sertifika doğrulamasında panele oradan
// bağlanır; satırı yalnız sahip yeni portta oturum açtığını onaylayınca değiştirir.

const panelListenEnvMaxSize = 16 << 10

var (
	panelListenEnvPath = panellisten.EnvFile
	// panelListenUnitPath is the vendor unit; its Environment= line is the
	// fallback when panel.env does not exist (a pre-panel.env install).
	panelListenUnitPath = "/etc/systemd/system/celikpanel-panel.service"
	// panelListenRequireRoot is replaced in tests that run unprivileged.
	panelListenRequireRoot = requireRootOwner
)

func readPanelListenEnv() ([]byte, os.FileInfo, error) {
	file, err := os.Open(panelListenEnvPath)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, errors.New("panel.env is not a regular file")
	}
	raw, err := io.ReadAll(io.LimitReader(file, panelListenEnvMaxSize+1))
	if err != nil {
		return nil, nil, err
	}
	if len(raw) > panelListenEnvMaxSize {
		return nil, nil, errors.New("panel.env is too large")
	}
	return raw, info, nil
}

func panelListenSHA256(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// configuredPanelListen is the address the panel is configured to listen on:
// panel.env, else the vendor unit's Environment= line, else the default.
func configuredPanelListen() string {
	if raw, _, err := readPanelListenEnv(); err == nil {
		if listen, found, err := panellisten.ListenFromEnv(raw); err == nil && found {
			if _, err := panellisten.Port(listen); err == nil {
				return listen
			}
		}
	}
	if raw, err := os.ReadFile(panelListenUnitPath); err == nil && len(raw) <= panelListenEnvMaxSize {
		if listen, ok := panelListenFromUnit(raw); ok {
			return listen
		}
	}
	return panellisten.DefaultListen
}

func panelListenFromUnit(raw []byte) (string, bool) {
	const prefix = "Environment=" + panellisten.EnvKey + "="
	value, found := "", false
	for _, line := range splitPanelListenLines(raw) {
		if len(line) > len(prefix) && line[:len(prefix)] == prefix {
			value, found = line[len(prefix):], true
		}
	}
	if !found {
		return "", false
	}
	if _, err := panellisten.Port(value); err != nil {
		return "", false
	}
	return value, true
}

func splitPanelListenLines(raw []byte) []string {
	var lines []string
	start := 0
	for i, b := range raw {
		if b == '\n' {
			lines = append(lines, string(raw[start:i]))
			start = i + 1
		}
	}
	if start < len(raw) {
		lines = append(lines, string(raw[start:]))
	}
	return lines
}

// panelCertificateLoopbackAddress is where the Agent reaches the panel's own
// TLS listener on this server: the configured listen address, never a
// constant (alpha.114: new installs use 1923, older ones keep 2083).
func panelCertificateLoopbackAddress() string {
	address, err := panellisten.ProbeAddress(configuredPanelListen())
	if err != nil {
		address, _ = panellisten.ProbeAddress(panellisten.DefaultListen)
	}
	return address
}

// PanelListenConfigV1 reports panel.env's listen address and digest.
func (a *Agent) PanelListenConfigV1(_ *transport.Empty, resp *transport.PanelListenConfigResponse) error {
	*resp = readPanelListenConfig()
	return nil
}

func readPanelListenConfig() transport.PanelListenConfigResponse {
	raw, info, err := readPanelListenEnv()
	if errors.Is(err, os.ErrNotExist) {
		return transport.PanelListenConfigResponse{}
	}
	if err != nil {
		return transport.PanelListenConfigResponse{Present: true, Error: "panel.env could not be read: " + boundedText(err.Error(), 160)}
	}
	response := transport.PanelListenConfigResponse{Present: true, SHA256: panelListenSHA256(raw)}
	if err := panelListenRequireRoot(info, "panel.env"); err != nil || info.Mode().Perm() != 0o600 {
		response.Error = "panel.env must belong to root with mode 0600"
		return response
	}
	listen, found, err := panellisten.ListenFromEnv(raw)
	switch {
	case err != nil:
		response.Error = "panel.env could not be read: " + boundedText(err.Error(), 160)
	case !found:
		response.Error = "panel.env has no " + panellisten.EnvKey + " line"
	default:
		response.Listen = listen
	}
	return response
}

// SetPanelListenPortV1 changes panel.env's listen line in its own
// panel_port_change job, only over the file the Panel saw. Every other line
// is kept; the file is replaced atomically, root-only, so systemd and
// install/update read either the old or the new file, never a partial one.
// The running panel is not restarted: it already listens on the new port.
func (a *Agent) SetPanelListenPortV1(req *transport.PanelListenSetRequest, resp *transport.PanelListenSetResponse) error {
	*resp = transport.PanelListenSetResponse{}
	if req == nil || len(req.ExpectedSHA256) != 64 {
		resp.ErrorCode, resp.Error = "INVALID_REQUEST", "the current and new listen addresses and the file digest are required"
		return nil
	}
	port, err := panellisten.Port(req.To)
	if err != nil {
		resp.ErrorCode, resp.Error = "INVALID_REQUEST", "the new listen address has no valid port"
		return nil
	}
	if code, reason := panellisten.ChoiceRefusal(port, nil); code != "" {
		resp.ErrorCode, resp.Error = code, reason
		return nil
	}
	_, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepSetPanelListen, panelListenMutationTarget, fmt.Sprint(port), "port"),
	)
	if err != nil {
		resp.ErrorCode, resp.Error = "MUTATION_REFUSED", err.Error()
		return nil
	}
	defer finishStep()
	setPanelListen(req, resp)
	return nil
}

const panelListenMutationTarget = "panel-listen"

func setPanelListen(req *transport.PanelListenSetRequest, resp *transport.PanelListenSetResponse) {
	raw, info, err := readPanelListenEnv()
	if err != nil {
		resp.ErrorCode, resp.Error = "CONFIG_CHANGED", "panel.env could not be read, so nothing was written"
		return
	}
	if err := panelListenRequireRoot(info, "panel.env"); err != nil || info.Mode().Perm() != 0o600 {
		resp.ErrorCode, resp.Error = "CONFIG_UNSAFE", "panel.env must belong to root with mode 0600; nothing was written"
		return
	}
	current := panelListenSHA256(raw)
	if listen, found, _ := panellisten.ListenFromEnv(raw); found && listen == req.To {
		// Already written (a repeated request after an unknown answer).
		resp.Changed, resp.SHA256 = true, current
		return
	}
	if current != req.ExpectedSHA256 {
		resp.ErrorCode, resp.Error = "CONFIG_CHANGED", "panel.env changed after the port change started, so nothing was written"
		return
	}
	updated, err := panellisten.ReplaceListen(raw, req.From, req.To)
	if err != nil {
		resp.ErrorCode, resp.Error = "CONFIG_CHANGED", "panel.env does not name the current port ("+boundedText(err.Error(), 120)+"), so nothing was written"
		return
	}
	if err := writePanelListenEnv(updated); err != nil {
		resp.ErrorCode, resp.Error = "WRITE_FAILED", "the new panel.env could not be saved ("+boundedText(err.Error(), 160)+"); the previous file is kept"
		return
	}
	resp.Changed, resp.SHA256 = true, panelListenSHA256(updated)
}

func writePanelListenEnv(content []byte) error {
	dir := filepath.Dir(panelListenEnvPath)
	temp, err := os.CreateTemp(dir, ".panel.env.port-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(name)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if os.Geteuid() == 0 {
		if err := temp.Chown(0, 0); err != nil {
			temp.Close()
			return err
		}
	}
	if _, err := temp.Write(content); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, panelListenEnvPath); err != nil {
		return err
	}
	keep = true
	if directory, err := os.Open(dir); err == nil {
		_ = directory.Sync()
		directory.Close()
	}
	return nil
}

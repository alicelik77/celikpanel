package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Arch Linux ships one PHP (extra/php) with most extensions built as shared
// modules that php.ini leaves commented out: mysqli, which phpMyAdmin needs,
// is one of them, and iconv (phpMyAdmin's charset conversion) is another.
// mbstring and the XML/DOM family are built in. CelikPanel enables what a
// tool needs with one drop-in per tool in /etc/php/conf.d, never by editing
// the owner's php.ini, and only for an extension PHP does not load already
// (loading a module twice is a PHP startup warning on every request).
//
// Arch tek PHP getirir; mysqli gibi uzantılar php.ini'de yorum satırıdır.
// CelikPanel gerekeni araç başına /etc/php/conf.d içinde tek ek dosyayla
// açar; sahibin php.ini'sine dokunmaz ve zaten yüklü uzantıyı yeniden açmaz.

var pacmanPHPToolExtensionModules = map[string][]string{
	phpToolPHPMyAdmin: {"mysqli", "iconv"},
}

var pacmanPHPConfDir = "/etc/php/conf.d"

type pacmanPHPToolOps struct {
	loaded func(ctx context.Context, ext string) (bool, error)
	read   func(path string) ([]byte, bool, error)
	write  func(path string, content []byte) error
	reload func(ctx context.Context) error
}

func pacmanPHPToolDropInPath(tool string) string {
	return filepath.Join(pacmanPHPConfDir, "celikpanel-"+tool+".ini")
}

func renderPacmanPHPToolDropIn(tool string, modules []string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "; Managed by CelikPanel for %s. CelikPanel rewrites this file only while it holds exactly this text.\n", phpToolDisplayName(tool))
	for _, module := range modules {
		b.WriteString("extension=" + module + "\n")
	}
	return []byte(b.String())
}

func ensurePacmanPHPToolExtensions(ctx context.Context, tool string) error {
	return ensurePacmanPHPToolExtensionsWithOps(ctx, tool, pacmanPHPToolOps{
		loaded: func(ctx context.Context, ext string) (bool, error) {
			out, err := serviceMutationCommand(ctx, "php", "-r",
				"echo extension_loaded('"+ext+"')?'yes':'no';").Output()
			if err != nil {
				return false, fmt.Errorf("ask PHP whether %s is loaded: %w", ext, err)
			}
			return strings.TrimSpace(string(out)) == "yes", nil
		},
		read: func(path string) ([]byte, bool, error) {
			exists, err := secureMailFileExists(path)
			if err != nil || !exists {
				return nil, false, err
			}
			content, err := secureReadConfig(path)
			return content, true, err
		},
		write: func(path string, content []byte) error {
			info, err := os.Lstat(filepath.Dir(path))
			if err != nil || !info.IsDir() {
				return fmt.Errorf("PHP configuration directory %s is missing", filepath.Dir(path))
			}
			return secureWriteConfig(path, content, 0o644)
		},
		reload: func(ctx context.Context) error {
			out, err := runServiceMutationCombinedOutput(ctx, "systemctl", "reload-or-restart", "php-fpm")
			if err != nil {
				return errors.New(commandFailureDetail("PHP-FPM reload", out, err))
			}
			return nil
		},
	})
}

func ensurePacmanPHPToolExtensionsWithOps(ctx context.Context, tool string, ops pacmanPHPToolOps) error {
	modules := pacmanPHPToolExtensionModules[tool]
	if len(modules) == 0 {
		return nil
	}
	path := pacmanPHPToolDropInPath(tool)
	_, exists, err := ops.read(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	var missing []string
	for _, module := range modules {
		loaded, err := ops.loaded(ctx, module)
		if err != nil {
			return err
		}
		if !loaded {
			missing = append(missing, module)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if exists {
		// Our file is already there and PHP still lacks a module: the owner
		// changed it, or PHP cannot load the module. Never rewrite it blindly.
		return fmt.Errorf("%s needs the PHP extensions %s, which PHP does not load although %s exists; the server administrator checks that file and /etc/php/php.ini, then configures %s again",
			phpToolDisplayName(tool), strings.Join(missing, ", "), path, phpToolDisplayName(tool))
	}
	if err := ops.write(path, renderPacmanPHPToolDropIn(tool, missing)); err != nil {
		return fmt.Errorf("enable the PHP extensions %s needs (%s): %w", phpToolDisplayName(tool), strings.Join(missing, ", "), err)
	}
	if err := ops.reload(ctx); err != nil {
		return err
	}
	for _, module := range missing {
		loaded, err := ops.loaded(ctx, module)
		if err != nil {
			return err
		}
		if !loaded {
			return fmt.Errorf("PHP does not load %s after %s enabled it in %s", module, phpToolDisplayName(tool), path)
		}
	}
	return nil
}

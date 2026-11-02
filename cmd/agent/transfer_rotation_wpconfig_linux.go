//go:build linux

package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"golang.org/x/sys/unix"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/transport"
)

// wp-config.php lives in a directory the site user can write. The Agent
// (root) opens the root-owned sites folder, resolves the document root
// beneath it without following any symbolic link, and works on that
// directory's descriptor: the file must be a regular file of the site user
// with one link. A new file is written beside it and renamed over it, so a
// crash leaves the old or the new file, never half of one.

const wordpressConfigMaxBytes = 256 << 10

// Seams for unprivileged tests.
var (
	wordpressConfigRoot      = hostingpath.SubscriptionsRoot
	wordpressConfigRootOwner = uint32(0)
	wordpressConfigOwnerUID  = func(username string) (int, error) {
		account, err := user.Lookup(username)
		if err != nil {
			return 0, err
		}
		uid, err := strconv.Atoi(account.Uid)
		if err != nil || uid < 1000 {
			return 0, errors.New("the site user is not a hosting account")
		}
		return uid, nil
	}
)

type wordpressConfigFile struct {
	dirFD   int
	content []byte
	stat    unix.Stat_t
}

func (f *wordpressConfigFile) close() {
	if f.dirFD >= 0 {
		_ = unix.Close(f.dirFD)
		f.dirFD = -1
	}
}

// openWordPressConfigForRotation returns the file when it names dbName and
// dbUser, else a reason it is not the file to update.
func openWordPressConfigForRotation(app transport.DatabaseUserApp, dbUser string) (*wordpressConfigFile, string) {
	uid, err := wordpressConfigOwnerUID(app.SiteUsername)
	if err != nil {
		return nil, "the site user " + app.SiteUsername + " was not found"
	}
	sites := filepath.Join(wordpressConfigRoot(), strconv.Itoa(app.SubscriptionID), "sites")
	sitesFD, err := unix.Open(sites, unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, "the site folder could not be opened"
	}
	defer unix.Close(sitesFD)
	var sitesStat unix.Stat_t
	if err := unix.Fstat(sitesFD, &sitesStat); err != nil || sitesStat.Uid != wordpressConfigRootOwner || sitesStat.Mode&0o022 != 0 {
		return nil, "the subscription's sites folder is not root-owned and protected"
	}
	dirFD, err := unix.Openat2(sitesFD, filepath.Join(strconv.Itoa(app.DomainID), "public_html"), &unix.OpenHow{
		Flags:   uint64(unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return nil, "the site's document root could not be opened"
	}
	keep := false
	defer func() {
		if !keep {
			_ = unix.Close(dirFD)
		}
	}()
	fd, err := unix.Openat(dirFD, "wp-config.php", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, "wp-config.php is not in the site's document root"
	}
	file := os.NewFile(uintptr(fd), "wp-config.php")
	defer file.Close()
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return nil, "wp-config.php could not be read"
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || int(st.Uid) != uid || st.Size > wordpressConfigMaxBytes {
		return nil, "wp-config.php is not a plain file of the site user"
	}
	content, err := io.ReadAll(io.LimitReader(file, wordpressConfigMaxBytes+1))
	if err != nil || len(content) > wordpressConfigMaxBytes {
		return nil, "wp-config.php could not be read"
	}
	if reason := wordpressConfigMatches(content, app.DBName, dbUser); reason != "" {
		return nil, reason
	}
	keep = true
	return &wordpressConfigFile{dirFD: dirFD, content: content, stat: st}, ""
}

// writePassword replaces DB_PASSWORD's value: a new file with the old one's
// owner and mode, written and synced, then renamed over the old one.
func (f *wordpressConfigFile) writePassword(password string) error {
	next, err := replaceWordPressDBPassword(f.content, password)
	if err != nil {
		return err
	}
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		return err
	}
	tmp := ".wp-config.php.celikpanel-" + hex.EncodeToString(suffix)
	fd, err := unix.Openat(f.dirFD, tmp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return fmt.Errorf("create the new file: %w", err)
	}
	file := os.NewFile(uintptr(fd), tmp)
	cleanup := func(cause error) error {
		_ = file.Close()
		_ = unix.Unlinkat(f.dirFD, tmp, 0)
		return cause
	}
	if err := unix.Fchown(fd, int(f.stat.Uid), int(f.stat.Gid)); err != nil {
		return cleanup(fmt.Errorf("set the owner: %w", err))
	}
	if err := unix.Fchmod(fd, f.stat.Mode&0o7777); err != nil {
		return cleanup(fmt.Errorf("set the mode: %w", err))
	}
	if _, err := file.Write(next); err != nil {
		return cleanup(fmt.Errorf("write: %w", err))
	}
	if err := file.Sync(); err != nil {
		return cleanup(fmt.Errorf("sync: %w", err))
	}
	if err := file.Close(); err != nil {
		_ = unix.Unlinkat(f.dirFD, tmp, 0)
		return fmt.Errorf("close: %w", err)
	}
	if err := unix.Renameat(f.dirFD, tmp, f.dirFD, "wp-config.php"); err != nil {
		_ = unix.Unlinkat(f.dirFD, tmp, 0)
		return fmt.Errorf("replace: %w", err)
	}
	if dir, err := unix.Openat(f.dirFD, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0); err == nil {
		_ = unix.Fsync(dir)
		_ = unix.Close(dir)
	}
	f.content = next
	return nil
}

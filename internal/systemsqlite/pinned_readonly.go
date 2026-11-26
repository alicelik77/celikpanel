package systemsqlite

import (
	"errors"
	"fmt"
	"os"
)

// PinnedReadOnly is a database file another account owns, opened for reading
// through the managed-file checks (alpha.149, review of PR #370 finding 2):
// an absolute path with no symbolic link in it, a regular file with one link
// (never a FIFO, a device or a hard link to another file), pinned by a
// descriptor opened without following links, owned by one of the expected
// accounts; its SQLite companions (-wal, -shm, -journal), when present, are
// held to the same. SQLite then opens the file by its own name - a WAL
// database's companions are found next to that name, so the descriptor's
// /proc path would read a stale copy - and Verify, after the read, proves
// the name still holds the pinned file.
//
// Başka hesabın veritabanı dosyası, yönetilen dosya denetimleriyle salt okunur
// açılır: bağlantı yok, düz dosya, beklenen sahip; okumadan sonra kimliği
// yeniden doğrulanır.
type PinnedReadOnly struct {
	source *managedSource
}

// OpenPinnedReadOnly pins path for reading; owners are the accounts that may
// own it and its companions (none given: any owner).
func OpenPinnedReadOnly(path string, owners ...uint32) (*PinnedReadOnly, error) {
	source, err := openManagedSource(path, false)
	if err != nil {
		return nil, err
	}
	if err := ownedByOneOf(source.info, owners); err != nil {
		source.close()
		return nil, err
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		companion := source.path + suffix
		info, err := os.Lstat(companion)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			source.close()
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			source.close()
			return nil, fmt.Errorf("the database's %s file is not a regular file", suffix)
		}
		if err := ownedByOneOf(info, owners); err != nil {
			source.close()
			return nil, err
		}
	}
	return &PinnedReadOnly{source: source}, nil
}

// Path is the file's own (clean, absolute) name, for SQLite to open.
func (p *PinnedReadOnly) Path() string {
	if p == nil || p.source == nil {
		return ""
	}
	return p.source.path
}

// Verify proves the name still holds the pinned file (no swap since the pin).
func (p *PinnedReadOnly) Verify() error {
	if p == nil {
		return errors.New("managed database is not pinned")
	}
	return p.source.verifyIdentity()
}

// Close releases the pin.
func (p *PinnedReadOnly) Close() {
	if p != nil {
		p.source.close()
	}
}

func ownedByOneOf(info os.FileInfo, owners []uint32) error {
	if len(owners) == 0 {
		return nil
	}
	uid, ok := fileOwnerUID(info)
	if !ok {
		return errors.New("the database file's owner could not be read")
	}
	for _, owner := range owners {
		if uid == owner {
			return nil
		}
	}
	return fmt.Errorf("the database file is owned by an unexpected account (uid %d)", uid)
}

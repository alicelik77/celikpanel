package siteruntime

import (
	"bytes"
	"encoding/binary"
	"errors"
	"path"
	"regexp"
	"strings"
)

// The entry is the program systemd starts: a file under <app>/bin, named by
// the customer relative to that folder ("server", "release/api"). The name is
// checked here; the file itself is checked by the Agent without following any
// link (openat2 RESOLVE_BENEATH | RESOLVE_NO_SYMLINKS): a regular file, owned
// by the site's account, executable by it, not setuid or setgid, not writable
// by others, and either an ELF program for this server's processor or a "#!"
// script whose interpreter is inside the app folder. A script for /bin/sh or
// python3 is refused before it is started: the sandbox lets an uploaded
// program execute only from its own folder and the shared libraries, so such
// a script could never run.

// Entry check codes (the screen words them: runtime.reason.<code>).
const (
	EntryOK             = "ok"
	EntryInvalid        = "entry_invalid"
	EntryMissing        = "entry_missing"
	EntryNotRegular     = "entry_not_regular"
	EntryLink           = "entry_link"
	EntryNotOwned       = "entry_not_owned"
	EntryNotExecutable  = "entry_not_executable"
	EntrySetID          = "entry_setid"
	EntryWritable       = "entry_writable"
	EntryUnknownFormat  = "entry_unknown_format"
	EntryWrongArch      = "entry_wrong_arch"
	EntryInterpreter    = "entry_interpreter"
	EntryFolderNotOwned = "app_folder_not_owned"
	// EntryUnreadable: the entry could not be read (an unknown result, never a
	// verdict on the file).
	EntryUnreadable = "entry_unreadable"
)

var entryPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,63}(/[A-Za-z0-9_][A-Za-z0-9._-]{0,63}){0,3}$`)

// ValidEntry checks the customer's entry name and returns the absolute path
// of the entry under the layout's bin folder.
func ValidEntry(layout Layout, entry string) (string, error) {
	if !entryPattern.MatchString(entry) {
		return "", errors.New("the entry is a file name under app/bin: letters, digits, '.', '_' and '-', up to four folders deep")
	}
	for _, part := range strings.Split(entry, "/") {
		if part == "." || part == ".." || strings.HasPrefix(part, "..") {
			return "", errors.New("the entry may not name a parent folder")
		}
	}
	full := path.Join(layout.BinDir(), entry)
	if !strings.HasPrefix(full, layout.BinDir()+"/") {
		return "", errors.New("the entry is outside app/bin")
	}
	return full, nil
}

// EntryRelative is the entry's path relative to the app folder ("bin/<entry>"),
// the name the Agent opens beneath the folder.
func EntryRelative(entry string) string { return "bin/" + entry }

// ELF machine numbers this release recognises.
const (
	elfMachineX86_64  = 62
	elfMachineAArch64 = 183
)

// MachineFor is the ELF machine of a Go GOARCH ("amd64", "arm64"), 0 when
// this release does not know it.
func MachineFor(goarch string) uint16 {
	switch goarch {
	case "amd64":
		return elfMachineX86_64
	case "arm64":
		return elfMachineAArch64
	}
	return 0
}

// FileFacts is what the Agent read of the entry without following a link.
type FileFacts struct {
	Regular    bool
	Dir        bool // a real folder (the Python kind's app/src)
	Link       bool
	UID        int
	Mode       uint32 // permission and set-id bits
	Header     []byte // the first bytes (up to 256)
	HostArch   uint16 // the server's ELF machine (MachineFor)
	FolderUID  int    // owner of the app folder
	FolderLink bool   // the app folder itself is a link
}

// CheckEntry decides the entry check from what was read. uid is the site
// account's. appRoot is the layout root (an interpreter of a "#!" script must
// be inside it).
func CheckEntry(f FileFacts, uid int, appRoot string) (string, string) {
	switch {
	case f.FolderLink || f.FolderUID != uid:
		return EntryFolderNotOwned, "the app folder must be a real folder of the site's own account"
	case f.Link:
		return EntryLink, "the entry is a link; upload the program itself"
	case !f.Regular:
		return EntryNotRegular, "the entry is not a regular file"
	case f.UID != uid:
		return EntryNotOwned, "the entry belongs to another account; upload it again with the site's file manager or SFTP"
	case f.Mode&0o6000 != 0:
		return EntrySetID, "the entry has the setuid or setgid bit; remove it (chmod u-s,g-s)"
	case f.Mode&0o002 != 0:
		return EntryWritable, "anyone on the server could change the entry; remove the others' write permission (chmod o-w)"
	case f.Mode&0o100 == 0:
		return EntryNotExecutable, "the entry is not executable; set it executable (chmod u+x) in the file manager or over SFTP"
	}
	h := f.Header
	switch {
	case len(h) >= 20 && bytes.Equal(h[:4], []byte{0x7f, 'E', 'L', 'F'}):
		var machine uint16
		switch h[5] {
		case 1:
			machine = binary.LittleEndian.Uint16(h[18:20])
		case 2:
			machine = binary.BigEndian.Uint16(h[18:20])
		}
		if f.HostArch != 0 && machine != f.HostArch {
			return EntryWrongArch, "the program was built for another processor than this server's"
		}
		return EntryOK, "elf"
	case len(h) >= 2 && h[0] == '#' && h[1] == '!':
		line, _, _ := bytes.Cut(h[2:], []byte("\n"))
		fields := strings.Fields(string(line))
		if len(fields) == 0 {
			return EntryInterpreter, "the script names no interpreter"
		}
		interp := path.Clean(fields[0])
		if !path.IsAbs(fields[0]) || !strings.HasPrefix(interp, appRoot+"/") {
			return EntryInterpreter, "the script's interpreter " + fields[0] + " is outside the app folder; uploaded programs may not start system interpreters such as /bin/sh or python3"
		}
		return EntryOK, "script"
	}
	return EntryUnknownFormat, "the entry is neither a program for this server (ELF) nor a script starting with #!"
}

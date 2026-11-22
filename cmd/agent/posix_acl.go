package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sort"
)

// The portable half of the access-control-list entry on Roundcube's tree
// (posix_acl_linux.go has the system calls).

const (
	aclUserObj  = 0x01
	aclUser     = 0x02
	aclGroupObj = 0x04
	aclGroup    = 0x08
	aclMask     = 0x10
	aclOther    = 0x20
	aclNoID     = 0xffffffff
	aclVersion  = 2
	aclExecute  = 1
)

// errACLUnsupported: the file system does not accept access-control lists.
var errACLUnsupported = errors.New("the file system does not accept access-control lists")

type aclEntry struct {
	Tag  uint16
	Perm uint16
	ID   uint32
}

func decodeACL(raw []byte) ([]aclEntry, error) {
	if len(raw) < 4 || (len(raw)-4)%8 != 0 || binary.LittleEndian.Uint32(raw) != aclVersion {
		return nil, fmt.Errorf("not an access-control list the kernel writes")
	}
	entries := make([]aclEntry, 0, (len(raw)-4)/8)
	for at := 4; at < len(raw); at += 8 {
		entries = append(entries, aclEntry{
			Tag:  binary.LittleEndian.Uint16(raw[at:]),
			Perm: binary.LittleEndian.Uint16(raw[at+2:]),
			ID:   binary.LittleEndian.Uint32(raw[at+4:]),
		})
	}
	return entries, nil
}

func encodeACL(entries []aclEntry) []byte {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Tag != entries[j].Tag {
			return entries[i].Tag < entries[j].Tag
		}
		return entries[i].ID < entries[j].ID
	})
	raw := make([]byte, 4+8*len(entries))
	binary.LittleEndian.PutUint32(raw, aclVersion)
	for i, entry := range entries {
		at := 4 + 8*i
		binary.LittleEndian.PutUint16(raw[at:], entry.Tag)
		binary.LittleEndian.PutUint16(raw[at+2:], entry.Perm)
		binary.LittleEndian.PutUint32(raw[at+4:], entry.ID)
	}
	return raw
}

// aclWithUser is the access-control list a path has after its mode becomes
// mode and the entry "user:<uid>:<perm>" is added: the owner's entries are
// kept (a named entry for the same uid is replaced), the base entries follow
// the mode (as chmod makes them), and the mask grows by perm only, so the
// entry is effective and nothing else gains more than perm. It also returns
// the mode the path shows afterwards (the group bits are the mask).
func aclWithUser(before []byte, mode os.FileMode, uid int, perm uint16) ([]byte, os.FileMode, error) {
	bits := uint16(mode.Perm())
	owner, group, other := bits>>6&7, bits>>3&7, bits&7
	var entries []aclEntry
	if len(before) > 0 {
		decoded, err := decodeACL(before)
		if err != nil {
			return nil, 0, err
		}
		entries = decoded
	}
	hasMask := false
	for _, entry := range entries {
		if entry.Tag == aclMask {
			hasMask = true
		}
	}
	out := []aclEntry{}
	seen := map[uint16]bool{}
	for _, entry := range entries {
		switch entry.Tag {
		case aclUserObj:
			entry.Perm = owner
		case aclOther:
			entry.Perm = other
		case aclMask:
			entry.Perm = group
		case aclGroupObj:
			if !hasMask {
				entry.Perm = group
			}
		case aclUser:
			if entry.ID == uint32(uid) {
				continue
			}
		}
		seen[entry.Tag] = true
		out = append(out, entry)
	}
	for tag, value := range map[uint16]uint16{aclUserObj: owner, aclGroupObj: group, aclOther: other} {
		if !seen[tag] {
			out = append(out, aclEntry{Tag: tag, Perm: value, ID: aclNoID})
		}
	}
	out = append(out, aclEntry{Tag: aclUser, Perm: perm, ID: uint32(uid)})
	mask := group | perm
	if hasMask {
		for i := range out {
			if out[i].Tag == aclMask {
				out[i].Perm |= perm
				mask = out[i].Perm
			}
		}
	} else {
		out = append(out, aclEntry{Tag: aclMask, Perm: mask, ID: aclNoID})
	}
	after := mode&^os.ModePerm | os.FileMode(owner<<6|mask<<3|other)
	return encodeACL(out), after, nil
}

// aclHasUser: the list gives uid at least perm (named entry and mask).
func aclHasUser(raw []byte, uid int, perm uint16) bool {
	if len(raw) == 0 {
		return false
	}
	entries, err := decodeACL(raw)
	if err != nil {
		return false
	}
	named, mask := uint16(0), uint16(7)
	found := false
	for _, entry := range entries {
		switch {
		case entry.Tag == aclUser && entry.ID == uint32(uid):
			named, found = entry.Perm, true
		case entry.Tag == aclMask:
			mask = entry.Perm
		}
	}
	return found && named&mask&perm == perm
}

// sameACL compares two lists entry by entry ("none" equals "none").
func sameACL(a, b []byte) bool {
	if len(a) == 0 || len(b) == 0 {
		return len(a) == len(b)
	}
	left, errA := decodeACL(a)
	right, errB := decodeACL(b)
	if errA != nil || errB != nil || len(left) != len(right) {
		return false
	}
	encodeACL(left)
	encodeACL(right)
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

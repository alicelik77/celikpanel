package dbremote

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
)

// The Agent's record of what it changed for remote access, so that it only
// ever undoes its own changes (D-022): an engine setting it set, a MariaDB
// account it made, its option file and its jail. Root-owned, 0600, at
// StatePath. Losing it fails closed: CelikPanel then treats those changes as
// the owner's and leaves them, and says so.
//
// Agent'ın uzak erişim için yaptığı değişikliklerin kaydı; yalnız kendi
// değişikliklerini geri alır.
const (
	StatePath    = "/etc/celikpanel/database-remote.json"
	StateVersion = 1
	maxStateSize = 256 << 10
)

// State is the record of every target.
type State struct {
	Version int                    `json:"version"`
	Targets map[string]TargetState `json:"targets"`
}

// TargetState is what CelikPanel changed on one engine instance.
type TargetState struct {
	Engine string `json:"engine"`
	Port   int    `json:"port"`
	// Addresses: the addresses the nft file allows to this port now.
	Addresses []string `json:"addresses,omitempty"`
	// Listen: CelikPanel set the listen address (PostgreSQL: ALTER SYSTEM;
	// MariaDB: its option file).
	Listen bool `json:"listen,omitempty"`
	// ListenPending: the listen address went back to local but the engine
	// may still listen beyond it until it restarts; the nft file keeps
	// dropping the port meanwhile.
	ListenPending bool `json:"listen_pending,omitempty"`
	// TLS: CelikPanel turned TLS on (PostgreSQL: ALTER SYSTEM; MariaDB: in
	// its option file).
	TLS bool `json:"tls,omitempty"`
	// AuthDelay: CelikPanel set shared_preload_libraries = 'auth_delay'.
	AuthDelay bool `json:"auth_delay,omitempty"`
	// Jail: CelikPanel wrote the Fail2ban jail (MariaDB).
	Jail bool `json:"jail,omitempty"`
	// Accounts: MariaDB accounts CelikPanel made, user@host.
	Accounts []string `json:"accounts,omitempty"`
}

// Empty: nothing CelikPanel changed is left on this target.
func (s TargetState) Empty() bool {
	return len(s.Addresses) == 0 && !s.Listen && !s.ListenPending && !s.TLS && !s.AuthDelay && !s.Jail && len(s.Accounts) == 0
}

// EncodeState writes the record in one stable form.
func EncodeState(state State) ([]byte, error) {
	state.Version = StateVersion
	if state.Targets == nil {
		state.Targets = map[string]TargetState{}
	}
	for key, target := range state.Targets {
		if key != TargetKey(target.Engine, target.Port) {
			return nil, fmt.Errorf("state key %q does not name its target", key)
		}
		sort.Strings(target.Addresses)
		sort.Strings(target.Accounts)
		if target.Empty() {
			delete(state.Targets, key)
			continue
		}
		state.Targets[key] = target
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// DecodeState reads the record strictly. An absent file is an empty record;
// a file that is not this shape is an error, never an empty record, so a
// damaged file can never make CelikPanel forget what it must undo.
func DecodeState(data []byte) (State, error) {
	if len(data) == 0 {
		return State{Version: StateVersion, Targets: map[string]TargetState{}}, nil
	}
	if len(data) > maxStateSize {
		return State{}, errors.New("the remote access record is larger than any record CelikPanel writes")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var state State
	if err := decoder.Decode(&state); err != nil {
		return State{}, fmt.Errorf("the remote access record cannot be read: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return State{}, errors.New("the remote access record has trailing data")
	}
	if state.Version != StateVersion {
		return State{}, fmt.Errorf("the remote access record has version %d, this release reads %d", state.Version, StateVersion)
	}
	if state.Targets == nil {
		state.Targets = map[string]TargetState{}
	}
	for key, target := range state.Targets {
		if key != TargetKey(target.Engine, target.Port) || (target.Engine != EngineMySQL && target.Engine != EnginePostgreSQL) {
			return State{}, fmt.Errorf("the remote access record names an unknown target %q", key)
		}
	}
	return state, nil
}

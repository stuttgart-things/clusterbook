package internal

import (
	"errors"
	"sync"
)

// ErrLedgerConflict means the network config changed between Load and Save, so
// the save was refused rather than overwriting that change. Retrying the whole
// operation — load again, decide again — is safe.
var ErrLedgerConflict = errors.New("network config changed since it was loaded")

// ledgerMu serializes ledger writes within this process. Only LedgerWrite takes it.
var ledgerMu sync.Mutex

// LedgerWrite is one load → change → save of the ledger.
//
// Issue #199: every writer used to load the whole config, change an entry and
// save the whole config back without any lock, so overlapping writers lost each
// other's updates — two reservations could receive the same address. Saving is
// only possible through a LedgerWrite, so a new write path cannot skip the lock.
//
// Keep it open across the DNS calls that follow the save: DD-WRT records are
// themselves a get → merge → set on nvram, and concurrent edits drop entries.
//
// The lock only covers this process. Across replicas, or against a manual
// kubectl edit, cr mode saves conditionally on the resourceVersion Load saw and
// returns ErrLedgerConflict when it moved. Disk mode has no such check: a config
// file is meant to be written by one clusterbook process.
type LedgerWrite struct {
	source, location, name string

	version string
	loaded  bool
	ended   bool
}

// BeginLedgerWrite blocks until no other ledger write is in progress. Always
// pair it with End, typically `defer w.End()`.
func BeginLedgerWrite(source, location, name string) *LedgerWrite {
	ledgerMu.Lock()
	return &LedgerWrite{source: source, location: location, name: name}
}

// End releases the lock. Calling it more than once is harmless.
func (l *LedgerWrite) End() {
	if l.ended {
		return
	}
	l.ended = true
	ledgerMu.Unlock()
}

// Load reads the current config and remembers its version for Save.
func (l *LedgerWrite) Load() (map[string]IPs, error) {
	ipList, version, err := loadProfileVersioned(l.source, l.location, l.name)
	if err != nil {
		return nil, err
	}
	l.version = version
	l.loaded = true
	return ipList, nil
}

// Save persists ipList, provided nobody changed the config since Load.
func (l *LedgerWrite) Save(ipList map[string]IPs) error {
	switch {
	case l.ended:
		return errors.New("ledger write: save after End")
	case !l.loaded:
		return errors.New("ledger write: save without Load")
	}

	version, err := saveConfig(ipList, l.source, l.location, l.name, l.version)
	if err != nil {
		return err
	}
	l.version = version
	return nil
}

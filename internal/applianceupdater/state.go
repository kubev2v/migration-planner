package applianceupdater

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func strictDecode(data []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return fmt.Errorf("trailing JSON content")
	}
	return nil
}

// AtomicWrite makes both the replacement and its directory entry durable.
func AtomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, ".upgrade-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	_, err = file.Write(data)
	err = errors.Join(err, file.Sync(), file.Close())
	if err != nil {
		return err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return err
	}
	folder, err := os.Open(dir)
	if err != nil {
		return err
	}
	return errors.Join(folder.Sync(), folder.Close())
}

type State struct{ Root string }

func (s State) write(name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return AtomicWrite(filepath.Join(s.Root, name), append(data, '\n'), 0600)
}

func (s State) read(name string, value any) error {
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, 1024*1024+1))
	if err != nil || len(data) > 1024*1024 {
		return fmt.Errorf("host state is unreadable or exceeds its size limit")
	}
	return strictDecode(data, value)
}

func (s State) Lock() (*os.File, error) {
	if err := os.MkdirAll(filepath.Join(s.Root, "host/attempts"), 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(s.Root, "host/coordinator.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("another host coordinator is active: %w", err)
	}
	return lock, nil
}

func (s State) Save(attempt Journal) error {
	if err := s.write("host/attempts/"+attempt.AttemptID+".json", attempt); err != nil {
		return err
	}
	return s.write("host/active.json", struct {
		AttemptID string `json:"attempt_id"`
	}{attempt.AttemptID})
}

func (s State) Active() (*Journal, error) {
	var active struct {
		AttemptID string `json:"attempt_id"`
	}
	if err := s.read("host/active.json", &active); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// A journal published before an interrupted pointer update must not
			// turn a partially migrated data directory into unmanaged startup.
			journals, globErr := filepath.Glob(filepath.Join(s.Root, "host/attempts/*.json"))
			if globErr != nil || len(journals) != 0 {
				return nil, fmt.Errorf("host journals exist without an active attempt")
			}
			return nil, nil
		}
		return nil, err
	}
	if !attemptID.MatchString(active.AttemptID) {
		return nil, fmt.Errorf("invalid active attempt")
	}
	var attempt Journal
	if err := s.read("host/attempts/"+active.AttemptID+".json", &attempt); err != nil {
		return nil, err
	}
	validIdentity := attempt.Version == 1 && attempt.AttemptID == active.AttemptID
	validImages := imageID.MatchString(attempt.Previous.ID) && imageID.MatchString(attempt.Candidate.ID)
	validArchive := filepath.Base(attempt.Archive) == attempt.Archive && attempt.Archive != "." && attempt.Archive != ".."
	if !validIdentity || !validImages || !validArchive {
		return nil, fmt.Errorf("invalid host journal")
	}

	switch attempt.Phase {
	case PhasePreparing, PhasePrepared, PhaseApplying, PhaseRollingBack, PhaseRestoring, PhaseCommitted, PhaseRolledBack, PhaseAborted, PhaseRecoveryFailed:
	default:
		return nil, fmt.Errorf("unknown host phase")
	}
	if attempt.Phase == PhaseCommitted && attempt.CommitTime.IsZero() {
		return nil, fmt.Errorf("commit time is missing")
	}
	if attempt.Reported && !attempt.Phase.allowsStartup() {
		return nil, fmt.Errorf("unfinished attempt cannot be marked reported")
	}
	if (attempt.Phase == PhaseRestoring || attempt.Phase == PhaseRolledBack) && !attempt.RollbackVerified {
		return nil, fmt.Errorf("old-image recovery has not been verified")
	}
	if attempt.Phase == PhaseRecoveryFailed {
		switch attempt.RecoveryFrom {
		case PhasePreparing, PhasePrepared, PhaseApplying, PhaseRollingBack, PhaseRestoring:
		default:
			return nil, fmt.Errorf("recovery failure has an invalid origin phase")
		}
	}
	return &attempt, nil
}

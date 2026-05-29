package mg

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const (
	installedProviderProfileFilename = "linph-profile.json"
	installedProviderStateVersion    = 1
	installedProviderPsi             = "psi"
	installedProviderVG              = "vg"
)

type installedProviderState struct {
	SchemaVersion  int                       `json:"schema_version"`
	ActiveProvider string                    `json:"active_provider"`
	Providers      installedProviderStateSet `json:"providers"`
}

type installedProviderStateSet struct {
	Psi *installedProfile   `json:"psi,omitempty"`
	VG  *installedVGProfile `json:"vg,omitempty"`
}

func (layout installLayout) installedProviderProfilePath() string {
	return filepath.Join(layout.installedRuntimeRoot(), installedProviderProfileFilename)
}

func readInstalledProviderState(layout installLayout) (installedProviderState, bool, error) {
	data, err := os.ReadFile(layout.installedProviderProfilePath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return installedProviderState{}, false, nil
		}
		return installedProviderState{}, false, err
	}
	var state installedProviderState
	if err := json.Unmarshal(data, &state); err != nil {
		return installedProviderState{}, false, err
	}
	if err := normalizeInstalledProviderState(&state); err != nil {
		return installedProviderState{}, false, err
	}
	return state, true, nil
}

func loadInstalledProviderState(layout installLayout) (installedProviderState, bool, error) {
	state, ok, err := readInstalledProviderState(layout)
	if err != nil || ok {
		return state, ok, err
	}

	legacy, legacyOK, err := readLegacyInstalledProfile(layout.installedProfilePath())
	if err != nil || !legacyOK {
		return installedProviderState{}, false, err
	}
	state = installedProviderStateFromPsi(legacy)
	if err := backupLegacyInstalledProfile(layout.installedProfilePath()); err != nil {
		return installedProviderState{}, false, fmt.Errorf("failed to back up legacy installed profile before migration: %w; rerun with permission to write %s", err, layout.installedRuntimeRoot())
	}
	if err := writeInstalledProviderState(layout, state); err != nil {
		return installedProviderState{}, false, fmt.Errorf("failed to migrate legacy installed profile to %s: %w; run linph install with sufficient privileges to repair provider state", layout.installedProviderProfilePath(), err)
	}
	return state, true, nil
}

func writeInstalledProviderState(layout installLayout, state installedProviderState) error {
	if err := normalizeInstalledProviderState(&state); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return copyBytesAtomic(append(data, '\n'), layout.installedProviderProfilePath(), 0o644)
}

func installedProviderStateFromPsi(profile installedProfile) installedProviderState {
	profile.Version = installedProfileVersion
	return installedProviderState{
		SchemaVersion:  installedProviderStateVersion,
		ActiveProvider: installedProviderPsi,
		Providers: installedProviderStateSet{
			Psi: &profile,
		},
	}
}

func normalizeInstalledProviderState(state *installedProviderState) error {
	if state.SchemaVersion == 0 {
		state.SchemaVersion = installedProviderStateVersion
	}
	if state.SchemaVersion != installedProviderStateVersion {
		return fmt.Errorf("unsupported provider state schema version: %d", state.SchemaVersion)
	}
	if state.Providers.Psi != nil {
		state.Providers.Psi.Version = installedProfileVersion
		if state.ActiveProvider == "" {
			state.ActiveProvider = installedProviderPsi
		}
	}
	if state.Providers.VG != nil {
		normalizeInstalledVGProfile(state.Providers.VG)
	}
	if state.ActiveProvider == "" {
		return fmt.Errorf("active provider is not set")
	}
	switch state.ActiveProvider {
	case installedProviderPsi:
		if state.Providers.Psi == nil {
			return fmt.Errorf("active provider psi is missing provider state")
		}
	case installedProviderVG:
		if state.Providers.VG == nil {
			return fmt.Errorf("active provider vg is missing provider state")
		}
	default:
		return fmt.Errorf("unsupported active provider: %s", state.ActiveProvider)
	}
	return nil
}

func installedPsiProfileFromState(state installedProviderState) (installedProfile, error) {
	if err := normalizeInstalledProviderState(&state); err != nil {
		return installedProfile{}, err
	}
	if state.Providers.Psi == nil {
		return installedProfile{}, fmt.Errorf("provider psi is missing provider state")
	}
	return *state.Providers.Psi, nil
}

func backupLegacyInstalledProfile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	backupPath := fmt.Sprintf("%s.legacy-backup-%d", path, time.Now().UnixNano())
	return copyBytesAtomic(data, backupPath, 0o644)
}

func readLegacyInstalledProfile(path string) (installedProfile, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return installedProfile{}, false, nil
		}
		return installedProfile{}, false, err
	}
	var profile installedProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return installedProfile{}, false, err
	}
	return profile, true, nil
}

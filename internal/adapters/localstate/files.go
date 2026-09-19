package localstate

import (
	"errors"
	"os"
	"path/filepath"

	"brewwarden/internal/domain"
)

type Files struct {
	ConfigPath string
	StatePath  string
}

func (f Files) LoadConfig(location string) (domain.Policy, error) {
	explicit := location != ""
	if !explicit {
		location = f.ConfigPath
	}
	if location == "" {
		return domain.Policy{}, errors.New("user configuration directory is unavailable")
	}
	info, err := os.Lstat(location)
	if errors.Is(err, os.ErrNotExist) && !explicit {
		return domain.DefaultPolicy(), nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 {
		return domain.Policy{}, errors.New("configuration must be a regular file without group or other write permission")
	}
	parent, err := os.OpenRoot(filepath.Dir(location))
	if err != nil {
		return domain.Policy{}, errors.New("cannot open configuration directory")
	}
	defer parent.Close()
	file, err := parent.Open(filepath.Base(location))
	if err != nil {
		return domain.Policy{}, errors.New("cannot open configuration")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return domain.Policy{}, errors.New("configuration changed while opening")
	}
	return ParseConfig(file)
}

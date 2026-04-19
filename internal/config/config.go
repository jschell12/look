// Package config manages per-repo .xmuggle/ directories and daemon config.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// RepoPaths holds paths under <repo>/.xmuggle/.
type RepoPaths struct {
	Root       string // <repo>/.xmuggle/
	QueueDir   string // <repo>/.xmuggle/queue/
	ResultsDir string // <repo>/.xmuggle/results/
	PeersDir   string // <repo>/.xmuggle/peers/
	ImagesFile string // <repo>/.xmuggle/images.json
	ConfigFile string // <repo>/.xmuggle/config.json
	RepoRoot   string // <repo>/
	Home       string
}

// GetRepoPaths returns paths for a specific repo root.
func GetRepoPaths(repoRoot string) RepoPaths {
	home, _ := os.UserHomeDir()
	root := filepath.Join(repoRoot, ".xmuggle")
	return RepoPaths{
		Root:       root,
		QueueDir:   filepath.Join(root, "queue"),
		ResultsDir: filepath.Join(root, "results"),
		PeersDir:   filepath.Join(root, "peers"),
		ImagesFile: filepath.Join(root, "images.json"),
		ConfigFile: filepath.Join(root, "config.json"),
		RepoRoot:   repoRoot,
		Home:       home,
	}
}

// DaemonPaths holds global daemon paths under ~/.xmuggle/.
type DaemonPaths struct {
	Root       string // ~/.xmuggle/
	LogsDir    string // ~/.xmuggle/logs/
	ConfigFile string // ~/.xmuggle/daemon.json
}

// GetDaemonPaths returns global daemon paths.
func GetDaemonPaths() DaemonPaths {
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, ".xmuggle")
	return DaemonPaths{
		Root:       root,
		LogsDir:    filepath.Join(root, "logs"),
		ConfigFile: filepath.Join(root, "daemon.json"),
	}
}

// EnsureRepoDirs creates .xmuggle/ subdirectories in a repo.
func EnsureRepoDirs(repoRoot string) error {
	p := GetRepoPaths(repoRoot)
	for _, d := range []string{p.Root, p.QueueDir, p.ResultsDir, p.PeersDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// EnsureDaemonDirs creates ~/.xmuggle/ directories for daemon state.
func EnsureDaemonDirs() error {
	p := GetDaemonPaths()
	for _, d := range []string{p.Root, p.LogsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

var hostnameSanitizer = regexp.MustCompile(`[^a-z0-9-]`)

// NormalizeHostname returns a filesystem-safe identifier.
func NormalizeHostname(raw string) string {
	s := strings.ToLower(raw)
	s = strings.TrimSuffix(s, ".local")
	s = hostnameSanitizer.ReplaceAllString(s, "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

// Config is the per-repo .xmuggle/config.json.
type Config struct {
	Version  int    `json:"version"`
	Hostname string `json:"hostname"`
}

// Load reads <repoRoot>/.xmuggle/config.json.
func Load(repoRoot string) (*Config, error) {
	p := GetRepoPaths(repoRoot)
	data, err := os.ReadFile(p.ConfigFile)
	if os.IsNotExist(err) {
		return defaultConfig(), nil
	}
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if cfg.Hostname == "" {
		h, _ := os.Hostname()
		cfg.Hostname = NormalizeHostname(h)
	}
	return cfg, nil
}

// Save writes the config to <repoRoot>/.xmuggle/config.json.
func Save(repoRoot string, cfg *Config) error {
	p := GetRepoPaths(repoRoot)
	if err := os.MkdirAll(filepath.Dir(p.ConfigFile), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(p.ConfigFile, data, 0o644)
}

func defaultConfig() *Config {
	h, _ := os.Hostname()
	return &Config{
		Version:  1,
		Hostname: NormalizeHostname(h),
	}
}

// DaemonConfig is the global ~/.xmuggle/daemon.json.
type DaemonConfig struct {
	Repos []string `json:"repos"` // absolute paths to repo roots
}

// LoadDaemonConfig reads ~/.xmuggle/daemon.json.
func LoadDaemonConfig() (*DaemonConfig, error) {
	p := GetDaemonPaths()
	data, err := os.ReadFile(p.ConfigFile)
	if os.IsNotExist(err) {
		return &DaemonConfig{}, nil
	}
	if err != nil {
		return nil, err
	}
	cfg := &DaemonConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// SaveDaemonConfig writes ~/.xmuggle/daemon.json.
func SaveDaemonConfig(cfg *DaemonConfig) error {
	p := GetDaemonPaths()
	if err := os.MkdirAll(filepath.Dir(p.ConfigFile), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(p.ConfigFile, data, 0o644)
}

// RegisterRepo adds a repo path to daemon.json (idempotent).
func RegisterRepo(repoRoot string) error {
	if err := EnsureDaemonDirs(); err != nil {
		return err
	}
	cfg, err := LoadDaemonConfig()
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return err
	}
	for _, r := range cfg.Repos {
		if r == abs {
			return nil // already registered
		}
	}
	cfg.Repos = append(cfg.Repos, abs)
	return SaveDaemonConfig(cfg)
}

// Package config manages ~/.xmuggle directories and the JSON config file.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type GitConfig struct {
	QueueRepo      string `json:"queue_repo"`
	CloneDir       string `json:"clone_dir"`
	PollIntervalMS int    `json:"poll_interval_ms"`
	Branch         string `json:"branch"`
	AuthorName     string `json:"author_name"`
	AuthorEmail    string `json:"author_email"`
}

type AgeConfig struct {
	IdentityFile string `json:"identity_file"`
	Pubkey       string `json:"pubkey"`
}

type Recipient struct {
	Hostname string `json:"hostname"`
	Pubkey   string `json:"pubkey"`
}

type Retention struct {
	QueueDays   int `json:"queue_days"`
	ResultsDays int `json:"results_days"`
}

type Config struct {
	Version          int         `json:"version"`
	Hostname         string      `json:"hostname"`
	Git              *GitConfig  `json:"git,omitempty"`
	Age              *AgeConfig  `json:"age,omitempty"`
	Recipients       []Recipient `json:"recipients,omitempty"`
	DefaultRecipient string      `json:"default_recipient,omitempty"`
	Retention        *Retention  `json:"retention,omitempty"`
}

// Paths returns commonly used ~/.xmuggle paths.
type Paths struct {
	ConfigDir    string
	QueueDir     string
	ResultsDir   string
	LogsDir      string
	QueueRepoDir string
	AgeDir       string
	ConfigFile   string
	Home         string
}

func GetPaths() Paths {
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, ".xmuggle")
	return Paths{
		ConfigDir:    root,
		QueueDir:     filepath.Join(root, "queue"),
		ResultsDir:   filepath.Join(root, "results"),
		LogsDir:      filepath.Join(root, "logs"),
		QueueRepoDir: filepath.Join(root, "queue-repo"),
		AgeDir:       filepath.Join(root, "age"),
		ConfigFile:   filepath.Join(root, "config.json"),
		Home:         home,
	}
}

// EnsureDirs creates all the ~/.xmuggle subdirectories.
func EnsureDirs() error {
	p := GetPaths()
	for _, d := range []string{p.ConfigDir, p.QueueDir, p.ResultsDir, p.LogsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(p.AgeDir, 0o700); err != nil {
		return err
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

// DefaultIdentityPath returns the path for the local age identity file.
func DefaultIdentityPath() string {
	return filepath.Join(GetPaths().AgeDir, "key.txt")
}

// Load reads ~/.xmuggle/config.json, creating a default if missing.
func Load() (*Config, error) {
	if err := EnsureDirs(); err != nil {
		return nil, err
	}
	p := GetPaths()
	data, err := os.ReadFile(p.ConfigFile)
	if os.IsNotExist(err) {
		cfg := defaultConfig()
		if err := Save(cfg); err != nil {
			return nil, err
		}
		return cfg, nil
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

// Save writes the config to disk.
func Save(cfg *Config) error {
	if err := EnsureDirs(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(GetPaths().ConfigFile, data, 0o644)
}

func defaultConfig() *Config {
	h, _ := os.Hostname()
	return &Config{
		Version:  1,
		Hostname: NormalizeHostname(h),
	}
}

// SetGit configures git transport with defaults for missing fields.
func (c *Config) SetGit(queueRepo string) {
	if c.Git == nil {
		c.Git = &GitConfig{}
	}
	c.Git.QueueRepo = queueRepo
	if c.Git.CloneDir == "" {
		c.Git.CloneDir = GetPaths().QueueRepoDir
	}
	if c.Git.PollIntervalMS == 0 {
		c.Git.PollIntervalMS = 10_000
	}
	if c.Git.Branch == "" {
		c.Git.Branch = "main"
	}
	if c.Git.AuthorName == "" {
		c.Git.AuthorName = "xmuggle bot"
	}
	if c.Git.AuthorEmail == "" {
		c.Git.AuthorEmail = "xmuggle@localhost"
	}
}

// SetAge stores the age identity.
func (c *Config) SetAge(identityFile, pubkey string) {
	c.Age = &AgeConfig{IdentityFile: identityFile, Pubkey: pubkey}
}

// RecipientPubkey returns the configured pubkey for a hostname, or "" if missing.
func (c *Config) RecipientPubkey(hostname string) string {
	for _, r := range c.Recipients {
		if r.Hostname == hostname {
			return r.Pubkey
		}
	}
	return ""
}

// UpsertRecipient adds or replaces a recipient.
func (c *Config) UpsertRecipient(r Recipient) {
	for i, existing := range c.Recipients {
		if existing.Hostname == r.Hostname {
			c.Recipients[i] = r
			return
		}
	}
	c.Recipients = append(c.Recipients, r)
}

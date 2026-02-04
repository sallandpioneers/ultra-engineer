package config

import (
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Provider     string        `yaml:"provider"`
	PollInterval time.Duration `yaml:"poll_interval"`
	TriggerLabel string        `yaml:"trigger_label"`
	LogFile      string        `yaml:"log_file"`

	Gitea  GiteaConfig  `yaml:"gitea"`
	GitHub GitHubConfig `yaml:"github"`
	GitLab GitLabConfig `yaml:"gitlab"`

	Claude      ClaudeConfig      `yaml:"claude"`
	Retry       RetryConfig       `yaml:"retry"`
	Defaults    DefaultsConfig    `yaml:"defaults"`
	Concurrency ConcurrencyConfig `yaml:"concurrency"`
	Progress    ProgressConfig    `yaml:"progress"`
}

type GiteaConfig struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

type GitHubConfig struct {
	Token string `yaml:"token"`
}

type GitLabConfig struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

type ClaudeConfig struct {
	Command      string        `yaml:"command"`
	Timeout      time.Duration `yaml:"timeout"`
	ReviewCycles int           `yaml:"review_cycles"`
}

type RetryConfig struct {
	MaxAttempts    int           `yaml:"max_attempts"`
	BackoffBase    time.Duration `yaml:"backoff_base"`
	RateLimitRetry time.Duration `yaml:"rate_limit_retry"`
}

type DefaultsConfig struct {
	BaseBranch string `yaml:"base_branch"`
	AutoMerge  bool   `yaml:"auto_merge"`
}

// ConcurrencyConfig controls concurrent issue processing
type ConcurrencyConfig struct {
	MaxPerRepo          int    `yaml:"max_per_repo"`          // Maximum concurrent issues per repository (default: 1)
	MaxTotal            int    `yaml:"max_total"`             // Maximum total concurrent issues (default: 5)
	DependencyDetection string `yaml:"dependency_detection"`  // "auto" | "manual" | "disabled" (default: "auto")
}

// ProgressConfig controls progress reporting
type ProgressConfig struct {
	Enabled          bool          `yaml:"enabled"`           // Enable progress comments (default: true)
	DebounceInterval time.Duration `yaml:"debounce_interval"` // Minimum time between updates (default: 60s)
}

// Default configuration values
func DefaultConfig() *Config {
	return &Config{
		Provider:     "gitea",
		PollInterval: 60 * time.Second,
		TriggerLabel: "ai-implement",
		Claude: ClaudeConfig{
			Command:      "claude",
			Timeout:      30 * time.Minute,
			ReviewCycles: 5,
		},
		Retry: RetryConfig{
			MaxAttempts:    3,
			BackoffBase:    10 * time.Second,
			RateLimitRetry: 5 * time.Minute,
		},
		Defaults: DefaultsConfig{
			BaseBranch: "main",
			AutoMerge:  true,
		},
		Concurrency: ConcurrencyConfig{
			MaxPerRepo:          1,
			MaxTotal:            5,
			DependencyDetection: "auto",
		},
		Progress: ProgressConfig{
			Enabled:          true,
			DebounceInterval: 60 * time.Second,
		},
	}
}

// Load reads configuration from a YAML file
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Expand environment variables in the format ${VAR}
	data = expandEnvVars(data)

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// expandEnvVars replaces ${VAR} patterns with environment variable values
func expandEnvVars(data []byte) []byte {
	re := regexp.MustCompile(`\$\{([^}]+)\}`)
	return re.ReplaceAllFunc(data, func(match []byte) []byte {
		varName := string(re.FindSubmatch(match)[1])
		return []byte(os.Getenv(varName))
	})
}

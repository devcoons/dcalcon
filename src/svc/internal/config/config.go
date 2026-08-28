package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/devcoons/dcalcon/internal/limits"
	"gopkg.in/yaml.v3"
)

type Config struct {
	HTTP             HTTP      `yaml:"http"`
	SQLite           SQLite    `yaml:"sqlite"`
	Auth             Auth      `yaml:"auth"`
	Bootstrap        Bootstrap `yaml:"bootstrap"`
	Worker           Worker    `yaml:"worker"`
	Backup           Backup    `yaml:"backup"`
	OAuth            OAuth     `yaml:"oauth"`
	Mail             Mail      `yaml:"mail"`
	SchedulingDomain string    `yaml:"scheduling_domain"`
}

type Mail struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	From     string `yaml:"from"`
}

type HTTP struct {
	Addr           string   `yaml:"addr"`
	PublicURL      string   `yaml:"public_url"`
	CORSOrigins    []string `yaml:"cors_origins"`
	MaxBodyBytes   int64    `yaml:"max_body_bytes"`
	TrustedProxies []string `yaml:"trusted_proxies"`
	MetricsToken   string   `yaml:"metrics_token"`
}

type SQLite struct {
	Path string `yaml:"path"`
}

type Auth struct {
	Realm            string `yaml:"realm"`
	SessionSecure    bool   `yaml:"session_secure"`
	SessionTTL       time.Duration
	SessionTTLRaw    string `yaml:"session_ttl"`
	TokenKey         string `yaml:"token_key"`
	MaxAttempts      int    `yaml:"max_attempts"`
	AttemptWindow    time.Duration
	AttemptWindowRaw string `yaml:"attempt_window"`
	Lockout          time.Duration
	LockoutRaw       string `yaml:"lockout"`
}

type Bootstrap struct {
	AdminUsername string `yaml:"admin_username"`
	AdminPassword string `yaml:"admin_password"`
}

type Worker struct {
	Interval    time.Duration
	IntervalRaw string `yaml:"interval"`
}

type Backup struct {
	Dir         string `yaml:"dir"`
	Interval    time.Duration
	IntervalRaw string `yaml:"interval"`
	Keep        int    `yaml:"keep"`
	Hook        string `yaml:"hook"`
}

type OAuth struct {
	GoogleClientID        string `yaml:"google_client_id"`
	GoogleClientSecret    string `yaml:"google_client_secret"`
	MicrosoftClientID     string `yaml:"microsoft_client_id"`
	MicrosoftClientSecret string `yaml:"microsoft_client_secret"`
	MicrosoftTenant       string `yaml:"microsoft_tenant"`
}

func Default() Config {
	return Config{
		HTTP: HTTP{
			Addr:         ":8080",
			PublicURL:    "http://localhost:8080",
			MaxBodyBytes: limits.MaxHTTPBody,
		},
		SQLite: SQLite{Path: "./data/dcalcon.db"},
		Auth: Auth{
			Realm:         "dCalCon",
			SessionSecure: false,
			SessionTTL:    24 * time.Hour,
			MaxAttempts:   8,
			AttemptWindow: 15 * time.Minute,
			Lockout:       15 * time.Minute,
		},
		Worker: Worker{Interval: 5 * time.Minute},
		Backup: Backup{Keep: 14, Interval: 24 * time.Hour},
	}
}

func Load() (Config, error) {
	cfg := Default()

	if path := os.Getenv("DCALCON_CONFIG"); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("read config: %w", err)
		}
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return cfg, fmt.Errorf("parse config: %w", err)
		}
	}

	if v := os.Getenv("DCALCON_HTTP_ADDR"); v != "" {
		cfg.HTTP.Addr = v
	}
	if v := os.Getenv("DCALCON_PUBLIC_URL"); v != "" {
		cfg.HTTP.PublicURL = strings.TrimRight(v, "/")
	}
	if v := os.Getenv("DCALCON_SQLITE_PATH"); v != "" {
		cfg.SQLite.Path = v
	}
	if v := os.Getenv("DCALCON_AUTH_REALM"); v != "" {
		cfg.Auth.Realm = v
	}
	if v := os.Getenv("DCALCON_SESSION_SECURE"); v != "" {
		cfg.Auth.SessionSecure = v == "1" || strings.EqualFold(v, "true")
	}
	if v := os.Getenv("DCALCON_ADMIN_USERNAME"); v != "" {
		cfg.Bootstrap.AdminUsername = v
	}
	if v := os.Getenv("DCALCON_ADMIN_PASSWORD"); v != "" {
		cfg.Bootstrap.AdminPassword = v
	}
	if v := os.Getenv("DCALCON_TOKEN_KEY"); v != "" {
		cfg.Auth.TokenKey = v
	}
	if v := os.Getenv("GOOGLE_OAUTH_CLIENT_ID"); v != "" {
		cfg.OAuth.GoogleClientID = v
	}
	if v := os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET"); v != "" {
		cfg.OAuth.GoogleClientSecret = v
	}
	if v := os.Getenv("MICROSOFT_OAUTH_CLIENT_ID"); v != "" {
		cfg.OAuth.MicrosoftClientID = v
	}
	if v := os.Getenv("MICROSOFT_OAUTH_CLIENT_SECRET"); v != "" {
		cfg.OAuth.MicrosoftClientSecret = v
	}
	if v := os.Getenv("MICROSOFT_OAUTH_TENANT"); v != "" {
		cfg.OAuth.MicrosoftTenant = v
	}
	if v := os.Getenv("DCALCON_SMTP_HOST"); v != "" {
		cfg.Mail.Host = v
	}
	if v := os.Getenv("DCALCON_SMTP_PORT"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			cfg.Mail.Port = n
		}
	}
	if v := os.Getenv("DCALCON_SMTP_USERNAME"); v != "" {
		cfg.Mail.Username = v
	}
	if v := os.Getenv("DCALCON_SMTP_PASSWORD"); v != "" {
		cfg.Mail.Password = v
	}
	if v := os.Getenv("DCALCON_SMTP_FROM"); v != "" {
		cfg.Mail.From = v
	}
	if v := os.Getenv("DCALCON_CORS_ORIGINS"); v != "" {
		cfg.HTTP.CORSOrigins = splitCSV(v)
	}
	if v := os.Getenv("DCALCON_MAX_BODY_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			cfg.HTTP.MaxBodyBytes = n
		}
	}
	if v := os.Getenv("DCALCON_AUTH_MAX_ATTEMPTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Auth.MaxAttempts = n
		}
	}
	if v := os.Getenv("DCALCON_AUTH_WINDOW"); v != "" {
		cfg.Auth.AttemptWindowRaw = v
	}
	if v := os.Getenv("DCALCON_AUTH_LOCKOUT"); v != "" {
		cfg.Auth.LockoutRaw = v
	}
	if v := os.Getenv("DCALCON_BACKUP_DIR"); v != "" {
		cfg.Backup.Dir = v
	}
	if v := os.Getenv("DCALCON_BACKUP_INTERVAL"); v != "" {
		cfg.Backup.IntervalRaw = v
	}
	if v := os.Getenv("DCALCON_BACKUP_KEEP"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Backup.Keep = n
		}
	}
	if v := os.Getenv("DCALCON_BACKUP_HOOK"); v != "" {
		cfg.Backup.Hook = v
	}
	if v := os.Getenv("DCALCON_SCHEDULING_DOMAIN"); v != "" {
		cfg.SchedulingDomain = v
	}
	if v := os.Getenv("DCALCON_TRUSTED_PROXIES"); v != "" {
		cfg.HTTP.TrustedProxies = splitCSV(v)
	}
	if v := os.Getenv("DCALCON_METRICS_TOKEN"); v != "" {
		cfg.HTTP.MetricsToken = strings.TrimSpace(v)
	}

	if err := cfg.applyDurations(); err != nil {
		return cfg, err
	}
	cfg.finalize()
	return cfg, nil
}

func (cfg *Config) applyDurations() error {
	if cfg.Auth.SessionTTLRaw != "" {
		d, err := time.ParseDuration(cfg.Auth.SessionTTLRaw)
		if err != nil {
			return fmt.Errorf("session_ttl: %w", err)
		}
		cfg.Auth.SessionTTL = d
	}
	if cfg.Auth.SessionTTL == 0 {
		cfg.Auth.SessionTTL = 24 * time.Hour
	}
	if cfg.Auth.AttemptWindowRaw != "" {
		d, err := time.ParseDuration(cfg.Auth.AttemptWindowRaw)
		if err != nil {
			return fmt.Errorf("attempt_window: %w", err)
		}
		cfg.Auth.AttemptWindow = d
	}
	if cfg.Auth.AttemptWindow == 0 {
		cfg.Auth.AttemptWindow = 15 * time.Minute
	}
	if cfg.Auth.LockoutRaw != "" {
		d, err := time.ParseDuration(cfg.Auth.LockoutRaw)
		if err != nil {
			return fmt.Errorf("lockout: %w", err)
		}
		cfg.Auth.Lockout = d
	}
	if cfg.Auth.Lockout == 0 {
		cfg.Auth.Lockout = 15 * time.Minute
	}
	if cfg.Auth.MaxAttempts <= 0 {
		cfg.Auth.MaxAttempts = 8
	}
	if cfg.Worker.IntervalRaw != "" {
		d, err := time.ParseDuration(cfg.Worker.IntervalRaw)
		if err != nil {
			return fmt.Errorf("worker.interval: %w", err)
		}
		cfg.Worker.Interval = d
	}
	if cfg.Worker.Interval == 0 {
		cfg.Worker.Interval = 5 * time.Minute
	}
	if cfg.Backup.IntervalRaw != "" {
		d, err := time.ParseDuration(cfg.Backup.IntervalRaw)
		if err != nil {
			return fmt.Errorf("backup.interval: %w", err)
		}
		cfg.Backup.Interval = d
	}
	if cfg.Backup.Interval == 0 {
		cfg.Backup.Interval = 24 * time.Hour
	}
	if cfg.Backup.Keep <= 0 {
		cfg.Backup.Keep = 14
	}
	if cfg.HTTP.Addr == "" {
		cfg.HTTP.Addr = ":8080"
	}
	if cfg.HTTP.MaxBodyBytes <= 0 {
		cfg.HTTP.MaxBodyBytes = limits.MaxHTTPBody
	}
	for _, p := range cfg.HTTP.TrustedProxies {
		if _, _, err := net.ParseCIDR(normalizeCIDR(p)); err != nil {
			if ip := net.ParseIP(strings.TrimSpace(p)); ip == nil {
				return fmt.Errorf("trusted_proxies: invalid CIDR or IP %q", p)
			}
		}
	}
	return nil
}

func normalizeCIDR(s string) string {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "/") {
		return s
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return s
	}
	if ip.To4() != nil {
		return ip.To4().String() + "/32"
	}
	return ip.String() + "/128"
}

func (cfg *Config) finalize() {
	cfg.HTTP.PublicURL = strings.TrimRight(cfg.HTTP.PublicURL, "/")
	if strings.HasPrefix(strings.ToLower(cfg.HTTP.PublicURL), "https://") {
		cfg.Auth.SessionSecure = true
	}
	cfg.SchedulingDomain = strings.ToLower(strings.TrimSpace(cfg.SchedulingDomain))
	if cfg.SchedulingDomain == "" {
		cfg.SchedulingDomain = "dcalcon.private"
	}
}

func (cfg Config) AllowedOrigins() []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.TrimRight(strings.TrimSpace(s), "/")
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	add(cfg.HTTP.PublicURL)
	if u, err := url.Parse(cfg.HTTP.PublicURL); err == nil && u.Host != "" {
		add(u.Scheme + "://" + u.Hostname())
		if u.Scheme == "https" {
			add("https://" + u.Host)
		}
	}
	add("http://localhost:3000")
	add("http://127.0.0.1:3000")
	for _, o := range cfg.HTTP.CORSOrigins {
		add(o)
	}
	return out
}

func (cfg Config) PublicIsHTTPS() bool {
	return strings.HasPrefix(strings.ToLower(cfg.HTTP.PublicURL), "https://")
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

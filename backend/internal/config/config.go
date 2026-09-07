// Package config loads settings from environment variables and an optional .env file.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Settings holds all runtime configuration for CAP.
type Settings struct {
	Environment   string
	HTTPAddr      string
	DBURL         string
	SimulatorOn   bool
	SimInterval   time.Duration
	EmergencySec  time.Duration
	StalenessSec  time.Duration
	MaxBatchSize  int
	DefaultLimit  int
	MaxLimit      int
	DBPathDefault string
}

// Load reads .env from the given path (ignored if missing) and then environment
// variables, applying defaults. Existing environment variables take precedence.
func Load(envPath string) (*Settings, error) {
	loadDotEnv(envPath)

	s := &Settings{
		Environment:   get("ENVIRONMENT", "development"),
		HTTPAddr:      get("HTTP_ADDR", "127.0.0.1:8000"),
		DBURL:         get("DB_URL", "sqlite://./cap.db"),
		SimulatorOn:   getBool("SIMULATOR_ENABLED", true),
		SimInterval:   getDuration("SIMULATOR_INTERVAL", 1*time.Second),
		EmergencySec:  getDuration("EMERGENCY_DURATION", 15*time.Second),
		StalenessSec:  getDuration("STALENESS_SECONDS", 60*time.Second),
		MaxBatchSize:  getInt("INGEST_MAX_BATCH", 500),
		DefaultLimit:  getInt("DEFAULT_LIMIT", 100),
		MaxLimit:      getInt("MAX_LIMIT", 1000),
		DBPathDefault: "cap.db",
	}
	if err := s.validate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Settings) validate() error {
	if s.MaxBatchSize < 1 || s.MaxBatchSize > 10000 {
		return fmt.Errorf("INGEST_MAX_BATCH must be in [1, 10000], got %d", s.MaxBatchSize)
	}
	if s.DefaultLimit < 1 || s.MaxLimit < s.DefaultLimit {
		return fmt.Errorf("invalid limit settings default=%d max=%d", s.DefaultLimit, s.MaxLimit)
	}
	return nil
}

func get(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func getInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

// loadDotEnv reads a simple KEY=VALUE file into the process environment without
// overriding variables that are already set.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		val = strings.Trim(val, `"'`)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		_ = os.Setenv(key, val)
	}
}

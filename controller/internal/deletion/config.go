package deletion

import (
	"os"
	"strconv"
	"time"
)

func getEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultVal
}

const (
	defaultPollInterval = 5 * time.Second
	defaultBatchSize    = 10
	defaultMaxAttempts  = 5
	defaultCHTimeout    = 5 * time.Minute
)

func pollInterval() time.Duration {
	return getEnvDuration("DELETION_POLL_INTERVAL", defaultPollInterval)
}

func batchSize() int {
	return getEnvInt("DELETION_BATCH_SIZE", defaultBatchSize)
}

func maxAttempts() int {
	return getEnvInt("DELETION_MAX_ATTEMPTS", defaultMaxAttempts)
}

func chTimeout() time.Duration {
	return getEnvDuration("DELETION_CH_TIMEOUT", defaultCHTimeout)
}

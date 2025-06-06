package main

import (
	"flag"
	"runtime"
	"runtime/debug"
	"time"
)

type Config struct {
	MaxMemories     int
	MaxMemoryMB     int
	DecayInterval   time.Duration
	Port            int
	EnableProfiling bool
}

func LoadConfig() *Config {
	config := &Config{}

	flag.IntVar(&config.MaxMemories, "max-memories", 1000, "Maximum number of memories to store")
	flag.IntVar(&config.MaxMemoryMB, "max-memory-mb", 100, "Maximum memory usage in MB")
	flag.DurationVar(&config.DecayInterval, "decay-interval", 5*time.Minute, "Memory decay check interval")
	flag.IntVar(&config.Port, "port", 0, "Port for HTTP transport (0 for stdio)")
	flag.BoolVar(&config.EnableProfiling, "profile", false, "Enable memory profiling")

	flag.Parse()

	return config
}

// Memory-efficient initialization
func InitializeMemoryLimits(config *Config) {
	// Set memory limit
	debug.SetMemoryLimit(int64(config.MaxMemoryMB) * 1024 * 1024)

	// Configure GC to be more aggressive
	debug.SetGCPercent(50) // Run GC more frequently

	// Limit number of OS threads
	runtime.GOMAXPROCS(2) // Use only 2 CPU cores
}

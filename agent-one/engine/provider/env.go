package provider

import "os"

func lookupEnv(key string) string { return os.Getenv(key) }

// DefaultModel is the model a provider uses when neither the flag nor AGENT_ONE_MODEL names one.
func DefaultModel(fallback string) string {
	if m := Getenv("AGENT_ONE_MODEL"); m != "" {
		return m
	}
	return fallback
}

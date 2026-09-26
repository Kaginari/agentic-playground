package openai

import "time"

func init() { backoff = func(int) time.Duration { return time.Millisecond } }

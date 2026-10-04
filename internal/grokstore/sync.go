package grokstore

import (
	"context"
	"time"
)

// RunSync preserves renewed credentials even when the UI tab is closed.
// Cancellation ends the worker; network refresh remains owned by the Grok CLI.
func (s *Store) RunSync(ctx context.Context, interval time.Duration, report func(error)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if _, err := s.List(); err != nil && report != nil {
			report(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

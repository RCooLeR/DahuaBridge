package media

import (
	"context"
	"testing"

	"RCooLeR/DahuaBridge/internal/config"
	"github.com/rs/zerolog"
)

func TestInvalidatedWorkerCannotReportFailureForNewGeneration(t *testing.T) {
	resolver := &failureReportingResolver{}
	m := New(config.MediaConfig{Enabled: true}, resolver, zerolog.Nop(), nil)
	_, oldProfile, _, err := m.resolveStream("cam", "stable")
	if err != nil {
		t.Fatal(err)
	}
	m.InvalidateStream("cam")
	// Invalidation publishes the new generation before calling old cancel funcs.
	// The upstream URL can be identical after restoring a previous preference.
	m.reportLiveSourceFailure(context.Background(), "cam", oldProfile)
	if len(resolver.failures) != 0 {
		t.Fatal("old worker failure reached the new live source generation")
	}
	_, currentProfile, _, err := m.resolveStream("cam", "stable")
	if err != nil {
		t.Fatal(err)
	}
	m.reportLiveSourceFailure(context.Background(), "cam", currentProfile)
	if len(resolver.failures) != 1 {
		t.Fatal("current worker failure was ignored")
	}
}

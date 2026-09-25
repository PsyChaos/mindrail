package impact_test

import (
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/impact"
	"github.com/PsyChaos/mindrail/internal/perf"
)

// TestAnalyzeObservesTraversal pins the telemetry wiring: one Analyze run
// banks the traversal span on the ambient timer. Presence is asserted,
// never the value.
func TestAnalyzeObservesTraversal(t *testing.T) {
	fx := newImpactFixture(t)
	seedChain(t, fx)
	timer := perf.Start(app.SystemClock{})
	if _, err := fx.service.Analyze(perf.WithTimer(t.Context(), timer), impact.Request{
		Symbols:    []impact.Input{{UID: "SYM-I-A"}},
		DirectOnly: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := timer.Stop().Breakdowns[perf.Traversal]; !ok {
		t.Fatal("traversal span not observed")
	}
}

package impact_test

// Traversal benchmark (MR-019 TASK-01): Analyze driven directly with an
// ambient timer so the traversal span publishes into the report. This is
// a component span, not an operation SLO — it reports p50/p95 without
// grading against the operation targets.

import (
	"context"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/impact"
	"github.com/PsyChaos/mindrail/internal/perf"
)

func BenchmarkAnalyze(b *testing.B) {
	fx := newImpactFixture(b)
	seedChain(b, fx)
	recorder := perf.NewRecorder("impact.traversal")
	for range b.N {
		timer := perf.Start(app.SystemClock{})
		_, err := fx.service.Analyze(perf.WithTimer(context.Background(), timer), impact.Request{
			Symbols:    []impact.Input{{UID: "SYM-I-A"}},
			DirectOnly: true,
		})
		if err != nil {
			b.Fatal(err)
		}
		recorder.Add(timer)
	}
	b.Logf("impact.traversal p50=%s p95=%s traversal-p95=%s samples=%d",
		recorder.Percentile(50), recorder.Percentile(95),
		recorder.PartPercentile(perf.Traversal, 95), recorder.Count())
}

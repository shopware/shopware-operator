package trigger

import (
	"slices"
	"strings"
	"time"

	"github.com/shopware/shopware-operator/internal/tracing"
	"go.opentelemetry.io/otel/trace"
)

func Cause(triggers []Trigger) string {
	if len(triggers) == 0 {
		return "unknown"
	}
	switch triggers[0].Event {
	case EventRequeue:
		return "requeue"
	case EventResync:
		return "periodic resync"
	case EventStartup:
		return "operator start"
	default:
		return "watch event"
	}
}

func RecordOnSpan(span trace.Span, triggers []Trigger) {
	span.SetAttributes(
		tracing.AttrTriggerCause.String(Cause(triggers)),
		tracing.AttrTriggerCount.Int(len(triggers)),
		tracing.AttrTriggerSources.String(strings.Join(Sources(triggers), ",")),
		tracing.AttrTriggers.StringSlice(Summaries(triggers)),
	)
	now := time.Now()
	for _, t := range triggers {
		span.AddEvent(tracing.EventReconcileTrigger, trace.WithAttributes(
			tracing.AttrTriggerSource.String(t.Source),
			tracing.AttrTriggerEvent.String(t.Event),
			tracing.AttrObjectName.String(t.Name),
			tracing.AttrTriggerReason.String(t.Reason),
			tracing.AttrTriggerChanges.StringSlice(t.Changes),
			tracing.AttrTriggerDelay.String(now.Sub(t.At).Round(time.Millisecond).String()),
		))
	}
}

func Sources(triggers []Trigger) []string {
	sources := make([]string, 0, len(triggers))
	for _, t := range triggers {
		sources = append(sources, t.Source)
	}
	slices.Sort(sources)
	return slices.Compact(sources)
}

func Summaries(triggers []Trigger) []string {
	summaries := make([]string, 0, len(triggers))
	for _, t := range triggers {
		summaries = append(summaries, t.String())
	}
	return summaries
}

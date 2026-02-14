package pipeline

import (
	"context"
	"time"

	"watcher/internal/highlight"
	"watcher/internal/rules"
	"watcher/internal/watch"
)

// HighlightedEvent is consumed by the TUI layer.
type HighlightedEvent struct {
	Timestamp time.Time
	Path      string
	Line      string
	RuleName  string
	Severity  rules.Severity
	Color     string
	Tags      []string
	Fragments []highlight.Fragment
	Err       error
}

type Stream struct {
	rules       rules.RuleSet
	showAll     bool
	minSeverity rules.Severity
}

// New creates a pipeline stream from a ruleset.
func New(rs rules.RuleSet, showAll bool, min rules.Severity) Stream {
	return Stream{rules: rs, showAll: showAll, minSeverity: min}
}

// Connect wires a tail stream to highlighted output.
func (s Stream) Connect(ctx context.Context, in <-chan watch.LogEvent) <-chan HighlightedEvent {
	out := make(chan HighlightedEvent)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-in:
				if !ok {
					return
				}
				if evt.Err != nil {
					out <- HighlightedEvent{Timestamp: time.Now(), Path: evt.Path, Err: evt.Err}
					continue
				}
				match, matched := s.rules.Match(evt.Line)
				highlightEvt := HighlightedEvent{
					Timestamp: time.Now(),
					Path:      evt.Path,
					Line:      evt.Line,
					Severity:  rules.SeverityNormal,
				}
				if matched {
					if !s.showAll && !rules.MeetsThreshold(match.Rule.Severity, s.minSeverity) {
						continue
					}
					highlightEvt.RuleName = match.Rule.Name
					highlightEvt.Severity = match.Rule.Severity
					highlightEvt.Color = match.Rule.Color
					highlightEvt.Tags = match.Rule.Tags
					highlightEvt.Fragments = highlight.BuildFragments(evt.Line, match.HighlightSpans)
				} else {
					if !s.showAll {
						continue
					}
					highlightEvt.Fragments = []highlight.Fragment{{Text: evt.Line}}
				}
				out <- highlightEvt
			}
		}
	}()
	return out
}

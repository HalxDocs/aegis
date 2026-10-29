package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/HalxDocs/aegis/internal/causal"
	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/observability"
)

// loadHistory reads a recorded history from disk and checks it as a graph.
//
// A log is an artifact: it can be edited, truncated or written by an older
// build, so reading one is where the graph's rules are first asked to defend an
// explanation they did not build themselves. The refusal is deliberately worded
// as a verdict on the file rather than a stack trace, and it names the rule that
// broke, because the alternative — printing a plausible chain from a damaged
// log — is the one outcome this project cannot afford.
func loadHistory(path string) (*causal.Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read log: %w", err)
	}
	defer f.Close()

	events, err := observability.ReadJSONL(f)
	if err != nil {
		return nil, fmt.Errorf("log %s is not a readable history: %w", path, err)
	}
	index, err := causal.New(events)
	if err != nil {
		return nil, fmt.Errorf("log %s is not a causal history: %w", path, err)
	}
	return index, nil
}

// writeExplanation prints an event and the reasons it rests on, one level at a
// time: the event itself, then what caused it, then what caused that.
//
// Depth zero prints the whole chain. A branch that is cut off says so, because
// an explanation that stops silently is worse than one that admits it stopped.
func writeExplanation(w io.Writer, exp *causal.Explanation, maxDepth int) error {
	var walk func(node *causal.Explanation, level int) error
	walk = func(node *causal.Explanation, level int) error {
		prefix := ""
		if level > 0 {
			prefix = strings.Repeat("    ", level-1) + "<- "
		}
		if _, err := fmt.Fprintf(w, "%s%s\n", prefix, describeEvent(node.Event)); err != nil {
			return err
		}
		if maxDepth > 0 && level >= maxDepth && len(node.Causes) > 0 {
			_, err := fmt.Fprintf(w, "%s...\n", strings.Repeat("    ", level+1))
			return err
		}
		for _, cause := range node.Causes {
			if err := walk(cause, level+1); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(exp, 0)
}

// describeEvent renders one line of an explanation in the same order the table
// view uses: sequence, virtual time, kind, detail.
func describeEvent(ev domain.Event) string {
	return fmt.Sprintf("%d  %s  %s  %s", ev.Seq, ev.At.Duration(), ev.Kind, detail(ev))
}

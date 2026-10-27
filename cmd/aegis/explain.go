package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/HalxDocs/aegis/internal/causal"
	"github.com/HalxDocs/aegis/internal/domain"
)

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

// Command consumer is the proof that aegis can be used from outside its own
// module, kept in the repository so the claim is checked by `make api-check`
// rather than by a paragraph in a README.
//
// It is a separate module on purpose. A package inside the aegis module can
// import the root package even if that package only works because it reaches
// into internal ones; only a different module is subject to the rule that makes
// `internal` unreachable from outside. An example living in the same module
// would compile whether or not the exported surface was real.
//
// Two modes, because there are two ways to use this:
//
//	consumer                 weigh evidence this program already has
//	consumer run.jsonl       read a log the command wrote, and check its proofs
package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/HalxDocs/aegis"
)

func main() {
	if len(os.Args) > 1 {
		fail(reportProofs(os.Args[1]))
		return
	}
	fail(demonstrateGate())
}

// reportProofs reads a finished log and asks the log, not the run, whether each
// action it names was actually carried out.
func reportProofs(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	events, err := aegis.ReadLog(file)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	idx, err := aegis.NewIndex(events)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	proofs, err := aegis.VerifyActions(idx)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	fmt.Printf("%s: %d events, schema %s\n", path, len(events), aegis.EventLogSchema)
	if len(proofs) == 0 {
		fmt.Println("no recovery action was proposed")
		return nil
	}
	for _, proof := range proofs {
		_, complete, err := aegis.VerifyAction(idx, proof.Action)
		if err != nil {
			return err
		}
		fmt.Printf("action %d: proposed %d, started %d, applied %d (%s)\n",
			proof.Action, proof.Proposed, proof.Started, proof.Applied, state(complete))
	}
	return nil
}

// demonstrateGate weighs evidence the way a program that collected its own
// reports would: no simulator, no scenario, no log.
func demonstrateGate() error {
	gate, err := aegis.NewGate(aegis.Policy{
		Kind:         aegis.ActionRestartNode,
		MinWitnesses: 2,
		Patience:     aegis.TickFrom(50 * time.Millisecond),
		Delay:        aegis.TickFrom(100 * time.Millisecond),
	})
	if err != nil {
		return err
	}

	first := aegis.Witness{
		Report: aegis.FailureReport{
			Observer:    "A",
			Target:      "B",
			SuspectedAt: aegis.TickFrom(570 * time.Millisecond),
			LastSeenAt:  aegis.TickFrom(220 * time.Millisecond),
			Timeout:     aegis.TickFrom(350 * time.Millisecond),
			Silence:     aegis.TickFrom(350 * time.Millisecond),
		},
		Ref: 9,
	}
	second := aegis.Witness{
		Report: aegis.FailureReport{
			Observer:    "C",
			Target:      "B",
			SuspectedAt: aegis.TickFrom(575 * time.Millisecond),
			LastSeenAt:  aegis.TickFrom(225 * time.Millisecond),
			Timeout:     aegis.TickFrom(350 * time.Millisecond),
			Silence:     aegis.TickFrom(350 * time.Millisecond),
		},
		Ref: 10,
	}

	// One observer's silence proves only that it stopped hearing: refused.
	alone, err := gate.Decide([]aegis.Witness{first})
	if err != nil {
		return err
	}
	fmt.Println(alone)

	// Two independent observers agree, so the bar is cleared.
	both, err := gate.Decide([]aegis.Witness{first, second})
	if err != nil {
		return err
	}
	fmt.Println(both)

	// The same observer twice is still one observer: the gate refuses to count
	// it as corroboration, which is the failure mode that lets one wrong node
	// restart a healthy one.
	if _, err := gate.Decide([]aegis.Witness{first, first}); !errors.Is(err, aegis.ErrRepeatedObserver) {
		return fmt.Errorf("one observer counted twice: err = %v, want ErrRepeatedObserver", err)
	}
	fmt.Println("one observer repeated: refused as corroboration")
	return nil
}

func state(complete bool) string {
	if complete {
		return "proved"
	}
	return "incomplete"
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "consumer:", err)
		os.Exit(1)
	}
}

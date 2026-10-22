package domain_test

import (
	"errors"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func suspectedObservation(t *testing.T, lastSeen, suspectedAt domain.Tick) domain.Observation {
	t.Helper()
	obs := domain.NewObservation("A", "B")
	obs, err := obs.Observed(lastSeen)
	if err != nil {
		t.Fatalf("Observed: %v", err)
	}
	obs, err = obs.Suspect(suspectedAt)
	if err != nil {
		t.Fatalf("Suspect: %v", err)
	}
	return obs
}

func TestNewFailureReportCarriesEvidence(t *testing.T) {
	obs := suspectedObservation(t, tick(220), tick(570))

	report, err := domain.NewFailureReport(obs, tick(350))
	if err != nil {
		t.Fatalf("NewFailureReport = %v", err)
	}
	if report.Observer != "A" || report.Target != "B" {
		t.Fatalf("endpoints = %s -> %s", report.Observer, report.Target)
	}
	if report.SuspectedAt != tick(570) || report.LastSeenAt != tick(220) || report.Silence != tick(350) {
		t.Fatalf("evidence = %+v", report)
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
}

func TestNewFailureReportRejectsWeakEvidence(t *testing.T) {
	suspected := suspectedObservation(t, tick(220), tick(570))
	healthy, err := domain.NewObservation("A", "B").Observed(tick(220))
	if err != nil {
		t.Fatal(err)
	}
	early := suspectedObservation(t, tick(220), tick(500))

	cases := []struct {
		name    string
		obs     domain.Observation
		timeout domain.Tick
	}{
		{"zero timeout", suspected, 0},
		{"negative timeout", suspected, -1},
		{"healthy observation", healthy, tick(350)},
		{"silence shorter than timeout", early, tick(350)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := domain.NewFailureReport(tc.obs, tc.timeout); !errors.Is(err, domain.ErrInvalidReport) {
				t.Fatalf("error = %v, want ErrInvalidReport", err)
			}
		})
	}
}

func TestFailureReportValidateRejectsInconsistency(t *testing.T) {
	base := domain.FailureReport{
		Observer:    "A",
		Target:      "B",
		SuspectedAt: tick(570),
		LastSeenAt:  tick(220),
		Timeout:     tick(350),
		Silence:     tick(350),
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("baseline report invalid: %v", err)
	}

	cases := map[string]domain.FailureReport{
		"silence does not match": {
			Observer: "A", Target: "B", SuspectedAt: tick(570), LastSeenAt: tick(220), Timeout: tick(350), Silence: tick(351),
		},
		"silence below timeout": {
			Observer: "A", Target: "B", SuspectedAt: tick(500), LastSeenAt: tick(220), Timeout: tick(350), Silence: tick(280),
		},
		"self report": {
			Observer: "A", Target: "A", SuspectedAt: tick(570), LastSeenAt: tick(220), Timeout: tick(350), Silence: tick(350),
		},
		"zero timeout": {
			Observer: "A", Target: "B", SuspectedAt: tick(570), LastSeenAt: tick(220), Timeout: 0, Silence: tick(350),
		},
	}
	for name, report := range cases {
		t.Run(name, func(t *testing.T) {
			if err := report.Validate(); !errors.Is(err, domain.ErrInvalidReport) {
				t.Fatalf("Validate() = %v, want ErrInvalidReport", err)
			}
		})
	}
}

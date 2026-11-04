package observability_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/observability"
)

// TestReadJSONLRefusesAnotherFormat is the guard on the guard: the schema is
// only worth writing if something refuses to read past a version it does not
// understand. A loader that decoded a v2 file with v1 rules would produce events
// that parse, look familiar, and mean something else.
func TestReadJSONLRefusesAnotherFormat(t *testing.T) {
	cases := map[string]struct {
		log  string
		want error
	}{
		"a newer format": {
			log:  `{"seq":1,"at":0,"kind":"scenario_started","scenario":"x","schema":"aegis.eventlog/v2"}` + "\n",
			want: domain.ErrUnsupportedSchema,
		},
		"no format at all": {
			log:  `{"seq":1,"at":0,"kind":"scenario_started","scenario":"x"}` + "\n",
			want: domain.ErrUnsupportedSchema,
		},
		"an empty file": {
			log:  "",
			want: observability.ErrDamagedLog,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := observability.ReadJSONL(strings.NewReader(tc.log))
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestWrittenLogsNameTheirFormat checks the other direction: whatever the
// recorder is handed, the file it writes says what it is, once.
func TestWrittenLogsNameTheirFormat(t *testing.T) {
	rec := observability.NewRecorder()
	if _, err := rec.Record(domain.Event{Kind: domain.KindScenarioStarted, Scenario: "x"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if _, err := rec.Record(domain.Event{Kind: domain.KindNodeStarted, Node: "A", Causes: []domain.Ref{1}}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	var out bytes.Buffer
	if err := rec.WriteJSONL(&out); err != nil {
		t.Fatalf("WriteJSONL: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	if !strings.Contains(lines[0], `"schema":"`+domain.EventLogSchema+`"`) {
		t.Fatalf("the opening event does not name the format: %s", lines[0])
	}
	if strings.Contains(lines[1], "schema") {
		t.Fatalf("a later event repeated the format: %s", lines[1])
	}

	// A caller that names a different format is refused rather than corrected:
	// the recorder writes this build's format, and silently overwriting a claim
	// would hide a caller that meant something else.
	_, err := rec.Record(domain.Event{
		Kind: domain.KindScenarioStarted, Scenario: "y", Schema: "aegis.eventlog/v9",
	})
	if !errors.Is(err, domain.ErrUnsupportedSchema) {
		t.Fatalf("a foreign schema was accepted: %v", err)
	}
}

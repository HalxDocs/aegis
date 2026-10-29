package scenario

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// releaseLink matches a row that links a release, capturing the label and the
// tag in the URL. Both must agree: a row copied from a neighbour and relabelled
// by hand is the exact mistake this guard exists to catch.
var releaseLink = regexp.MustCompile(
	`\[(v[0-9]+\.[0-9]+\.[0-9]+)\]\([^()]*/releases/tag/(v[0-9]+\.[0-9]+\.[0-9]+)\)`)

// versionTag matches the tags the README is expected to account for. A tag with
// another shape is somebody's own bookmark and not a milestone.
var versionTag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

// releaseRows returns the label cell of every row in the README's Releases
// table, in file order.
func releaseRows(readme string) ([]string, error) {
	lines := strings.Split(readme, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "| release | what it covers |" {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, errors.New("README has no releases table: expected a row header \"| release | what it covers |\"")
	}

	var rows []string
	for _, line := range lines[start+1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			break
		}
		if strings.HasPrefix(trimmed, "| ---") {
			continue
		}
		cells := strings.Split(trimmed, "|")
		if len(cells) < 4 {
			return nil, fmt.Errorf("releases table row is not two cells: %q", trimmed)
		}
		rows = append(rows, strings.TrimSpace(cells[1]))
	}
	if len(rows) == 0 {
		return nil, errors.New("releases table has no rows")
	}
	return rows, nil
}

// checkReleases reports every disagreement between the README's releases table
// and the tags that exist. It is pure so the rules can be tested on fixed
// tables; TestReleaseTableMatchesTheTags applies it to the real pair.
func checkReleases(readme string, tags map[string]bool) []string {
	rows, err := releaseRows(readme)
	if err != nil {
		return []string{err.Error()}
	}

	var problems []string
	linked := make(map[string]bool, len(rows))
	for _, cell := range rows {
		match := releaseLink.FindStringSubmatch(cell)
		if match == nil {
			// An untagged milestone is a legitimate state, and the row must say
			// so rather than being silently skipped.
			if strings.Contains(cell, "(untagged)") {
				continue
			}
			problems = append(problems, fmt.Sprintf(
				"release row has neither a tag link nor an (untagged) label: %q", cell))
			continue
		}
		label, target := match[1], match[2]
		if label != target {
			problems = append(problems, fmt.Sprintf(
				"release row is labelled %s but links %s", label, target))
			continue
		}
		linked[label] = true
		if !tags[label] {
			problems = append(problems, fmt.Sprintf(
				"README lists %s but no such tag exists (publish it, or mark the row untagged)", label))
		}
	}

	for tag := range tags {
		if versionTag.MatchString(tag) && !linked[tag] {
			problems = append(problems, fmt.Sprintf(
				"tag %s has no row in the README releases table", tag))
		}
	}
	slices.Sort(problems)
	return problems
}

// localTags lists the repository's tags, and reports false when this checkout
// cannot answer the question: no git, no work tree, or a shallow clone whose
// tags were never fetched. A guard that failed in those cases would fail for a
// reason that has nothing to do with the README.
func localTags(t *testing.T) (map[string]bool, bool) {
	t.Helper()
	shallow, err := exec.Command("git", "-C", "../..", "rev-parse", "--is-shallow-repository").Output()
	if err != nil {
		return nil, false
	}
	if strings.TrimSpace(string(shallow)) == "true" {
		return nil, false
	}
	out, err := exec.Command("git", "-C", "../..", "tag", "--list").Output()
	if err != nil {
		return nil, false
	}
	tags := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		if tag := strings.TrimSpace(line); tag != "" {
			tags[tag] = true
		}
	}
	return tags, true
}

// TestReleaseTableMatchesTheTags keeps the README's claims about what has been
// published honest. The table went stale once already — it advertised the
// foundation milestone as untagged across two later releases — because nothing
// compared it with the tags that actually exist.
func TestReleaseTableMatchesTheTags(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	tags, ok := localTags(t)
	if !ok {
		t.Skip("tags are not available in this checkout: no git, not a work tree, or a shallow clone")
	}
	if len(tags) == 0 {
		t.Skip("no tags fetched: nothing to compare the table with")
	}
	for _, problem := range checkReleases(string(readme), tags) {
		t.Error(problem)
	}
}

func TestCheckReleases(t *testing.T) {
	const header = "| release | what it covers |\n| --- | --- |\n"
	row := func(cell string) string { return "| " + cell + " | text |\n" }
	linked := func(label, target string) string {
		return row(fmt.Sprintf("[%s](https://github.com/HalxDocs/aegis/releases/tag/%s) — Milestone", label, target))
	}

	cases := map[string]struct {
		readme string
		tags   map[string]bool
		want   []string
	}{
		"agrees": {
			readme: header + linked("v0.1.0", "v0.1.0") + linked("v0.2.0", "v0.2.0"),
			tags:   map[string]bool{"v0.1.0": true, "v0.2.0": true, "not-a-milestone": true},
			want:   nil,
		},
		"untagged row with no tags": {
			readme: header + row("Milestone 3 — causal history (untagged)"),
			tags:   map[string]bool{},
			want:   nil,
		},
		"row links a tag that does not exist": {
			readme: header + linked("v0.9.9", "v0.9.9"),
			tags:   map[string]bool{},
			want:   []string{"README lists v0.9.9 but no such tag exists (publish it, or mark the row untagged)"},
		},
		"tag has no row": {
			readme: header + linked("v0.1.0", "v0.1.0"),
			tags:   map[string]bool{"v0.1.0": true, "v0.2.0": true},
			want:   []string{"tag v0.2.0 has no row in the README releases table"},
		},
		"label and link disagree": {
			readme: header + linked("v0.1.0", "v0.2.0"),
			tags:   map[string]bool{"v0.1.0": true, "v0.2.0": true},
			// Neither tag counts as represented: the row is ambiguous about which
			// one it means, so the guard refuses to credit either.
			want: []string{
				"release row is labelled v0.1.0 but links v0.2.0",
				"tag v0.1.0 has no row in the README releases table",
				"tag v0.2.0 has no row in the README releases table",
			},
		},
		"row says neither": {
			readme: header + row("Milestone 3 — causal history"),
			tags:   map[string]bool{},
			want:   []string{`release row has neither a tag link nor an (untagged) label: "Milestone 3 — causal history"`},
		},
		"no table": {
			readme: "# Project\n\nNothing here.\n",
			tags:   map[string]bool{"v0.1.0": true},
			want:   []string{`README has no releases table: expected a row header "| release | what it covers |"`},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := checkReleases(tc.readme, tc.tags)
			if len(got) != len(tc.want) {
				t.Fatalf("problems = %q, want %q", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("problem %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

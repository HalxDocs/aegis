package scenario

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// releaseRowCells returns the cells of every row in the README's Releases
// table, in file order. Cells keep their surrounding pipe delimiters, so a row
// that was pasted with an extra column is reported rather than misread.
func releaseRowCells(readme string) ([][]string, error) {
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

	var rows [][]string
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
		rows = append(rows, cells)
	}
	if len(rows) == 0 {
		return nil, errors.New("releases table has no rows")
	}
	return rows, nil
}

// releaseRows returns the label cell of every row in the table, in file order.
func releaseRows(readme string) ([]string, error) {
	cells, err := releaseRowCells(readme)
	if err != nil {
		return nil, err
	}
	labels := make([]string, 0, len(cells))
	for _, row := range cells {
		labels = append(labels, strings.TrimSpace(row[1]))
	}
	return labels, nil
}

// markdownLink matches an inline link, capturing its target.
var markdownLink = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)

// linkTargets returns the relative paths a row points at. A scheme, a bare
// anchor and a query string are not paths in this repository, so they are not
// the guard's business.
func linkTargets(row []string) []string {
	var out []string
	for _, cell := range row {
		for _, match := range markdownLink.FindAllStringSubmatch(cell, -1) {
			target := match[1]
			if strings.Contains(target, "://") || strings.HasPrefix(target, "#") ||
				strings.HasPrefix(target, "mailto:") {
				continue
			}
			if cut := strings.IndexAny(target, "#?"); cut >= 0 {
				target = target[:cut]
			}
			target = strings.TrimPrefix(strings.TrimPrefix(target, "./"), "/")
			if target == "" {
				continue
			}
			out = append(out, target)
		}
	}
	return out
}

// checkReleaseDocLinks reports every relative link in the Releases table that
// points at a file the repository does not contain. A row is the only place a
// reader is told where to read next, so a dead link there costs more than the
// usual typo: it is the milestone's own account of itself, missing.
func checkReleaseDocLinks(readme string, exists func(string) bool) []string {
	rows, err := releaseRowCells(readme)
	if err != nil {
		return nil // the structural problem is checkReleases' to report
	}

	var problems []string
	for _, row := range rows {
		label := strings.TrimSpace(row[1])
		for _, target := range linkTargets(row) {
			if exists(target) {
				continue
			}
			problems = append(problems, fmt.Sprintf(
				"release row %q links %s, which does not exist", label, target))
		}
	}
	slices.Sort(problems)
	return problems
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
	for _, problem := range checkReleaseDocLinks(string(readme), repoFile) {
		t.Error(problem)
	}
}

// repoFile reports whether a path named in the README exists at the repository
// root. The guards run from internal/scenario, hence the two levels up.
func repoFile(path string) bool {
	_, err := os.Stat(filepath.Join("..", "..", filepath.FromSlash(path)))
	return err == nil
}

// TestCheckReleaseDocLinks covers the shapes a row's links actually take: the
// document beside the README, a nested path, an anchor on the same page, and the
// release URL itself, which is not this repository's file to check.
func TestCheckReleaseDocLinks(t *testing.T) {
	const header = "| release | what it covers |\n| --- | --- |\n"
	const tag = "[v0.1.0](https://github.com/HalxDocs/aegis/releases/tag/v0.1.0) — Milestone 1: node crash"

	exists := func(present ...string) func(string) bool {
		set := make(map[string]bool, len(present))
		for _, p := range present {
			set[p] = true
		}
		return func(path string) bool { return set[path] }
	}

	cases := map[string]struct {
		readme string
		exists func(string) bool
		want   []string
	}{
		"document exists": {
			readme: header + "| " + tag + " | The foundation. See [doc_m1.md](doc_m1.md). |\n",
			exists: exists("doc_m1.md"),
			want:   nil,
		},
		"document missing": {
			readme: header + "| " + tag + " | The foundation. See [doc_m1.md](doc_m1.md). |\n",
			exists: exists(),
			want: []string{
				`release row "[v0.1.0](https://github.com/HalxDocs/aegis/releases/tag/v0.1.0) — Milestone 1: node crash" links doc_m1.md, which does not exist`,
			},
		},
		"nested path and anchor": {
			readme: header + "| " + tag + " | See [notes](docs/notes.md#top) and [here](#releases). |\n",
			exists: exists("docs/notes.md"),
			want:   nil,
		},
		"external links are ignored": {
			readme: header + "| " + tag + " | See [the site](https://example.com/missing.md). |\n",
			exists: exists(),
			want:   nil,
		},
		"two dead links are both reported": {
			readme: header + "| " + tag + " | [a](a.md) and [b](b.md). |\n",
			exists: exists(),
			want: []string{
				`release row "[v0.1.0](https://github.com/HalxDocs/aegis/releases/tag/v0.1.0) — Milestone 1: node crash" links a.md, which does not exist`,
				`release row "[v0.1.0](https://github.com/HalxDocs/aegis/releases/tag/v0.1.0) — Milestone 1: node crash" links b.md, which does not exist`,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := checkReleaseDocLinks(tc.readme, tc.exists)
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

package scenario

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// goSources lists the Go files under root. Test files are optional because the
// rules here are about what ships, not about what a test may do.
func goSources(t *testing.T, root string, includeTests bool) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if !includeTests && strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if len(files) == 0 {
		t.Fatalf("no Go files found under %s", root)
	}
	return files
}

func parse(t *testing.T, path string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return file
}

func importPath(t *testing.T, spec *ast.ImportSpec) string {
	t.Helper()
	path, err := strconv.Unquote(spec.Path.Value)
	if err != nil {
		t.Fatalf("unquote import %s: %v", spec.Path.Value, err)
	}
	return path
}

// wallClockCalls are the standard-library entry points that would make a run
// depend on something other than virtual time. time.Duration and the unit
// constants are conversions, not clocks, so they are not in this set.
var wallClockCalls = map[string]bool{
	"Now":       true,
	"Sleep":     true,
	"After":     true,
	"AfterFunc": true,
	"Tick":      true,
	"NewTicker": true,
	"NewTimer":  true,
	"Since":     true,
	"Until":     true,
}

func TestSimulationNeverReadsTheWallClock(t *testing.T) {
	for _, path := range goSources(t, "..", false) {
		file := parse(t, path)
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "time" {
				return true
			}
			if wallClockCalls[sel.Sel.Name] {
				t.Errorf("%s calls time.%s: simulation time is virtual", path, sel.Sel.Name)
			}
			return true
		})
	}
}

func TestSimulationDoesNotUseMathRand(t *testing.T) {
	for _, path := range goSources(t, "..", false) {
		for _, spec := range parse(t, path).Imports {
			switch importPath(t, spec) {
			case "math/rand", "math/rand/v2":
				t.Errorf("%s imports %s: replay needs a generator whose stream is specified in this repository", path, spec.Path.Value)
			}
		}
	}
}

func TestDetectorImportsOnlyTheDomain(t *testing.T) {
	const domainImport = "github.com/HalxDocs/aegis/internal/domain"
	for _, path := range goSources(t, "../detector", false) {
		for _, spec := range parse(t, path).Imports {
			imported := importPath(t, spec)
			if !strings.Contains(strings.SplitN(imported, "/", 2)[0], ".") {
				continue // standard library
			}
			if imported != domainImport {
				t.Errorf("%s imports %s: the detector must not be able to reach ground truth", path, imported)
			}
		}
	}
}

func TestOnlyTheCompositionRootWiresBothHalves(t *testing.T) {
	const (
		simImport      = "github.com/HalxDocs/aegis/internal/sim"
		detectorImport = "github.com/HalxDocs/aegis/internal/detector"
	)
	for _, path := range goSources(t, "..", false) {
		var importsSim, importsDetector bool
		for _, spec := range parse(t, path).Imports {
			switch importPath(t, spec) {
			case simImport:
				importsSim = true
			case detectorImport:
				importsDetector = true
			}
		}
		if importsSim && importsDetector && !strings.HasPrefix(filepath.ToSlash(path), "../scenario/") {
			t.Errorf("%s wires the simulator and the detector together; only the scenario package may do that", path)
		}
	}
}

package main

import (
	"fmt"
	"io"
	"runtime/debug"

	"github.com/HalxDocs/aegis/internal/domain"
)

// runVersion prints what this binary is and what it reads.
//
// Both lines are needed and neither is decoration. The version says which build
// produced a log or which build a reader should install; the schema says whether
// that build speaks the same format as the file in front of it. A version alone
// leaves a reader to guess the second, and the guess is the one thing that must
// not be made: two formats can carry the same field names and mean different
// things by them.
func runVersion(args []string, stdout io.Writer) error {
	if len(args) > 0 {
		return fmt.Errorf("version takes no arguments, got %q", args[0])
	}
	_, err := fmt.Fprintf(stdout, "aegis %s\nschema %s\n", buildVersion(), domain.EventLogSchema)
	return err
}

// buildVersion names the build, from the module information the toolchain
// stamped into it.
//
// `go install github.com/HalxDocs/aegis/cmd/aegis@v0.4.0` records v0.4.0 there,
// so a released binary knows its own tag without a variable every release has to
// remember to set. A binary built from a checkout says so, with the commit it
// came from, because "devel" on its own would hide which devel.
func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "(unknown)"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var revision string
	var dirty bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			dirty = setting.Value == "true"
		}
	}
	if revision == "" {
		return "(devel)"
	}
	if len(revision) > 7 {
		revision = revision[:7]
	}
	if dirty {
		revision += "+dirty"
	}
	return "(devel " + revision + ")"
}

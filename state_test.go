package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFolderForKeepsAReachToOnePathElement(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reach string
		want  string
	}{
		{"a project reach is kept as it is", "project-a", "project-a"},
		{"no project is a folder of its own", "", noProject},
		{"separators cannot break out of the folder", "../../etc", "______etc"},
		{"a windows separator cannot either", `..\..\Windows`, "______Windows"},
		{"a dot folder is not one", ".", "_"},
		{"anything else unusable in a name goes too", "AMB P:1*", "AMB_P_1_"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := folderFor(tc.reach); got != tc.want {
				t.Errorf("folderFor(%q) = %q, want %q", tc.reach, got, tc.want)
			}
		})
	}
}

func TestStateAtPutsEachProjectUnderItsOwnFolder(t *testing.T) {
	home := "base"
	one := stateAt(home, "project-a")
	two := stateAt(home, "project-b")

	want := filepath.Join(home, "plugins", pluginName, "project-a")
	if one.dir != want {
		t.Errorf("dir = %q, want %q", one.dir, want)
	}
	if one.dir == two.dir {
		t.Errorf("two projects share the folder %q", one.dir)
	}
}

func TestStateAtWithNoHomeRemembersNothing(t *testing.T) {
	s := stateAt("", "project-a")

	if s.remembers() {
		t.Fatalf("remembers() = true with no %s set", envHome)
	}
	if got := s.lines("seen"); got != nil {
		t.Errorf("lines() = %v, want nil", got)
	}
	if err := s.setLines("seen", []string{"a"}); err == nil {
		t.Error("setLines() succeeded with nowhere to write")
	}
}

func TestStateFromEnvReadsWhatAmenboSets(t *testing.T) {
	t.Setenv(envHome, "base")
	t.Setenv(envReach, "project-c")

	if got, want := stateFromEnv().dir, stateAt("base", "project-c").dir; got != want {
		t.Errorf("dir = %q, want %q", got, want)
	}
}

func TestLinesComeBackAsTheyWereWritten(t *testing.T) {
	s := stateAt(t.TempDir(), "project-a")

	if got := s.lines("pending"); got != nil {
		t.Errorf("lines() before anything was written = %v, want nil", got)
	}
	if err := s.setLines("pending", []string{"one", "two", "three"}); err != nil {
		t.Fatalf("setLines() = %v", err)
	}
	if got, want := s.lines("pending"), []string{"one", "two", "three"}; !reflect.DeepEqual(got, want) {
		t.Errorf("lines() = %v, want %v", got, want)
	}
}

func TestSetLinesReplacesEverythingThatWasThere(t *testing.T) {
	s := stateAt(t.TempDir(), "project-a")

	if err := s.setLines("pending", []string{"one", "two", "three"}); err != nil {
		t.Fatalf("setLines() = %v", err)
	}
	if err := s.setLines("pending", []string{"four"}); err != nil {
		t.Fatalf("setLines() = %v", err)
	}
	if got, want := s.lines("pending"), []string{"four"}; !reflect.DeepEqual(got, want) {
		t.Errorf("lines() = %v, want %v", got, want)
	}
}

func TestSetLinesWithNothingEmptiesIt(t *testing.T) {
	s := stateAt(t.TempDir(), "project-a")

	if err := s.setLines("pending", []string{"one"}); err != nil {
		t.Fatalf("setLines() = %v", err)
	}
	if err := s.setLines("pending", nil); err != nil {
		t.Fatalf("setLines() = %v", err)
	}
	if got := s.lines("pending"); got != nil {
		t.Errorf("lines() = %v, want nil", got)
	}
}

func TestSetLinesLeavesNoTemporaryFileBehind(t *testing.T) {
	s := stateAt(t.TempDir(), "project-a")

	if err := s.setLines("pending", []string{"one"}); err != nil {
		t.Fatalf("setLines() = %v", err)
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		t.Fatalf("ReadDir() = %v", err)
	}
	for _, e := range entries {
		if e.Name() != "pending" {
			t.Errorf("%q was left beside the file it was written for", e.Name())
		}
	}
}

func TestLinesAnswersWithNothingWhenItCannotRead(t *testing.T) {
	s := stateAt(t.TempDir(), "project-a")

	// A directory where the file should be: readable as a name, never as lines.
	if err := os.MkdirAll(filepath.Join(s.dir, "seen"), 0o700); err != nil {
		t.Fatalf("MkdirAll() = %v", err)
	}
	if got := s.lines("seen"); got != nil {
		t.Errorf("lines() = %v, want nil", got)
	}
}

func TestOneProjectDoesNotReadAnothersLines(t *testing.T) {
	home := t.TempDir()
	one, two := stateAt(home, "project-a"), stateAt(home, "project-b")

	if err := one.setLines("pending", []string{"one's work"}); err != nil {
		t.Fatalf("setLines() = %v", err)
	}
	if got := two.lines("pending"); got != nil {
		t.Errorf("the other project read %v", got)
	}
}

func TestAReachCannotWriteOutsideThePluginsFolder(t *testing.T) {
	home := t.TempDir()
	s := stateAt(home, "../../elsewhere")

	if err := s.setLines("pending", []string{"one"}); err != nil {
		t.Fatalf("setLines() = %v", err)
	}
	under := filepath.Join(home, "plugins", pluginName)
	if !strings.HasPrefix(s.dir, under+string(filepath.Separator)) {
		t.Fatalf("dir = %q, which is outside %q", s.dir, under)
	}
}

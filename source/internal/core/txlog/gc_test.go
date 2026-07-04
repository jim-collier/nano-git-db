// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package txlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGCEligibility(t *testing.T) {
	// dates are lexical; cutoff "5" means "older than 5"
	entries := []Entry{
		// row A: deleted long ago -> all three collectible
		{TxID: "1", Date: "1", Table: "t", RowID: "aa", Op: "create"},
		{TxID: "2", Date: "2", Table: "t", RowID: "aa", Field: "f", Op: "update", NewValue: "x"},
		{TxID: "3", Date: "3", Table: "t", RowID: "aa", Op: "delete"},
		// row B: alive -> kept
		{TxID: "4", Date: "1", Table: "t", RowID: "bb", Op: "create"},
		// row C: deleted, then revived by a later create -> kept whole
		{TxID: "5", Date: "1", Table: "t", RowID: "cc", Op: "create"},
		{TxID: "6", Date: "2", Table: "t", RowID: "cc", Op: "delete"},
		{TxID: "7", Date: "3", Table: "t", RowID: "cc", Op: "create"},
		// row D: deleted RECENTLY -> kept for the audit window
		{TxID: "8", Date: "1", Table: "t", RowID: "dd", Op: "create"},
		{TxID: "9", Date: "7", Table: "t", RowID: "dd", Op: "delete"},
	}
	keep, collected := GC(entries, "5")
	if collected != 3 {
		t.Fatalf("collected = %d, want 3", collected)
	}
	for _, e := range keep {
		if e.RowID == "aa" {
			t.Fatalf("row aa should be gone: %+v", e)
		}
	}
	if len(keep) != 6 {
		t.Fatalf("keep = %d entries, want 6: %+v", len(keep), keep)
	}
}

func TestRotateAndMultiSegmentRead(t *testing.T) {
	lg, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := lg.Append(
		Entry{TxID: "1", Date: "1", Table: "t", RowID: "aa", Op: "create"},
		Entry{TxID: "2", Date: "2", Table: "t", RowID: "aa", Op: "delete"},
		Entry{TxID: "3", Date: "3", Table: "t", RowID: "bb", Op: "create", Field: "f", NewValue: "kept"},
	); err != nil {
		t.Fatal(err)
	}
	entries, _, err := lg.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	keep, collected := GC(entries, "9")
	if collected != 2 || len(keep) != 1 {
		t.Fatalf("collected=%d keep=%d", collected, len(keep))
	}
	seg, err := lg.Rotate(keep)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(seg), "txlog-") {
		t.Fatalf("segment name %q", seg)
	}
	if _, err := os.Stat(lg.Path()); !os.IsNotExist(err) {
		t.Fatal("live txlog.csv should be retired by the rotation")
	}

	// the segment alone serves reads; a new append recreates the live file
	got, warns, err := lg.ReadAll()
	if err != nil || len(warns) != 0 {
		t.Fatalf("readall after rotate: %v %v", warns, err)
	}
	if len(got) != 1 || got[0].RowID != "bb" || got[0].NewValue != "kept" {
		t.Fatalf("survivors = %+v", got)
	}
	if err := lg.Append(Entry{TxID: "4", Date: "4", Table: "t", RowID: "cc", Op: "create"}); err != nil {
		t.Fatal(err)
	}
	got, _, err = lg.ReadAll()
	if err != nil || len(got) != 2 {
		t.Fatalf("segment+live read = %+v err=%v", got, err)
	}
}

// A GC rotation must reach other clients through the normal sync (the whole
// dir is staged: new segment, deleted live file).
func TestRotationSyncs(t *testing.T) {
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	mustGit(t, base, "init", "--bare", remote)
	dirA := clone(t, base, remote, "a")

	lgA, err := Open(dirA)
	if err != nil {
		t.Fatal(err)
	}
	if err := lgA.Append(
		Entry{TxID: "1", Date: "1", Table: "t", RowID: "aa", Op: "create"},
		Entry{TxID: "2", Date: "2", Table: "t", RowID: "aa", Op: "delete"},
		Entry{TxID: "3", Date: "3", Table: "t", RowID: "bb", Op: "create"},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSyncer(lgA, 0).Sync(); err != nil {
		t.Fatal(err)
	}

	entries, _, _ := lgA.ReadAll()
	keep, _ := GC(entries, "9")
	if _, err := lgA.Rotate(keep); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSyncer(lgA, 0).Sync(); err != nil {
		t.Fatal(err)
	}

	dirB := clone(t, base, remote, "b")
	lgB, err := Open(dirB)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := lgB.ReadAll()
	if err != nil || len(got) != 1 || got[0].RowID != "bb" {
		t.Fatalf("client B after rotation sync = %+v err=%v", got, err)
	}
	if _, err := os.Stat(lgB.Path()); !os.IsNotExist(err) {
		t.Fatal("client B should have no live txlog.csv after the rotation")
	}
}

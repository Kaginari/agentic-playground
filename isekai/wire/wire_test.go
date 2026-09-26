package wire

import (
	"fmt"
	"strings"
	"testing"
)

func TestCommission(t *testing.T) {
	c := ParseCommission("@ROOT /w/x\n@SCOPE pkgs a, b\n@ASK draft +unsaid wire:raw\n@CAP 512\n@DUMP .isekai/tmp/out.md\n@SIZE 4000\nBuild the thing.\n\nSecond line.")
	if c.Root != "/w/x" || c.Scope != "pkgs a, b" || c.Ask != "draft" || !c.Unsaid || !c.Raw || c.Cap != 512 || c.Dump != ".isekai/tmp/out.md" || c.Size != 4000 || len(c.Body) != 2 {
		t.Fatalf("%+v", c)
	}
	if ParseCommission("just prose").CapOrDefault() != DefaultCap {
		t.Fatal("default cap")
	}
	back := ParseCommission(c.String())
	if back.Ask != "draft" || !back.Unsaid || !back.Raw || back.Cap != 512 || len(back.Body) != 2 {
		t.Fatalf("round trip %+v\n%s", back, c.String())
	}
}

func TestReport(t *testing.T) {
	r, ok := ParseReport("Some preamble.\n@S PASS 3 checks\n@F a.go:12 the fact\n@V claim holds — evidence\n@? what about x\n@U colony the colony knows\n@U untyped thing\n@T mind x — path — load≈10tok\n@E 123\n")
	if !ok || r.Status != "PASS 3 checks" || len(r.Findings) != 1 || len(r.Verdicts) != 1 || len(r.Holes) != 1 || len(r.Tools) != 1 || r.Bytes != 123 || len(r.Other) != 1 {
		t.Fatalf("%+v", r)
	}
	if r.Unsaid[0].Kind != "colony" || r.Unsaid[0].Text != "the colony knows" || r.Unsaid[1].Kind != "" || r.Unsaid[1].Text != "untyped thing" {
		t.Fatalf("unsaid %+v", r.Unsaid)
	}
	if kinds := r.SortedKinds(); strings.Join(kinds, ",") != ",colony" {
		t.Fatalf("kinds %v", kinds)
	}
	if _, ok := ParseReport("no envelope here"); ok {
		t.Fatal("no @S means not on the wire")
	}
	raw, ok := ParseReport("S|DONE\nF|a.go:3|one fact\n?|a hole\nE|40")
	if !ok || raw.Status != "DONE" || raw.Findings[0] != "a.go:3 — one fact" || raw.Holes[0] != "a hole" || raw.Bytes != 40 {
		t.Fatalf("raw %+v", raw)
	}
	if _, ok := ParseReport("@Something odd"); ok {
		t.Fatal("unknown tags are not @S")
	}
}

func TestEmitAndCap(t *testing.T) {
	r := Report{Status: "PASS", Findings: []string{"a.go:1 one"}, Holes: []string{"h"}, Unsaid: []Unsaid{{Kind: "law", Text: "l"}}}
	out := r.String()
	want := "@S PASS\n@? h\n@U law l\n@F a.go:1 one\n@E "
	if !strings.HasPrefix(out, want) {
		t.Fatalf("order: %q", out)
	}
	if !strings.HasSuffix(out, fmt.Sprint(len(out))) {
		t.Fatalf("@E must equal the byte length: %q", out)
	}
	back, _ := ParseReport(out)
	if back.Bytes != len(out) {
		t.Fatal("@E parses back to the length")
	}
	big := Report{Status: "PASS", Unsaid: []Unsaid{{Kind: "territory", Text: "kept"}}, Holes: []string{"kept too"}}
	for i := 0; i < 100; i++ {
		big.Findings = append(big.Findings, fmt.Sprintf("f%d.go:%d a finding of some length", i, i))
	}
	for _, cap := range []int{200, 500, 1000, 2048} {
		out := big.Emit(cap, ".isekai/tmp/full.md")
		if len(out) > cap {
			t.Fatalf("cap %d: %d bytes", cap, len(out))
		}
		if !strings.Contains(out, "@U territory kept") || !strings.Contains(out, "@? kept too") || !strings.Contains(out, "@? @CAP "+fmt.Sprint(cap)+": ") || !strings.Contains(out, "full report at .isekai/tmp/full.md") {
			t.Fatalf("cap %d: %q", cap, out)
		}
		if !strings.HasSuffix(out, fmt.Sprint(len(out))) {
			t.Fatalf("cap %d: @E wrong: %q", cap, out)
		}
	}
	if len(big.Emit(0, "")) < 3000 {
		t.Fatal("cap 0 means no ceiling")
	}
	tiny := Report{Status: "PASS"}.Emit(10, "")
	if !strings.HasPrefix(tiny, "@S PASS") {
		t.Fatalf("a report that cannot fit still opens with @S: %q", tiny)
	}
}

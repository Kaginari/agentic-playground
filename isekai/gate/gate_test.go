package gate

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Kaginari/agentic-playground/isekai/tool"
)

func TestNeeds(t *testing.T) {
	g := New()
	if g.Needs(tool.Read) || g.Needs(tool.Write) || !g.Needs(tool.Outward) || !g.Needs(tool.Destructive) {
		t.Fatal("outward and destructive always need the gate")
	}
	g.Strict = true
	if !g.Needs(tool.Write) || g.Needs(tool.Read) {
		t.Fatal("strict gates writes")
	}
}

func TestAsk(t *testing.T) {
	g := New()
	g.IsTTY = func() bool { return false }
	a := g.Ask(Request{ID: "s1", Tool: "bash", Class: tool.Outward, Why: "network", Summary: "curl x"})
	if a.Decision != Denied || a.By != "no-tty" || !strings.Contains(a.Why, "--approve outward") {
		t.Fatalf("no tty: %+v", a)
	}
	g.Flag = "-approve"
	if a = g.Ask(Request{Class: tool.Destructive}); !strings.Contains(a.Why, "-approve destructive") {
		t.Fatalf("flag name is configurable: %+v", a)
	}
	if a = g.Ask(Request{Class: tool.Read}); a.Decision != NotNeeded || a.Needed {
		t.Fatalf("read: %+v", a)
	}
	var err error
	if g.Approve, err = ParseApprove(" outward , destructive"); err != nil {
		t.Fatal(err)
	}
	if g.Ask(Request{Class: tool.Outward}).By != "pre-approved" || g.Ask(Request{Class: tool.Destructive}).Decision != Approved {
		t.Fatal("pre-approval")
	}
	g.Approve = nil
	g.DryRun = true
	if a = g.Ask(Request{Class: tool.Outward}); a.Decision != WouldAsk || a.By != "dry-run" {
		t.Fatalf("dry run: %+v", a)
	}
	if _, err := ParseApprove("all"); err == nil {
		t.Fatal("unknown class")
	}
	if m, err := ParseApprove(""); err != nil || len(m) != 0 {
		t.Fatal("empty list")
	}
}

func TestTTY(t *testing.T) {
	var out bytes.Buffer
	g := New()
	g.IsTTY = func() bool { return true }
	g.Out = &out
	g.In = strings.NewReader("yes\n")
	a := g.Ask(Request{ID: "s2", Tool: "bash", Class: tool.Destructive, Why: "rm", Summary: "rm -rf build"})
	if a.Decision != Approved || a.By != "tty" {
		t.Fatalf("yes: %+v", a)
	}
	if !strings.Contains(out.String(), "gate s2 — destructive (rm)") || !strings.Contains(out.String(), "rm -rf build") || !strings.Contains(out.String(), "[y/N]") {
		t.Fatalf("prompt: %q", out.String())
	}
	for _, ans := range []string{"n\n", "\n", "maybe\n", ""} {
		g.In = strings.NewReader(ans)
		if g.Ask(Request{Class: tool.Outward}).Decision != Denied {
			t.Fatalf("%q should deny", ans)
		}
	}
	g.Prompt = func(r Request) string { return "custom? " }
	out.Reset()
	g.In = strings.NewReader("y\n")
	g.Ask(Request{Class: tool.Outward})
	if out.String() != "custom? " {
		t.Fatalf("custom prompt: %q", out.String())
	}
}

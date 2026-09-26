package instrument

import (
	"strings"
	"testing"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/provider"
)

func TestReading(t *testing.T) {
	b := Budget{}
	c := b.Reading(provider.Usage{Input: 1000, CacheRead: 185000, Output: 99})
	if !c.Available || c.Tokens != 186000 || c.Limit != DefaultBudget || c.Stress != DefaultStress || c.Zone != STRESS || !c.Stressed() || c.Percent() != 93 {
		t.Fatalf("%+v", c)
	}
	if !strings.Contains(c.String(), "186,000 / 200,000 tokens (93%)") || !strings.Contains(c.String(), "STRESS ZONE") {
		t.Fatalf("%s", c)
	}
	if z := b.Reading(provider.Usage{Input: 135000}).Zone; z != NEAR {
		t.Fatalf("near: %s", z)
	}
	if z := b.Reading(provider.Usage{Input: 134999}).Zone; z != OK {
		t.Fatalf("ok: %s", z)
	}
	small := Budget{Limit: 1000, Stress: 900}
	if c := small.Reading(provider.Usage{Input: 950}); c.Zone != STRESS || c.Limit != 1000 {
		t.Fatalf("custom budget %+v", c)
	}
	u := b.Unread("no provider call yet")
	if u.Available || u.Zone != UNREAD || u.Stressed() || !strings.Contains(u.String(), "unmeasured") {
		t.Fatalf("%+v", u)
	}
}

func TestStatusLine(t *testing.T) {
	s := Status{Provider: "mock", Body: "slime-x", Turns: 2, Steps: 3, Elapsed: 1500 * time.Millisecond, Spend: provider.Usage{Input: 1234, Output: 56}, Context: Budget{}.Reading(provider.Usage{Input: 1234}), Journal: "j.jsonl"}
	l := s.Line()
	for _, want := range []string{"slime-x", "mock", "turns 2", "steps 3", "2s", "spend in 1,234 out 56", "context 1,234 / 200,000", "journal j.jsonl"} {
		if !strings.Contains(l, want) {
			t.Fatalf("missing %q in %q", want, l)
		}
	}
	if group(999) != "999" || group(1000) != "1,000" || group(1234567) != "1,234,567" {
		t.Fatal("group")
	}
}

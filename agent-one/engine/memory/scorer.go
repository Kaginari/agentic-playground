package memory

// The local scorer: BM25 over a real token vocabulary, normalised to 0..1 as the share of the
// question's ideal match (memory.js's scorer, unchanged). Term order is kept as first-seen so
// floating-point sums land on the same bits as the JS.

import (
	"math"
	"strings"
	"time"
)

var stop = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields("a an and are as at be by for from has have if in into is it its of on or that the this to was were will with not no " +
		"you your we our they their he she i me my but so than then there these those which who what when where how can could would should " +
		"do does did done been being also any all each every more most much very just only over under up down out off about after before " +
		"again here now one two three") {
		m[w] = true
	}
	return m
}()

func tokChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '@' || c == '.' || c == '-'
}

// Tokens lower-cases, splits on anything outside [a-z0-9@.-], strips leading/trailing . and -,
// and drops one-char tokens, stop words and pure numbers.
func Tokens(text string) []string {
	s := strings.ToLower(text)
	var out []string
	emit := func(t string) {
		t = strings.Trim(t, ".-")
		if len(t) <= 1 || stop[t] {
			return
		}
		digits := true
		for i := 0; i < len(t); i++ {
			if t[i] < '0' || t[i] > '9' {
				digits = false
				break
			}
		}
		if digits {
			return
		}
		out = append(out, t)
	}
	start := -1
	for i := 0; i < len(s); i++ {
		if tokChar(s[i]) {
			if start < 0 {
				start = i
			}
		} else if start >= 0 {
			emit(s[start:i])
			start = -1
		}
	}
	if start >= 0 {
		emit(s[start:])
	}
	return out
}

// TF holds a document's term counts with the terms in first-seen order.
type TF struct {
	Map   map[string]int
	Order []string
	DL    int
}

func TermCounts(text string) *TF {
	tf := &TF{Map: map[string]int{}}
	for _, t := range Tokens(text) {
		if _, ok := tf.Map[t]; !ok {
			tf.Order = append(tf.Order, t)
		}
		tf.Map[t]++
		tf.DL++
	}
	return tf
}

const (
	k1 = 1.2
	b  = 0.75
)

func IDFFor(n, df float64) float64 { return math.Log((n-df+0.5)/(df+0.5) + 1) }

// Stats are corpus statistics: the index stores them, a shared-only pool computes them live.
type Stats struct {
	N     int
	AvgDL float64
	IDF   map[string]float64
}

// Doc is anything with term counts (a memory, a toolbox entry).
type Doc interface{ Terms() (map[string]int, int) }

func StatsOf(docs []Doc) Stats {
	df := map[string]int{}
	total := 0
	for _, d := range docs {
		tf, dl := d.Terms()
		total += dl
		for t := range tf {
			df[t]++
		}
	}
	idf := make(map[string]float64, len(df))
	for t, n := range df {
		idf[t] = IDFFor(float64(len(docs)), float64(n))
	}
	avg := 1.0
	if len(docs) > 0 {
		avg = float64(total) / float64(len(docs))
	}
	return Stats{N: len(docs), AvgDL: avg, IDF: idf}
}

// IDFQ answers a query term's idf: the corpus value, else the rarest (an unseen term).
func (s Stats) IDFQ(t string) float64 {
	if v, ok := s.IDF[t]; ok {
		return v
	}
	n := float64(s.N)
	if n == 0 {
		n = 1
	}
	return IDFFor(n, 1)
}

// BM25 is the share of the question's ideal match (every term saturated), 0..1.
func BM25(q *TF, tf map[string]int, dl int, avgdl float64, idfQ func(string) float64) float64 {
	if avgdl == 0 || avgdl != avgdl {
		avgdl = 1
	}
	s, ideal := 0.0, 0.0
	K := k1 * (1 - b + b*float64(dl)/avgdl)
	for _, t := range q.Order {
		idf := idfQ(t)
		ideal += idf * (k1 + 1)
		if f, ok := tf[t]; ok && f != 0 {
			s += idf * float64(f) * (k1 + 1) / (float64(f) + K)
		}
	}
	if ideal == 0 {
		return 0
	}
	return s / ideal
}

// Vec is a unit tf·idf vector of a question — only used to match questions to questions.
type Vec struct {
	Map   map[string]float64
	Order []string
}

func QVec(q *TF, idfQ func(string) float64) *Vec {
	v := &Vec{Map: map[string]float64{}, Order: q.Order}
	n := 0.0
	for _, t := range q.Order {
		w := float64(q.Map[t]) * idfQ(t)
		v.Map[t] = w
		n += w * w
	}
	n = math.Sqrt(n)
	if n == 0 {
		n = 1
	}
	for _, t := range q.Order {
		v.Map[t] /= n
	}
	return v
}

func (v *Vec) OJ() OJ {
	o := make(OJ, 0, len(v.Order))
	for _, t := range v.Order {
		o = append(o, KV{t, v.Map[t]})
	}
	return o
}

func Cosine(a *Vec, b map[string]float64) float64 {
	d := 0.0
	for _, t := range a.Order {
		if w, ok := b[t]; ok && w != 0 {
			d += a.Map[t] * w
		}
	}
	return d
}

// JSDateParse mirrors Date.parse for the ISO forms the workspace writes: a date-only string is UTC,
// a date-time without an offset is local time.
func JSDateParse(s string) (time.Time, bool) {
	for _, l := range []string{"2006-01-02T15:04:05.999999999Z07:00", "2006-01-02T15:04:05Z07:00", "2006-01-02T15:04Z07:00"} {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	for _, l := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05", "2006-01-02T15:04"} {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return t, true
		}
	}
	for _, l := range []string{"2006-01-02", "2006-01", "2006"} {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// Recency is the light prior on dated memories: +0.05 today, half at 30 days, ~0 past a season.
func Recency(when string, now time.Time) float64 {
	if when == "" {
		return 0
	}
	t, ok := JSDateParse(strings.Replace(JSTrim(when), " ", "T", 1))
	if !ok {
		return 0
	}
	age := float64(now.UnixMilli()-t.UnixMilli()) / 864e5
	return 0.05 * math.Exp(-math.Max(0, age)/43.3)
}

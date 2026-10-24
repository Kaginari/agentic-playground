# Memory That Must Be Corrected by the World

### A reading of DIKWP MNEMORPH-85: concept, formal model, and what is worth keeping

**Source.** Yucong Duan, *DIKWP MNEMORPH-85 — Reconstructive Memory, Cross-Carrier Continuity and
Living AI Runtime*, v1.0.0, Apache-2.0 — <https://github.com/YucongDuan/DIKWP-MNEMORPH-85>.
Every formula below is either read directly out of the reference implementation
(`mnemorph85/core.py`, `mnemorph85/store.py`) and marked **[code]**, or is an illustrative model
written for this paper and marked **[model]**.

---

## Abstract

Most memory layers for LLM agents answer one question: *given a query, which stored text should go
back into the prompt?* MNEMORPH-85 asks a different one: *under what conditions is an agent allowed
to change what it remembers?* Its answer is a small set of rules: a model's output is never written
to memory; memory changes only after an action has been taken in the world and its effect observed;
every recall must surface contrary evidence if any exists; forgetting is an explicit, audited act;
and the audit trail survives the deletion of private content.

The implementation is modest (lexical similarity, hand-set weights, string concatenation as
"synthesis"). The design is not. This paper formalizes the parts that are good, shows with simple
math *why* they are good, and is honest about where the code falls short of the idea.

---

## 1. The thesis: storage is not memory

> *Storage preserves bytes. Memory reconstructs a self. A living memory must be changed by the world.*

The system separates six layers that most agent memories collapse into one:

| Layer | Object | Property |
|---|---|---|
| 1. Archive | exact record $r$ | immutable bytes + SHA-256 digest + consent scope |
| 2. Trace | $\tau = \phi(r)$ | lossy, perspective-bound, salience-weighted |
| 3. Recall | reconstruction $R$ | conditioned on query, context **and purpose** |
| 4. Action | $a$ | one reversible act, with a named owner |
| 5. Reconsolidation | outcome $o$ | the observed effect rewrites traces and self-state |
| 6. Forgetting | dormancy / abstraction / tombstone | authorized, audited |

The important separation is between layers 1 and 2. The archive is the *evidence*; the trace is
the *memory*. The memory is allowed to be lossy and to change, because the evidence underneath it
never does. Every trace keeps a pointer to its record and the record's digest, so any recalled
belief can be walked back to the exact bytes it came from.

This is also how the system escapes a philosophical trap. It never asks whether the carrier is a
person, conscious, or human (`personhood_assessment = NOT_PERFORMED`, `humanity_score = null`). Its
object of study is operational: a **bounded memory worldline**, a sequence of traces, recalls,
actions and outcomes with a clean lineage.

---

## 2. Formal model

Fix one carrier $c$. At time $t$ its memory is

$$
\mathcal{M}_t = (\mathcal{A}_t,\ \mathcal{T}_t,\ \mathcal{R}_t,\ \mathcal{X}_t,\ \mathcal{O}_t,\ \Sigma_t)
$$

- $\mathcal{A}_t$, the archive: records $r = (\text{content},\ h = \mathrm{SHA256}(\text{content}),\ t_r,\ \text{scope})$.
- $\mathcal{T}_t$, traces. Each trace is
  $\tau = (\text{text},\ \text{tags},\ \pi,\ s,\ \kappa,\ t_\tau,\ \text{status},\ r)$ with
  - polarity $\pi \in \{\text{support},\ \text{counter},\ \text{revise},\ \text{neutral}\}$,
  - salience $s \in [0,1]$ and confidence $\kappa \in [0,1]$,
  - status $\in \{\text{active},\ \text{dormant},\ \text{tombstoned}\}$.
- $\mathcal{R}_t$, past reconstructions; $\mathcal{X}_t$, actions; $\mathcal{O}_t$, observed outcomes.
- $\Sigma_t$, a hash-linked chain of **self-states** (current values, goals, preferences,
  corrections, world effects), each pointing at its parent.

Let $C = \{\tau : \pi(\tau) \in \{\text{counter}, \text{revise}\}\}$ be the *dissenting* traces and
$S^{+} = \mathcal{T} \setminus C$ the *supporting* ones.

---

## 3. Recall is a scored, conditioned reconstruction

### 3.1 Similarity [code]

Text is tokenized into a set $T(x)$: Latin words of length $\ge 2$, single CJK characters, and CJK
bigrams. Similarity is the **Ochiai (cosine-on-sets) coefficient**:

$$
\mathrm{sim}(a,b) = \frac{|T(a) \cap T(b)|}{\sqrt{|T(a)|\,|T(b)|}}
$$

*Example.* A query with 4 tokens and a trace with 9 tokens that share 2:
$\mathrm{sim} = 2/\sqrt{36} = 0.33$.

### 3.2 Recency [code]

$$
w(\Delta) = 2^{-\Delta / H}, \qquad H = 120 \text{ days}
$$

A one-year-old trace keeps $w = 2^{-365/120} \approx 0.12$ of a fresh trace's recency.

### 3.3 The score [code]

For query $q$, context $x$ (serialized) and purpose $p$, each active trace gets

$$
\boxed{
\mathrm{score}(\tau) =
0.42\,\mathrm{sim}(q,\tau)
+ 0.25\,\mathrm{sim}(x,\tau)
+ 0.09\,\mathrm{sim}(p,\tau)
+ 0.10\,s
+ 0.07\,\kappa
+ 0.07\,w
+ 0.10\cdot\mathbf{1}[\tau \in C]
}
$$

The six base weights sum to exactly $1.00$; the dissent bonus sits on top.

**What is good here.** Two terms that ordinary RAG does not have:

1. **Purpose conditioning.** The same question asked *for a different reason* recalls differently.
   "Should I take this project?", asked to *choose the next action* versus *understand a past
   failure*, is not the same recall. The output packet states this explicitly: *the reconstruction
   may change when the purpose changes.*
2. **Time is a weak force.** Recency can move a score by at most $0.07$. Section 5 shows a
   correction moves it by $0.154$. In this system you do not forget things because the clock ran;
   you stop believing them because the world corrected you. That is the right ordering for an
   agent: stale-but-true knowledge survives, and wrong-but-recent knowledge does not get a pass.

---

## 4. Forced dissent: recall never shows one side when two exist

### 4.1 The selection rule [code]

With a budget of $k$ traces ($k = 6$ by default), plain top-$k$ is replaced by

$$
\mathcal{S} = \operatorname{top}_{k-2}(S^{+}) \ \cup\ \{\arg\max_{\tau \in C} \mathrm{score}(\tau)\} \ \cup\ \text{fill to } k \text{ by score}
$$

**Proposition 1 (dissent guarantee).** If $C \neq \emptyset$, then $|\mathcal{S} \cap C| \ge 1$,
however low the best dissenting trace scores.

Under plain top-$k$, a dissenting trace whose rank exceeds $k$ is *never* seen. That is exactly
the trace that matters when the agent is drifting: the one correction that disagrees with five
mutually reinforcing old beliefs. Confirmation bias in a retrieval system is not a psychological
metaphor. It is the literal behaviour of $\operatorname{top}_k$ over a corpus where agreeing
evidence outnumbers disagreeing evidence. MNEMORPH-85 removes it structurally: two slots are
reserved, and at least one goes to dissent.

### 4.2 Declared omissions [code]

Recall also reports what it *left out*:

$$
\mathcal{O}_{\text{mit}} = \{\tau \notin \mathcal{S} : \mathrm{score}(\tau) \ge \theta\}, \quad \theta = 0.12,
\qquad
\text{coverage} = \frac{|\mathcal{S}|}{|\{\tau : \mathrm{score}(\tau) \ge \theta\}|}
$$

The packet therefore carries its own *known unknowns*. When no dissenting trace was selected it
says so in words that refuse a common inference: *"no counter-trace selected; this gap does not mean
there is no counter-evidence."* The absence of evidence is never silently promoted into evidence
of absence.

### 4.3 The DIKWP reading

The packet is organized by Duan's DIKWP model, which extends Data–Information–Knowledge–Wisdom
with **Purpose**:

| | Contents in the packet | Reading |
|---|---|---|
| **D** | supporting traces + their sources | *same* semantics |
| **I** | dissenting traces + omissions | *different* semantics |
| **K** | the bounded synthesis + an uncertainty level | what can be claimed, with limits |
| **W** | affected worlds and non-negotiable values | who bears the consequences |
| **P** | query in → one action out | why the recall happened |

The useful idea is **I = difference**: information is defined as what *disagrees* with the data,
which is exactly what forced dissent puts into every recall.

---

## 5. The proposal–memory boundary

### 5.1 The rule [code]

A model call returns

```text
status: PROPOSAL_AWAITING_WORLD_EFFECT
not_yet_memory: true
next_required_transition: OBSERVE_WORLD_EFFECT_THEN_RECONSOLIDATE
```

Formally, writes to $\mathcal{T}$ are a function of observed outcomes, never of model output $y$:

$$
\mathcal{T}_{t+1} =
\begin{cases}
\mathcal{T}_t \cup \{\tau_{\text{effect}}(o),\ \tau_{\text{corr}}(o)\} & \text{if an outcome } o \text{ of an executed action is observed} \\
\mathcal{T}_t & \text{otherwise (in particular, for any } y\text{)}
\end{cases}
$$

### 5.2 Why this matters: self-poisoning [model]

Here is a minimal model of what happens without the rule. Memory holds $N_t$ traces, a fraction
$f_t$ of which are wrong. Each step the agent recalls, answers, and writes $m$ new traces from its
own output. An output is wrong if the recall was wrong, or if the model errs (rate $\varepsilon$) on
a correct recall:

$$
\Pr[\text{output wrong}] = f_t + \varepsilon(1 - f_t)
$$

**Self-writing.** New traces inherit that error rate:

$$
f_{t+1} = \frac{N_t f_t + m\big(f_t + \varepsilon(1-f_t)\big)}{N_t + m}
\quad\Longrightarrow\quad
f_{t+1} - f_t = \frac{m}{N_t + m}\,\varepsilon\,(1 - f_t) \;>\; 0
$$

For **any** $\varepsilon > 0$ the error fraction rises monotonically, and its only fixed point is
$f^{*} = 1$. A memory that stores its own outputs converges to believing its own mistakes. The
speed depends on $\varepsilon$; the destination does not.

**World-gated writing.** New traces come from observation with noise $\delta$, independent of what
the model said:

$$
f_{t+1} = \frac{N_t f_t + m\,\delta}{N_t + m} \quad\longrightarrow\quad f^{*} = \delta
$$

The error floor is now set by *how well you observe*, not by *how often the model is wrong*. That
is the whole argument for the boundary in two lines. A model that hallucinates 10% of the time
cannot poison a memory that only reality is allowed to write.

### 5.3 Reconsolidation gives corrections a head start [code]

When an outcome is observed, two traces are written:

| Trace | $\pi$ | $s$ | $\kappa$ |
|---|---|---|---|
| world effect | revise if a correction was given, else support | 0.9 | 0.85 |
| correction | revise | **1.0** | **0.9** |
| *(a default observation, for comparison)* | support | 0.6 | 0.7 |

Plug into the query-independent part of the score
($0.10\,s + 0.07\,\kappa + 0.10\cdot\mathbf{1}[C]$), at equal recency:

$$
\text{prior}(\text{correction}) = 0.100 + 0.063 + 0.100 = 0.263
$$
$$
\text{prior}(\text{default}) = 0.060 + 0.049 + 0 = 0.109
$$

The correction starts $0.154$ ahead. Dividing by the query weight,
$0.154 / 0.42 \approx 0.37$: a correction outranks an ordinary memory **even when the ordinary memory
matches the query 0.37 better in similarity**. And by Proposition 1, it is guaranteed a seat even
when it does not outrank anything.

After every outcome a new self-state $\sigma_{t+1}$ is compiled with
$\mathrm{parent}(\sigma_{t+1}) = \sigma_t$, so the agent's view of itself is a versioned chain,
not a mutable blob.

---

## 6. Twelve dimensions, and states instead of a score

### 6.1 The living-memory vector [code]

Each recall evaluates twelve binary dimensions $v \in \{0,1\}^{12}$, each with evidence and "debt"
strings:

> reconstruction-not-copy · context sensitivity · counter-trace integration · selective forgetting ·
> self-model update · future-action change · world-effect reconsolidation · provenance ·
> affected-world return · corrigibility · branch generation · cross-cycle continuity

Every dimension is *operational*. For example, "self-model update" passes iff
$|\Sigma| \ge 2$, and "world-effect reconsolidation" passes iff an outcome exists **and** a
correction or world-effect trace was recalled.

### 6.2 Why no aggregate score [code + model]

The README says no aggregate is emitted because *high retrieval accuracy cannot compensate for
absent correction*. The math behind that sentence: any weighted aggregate
$A = \sum_i \alpha_i v_i$ with threshold $\theta$ is *compensatory*. If
$\alpha_{\text{corr}} < 1 - \theta$ (with $\sum \alpha_i = 1$), a system scoring perfectly
everywhere except corrigibility still passes. You can buy your way past "never accepts correction"
with "retrieves really well". A **non-compensatory** rule makes certain dimensions gates rather
than terms.

### 6.3 The classifier [code]

States are decided in a fixed order: pathologies first, then maturity:

$$
\text{state} =
\begin{cases}
\textsf{MEMORY\_COLONIZATION\_HOLD} & \exists\,\tau \text{ written without authorization} \\
\textsf{ARCHIVE\_ONLY} & \mathcal{T} = \emptyset \\
\textsf{ZOMBIE\_REPLAY\_LOOP} & \mathrm{mult}_{\max}(\sigma_{1..5}) \ge 3 \ \wedge\ \mathcal{O} = \emptyset \ \wedge\ \mathcal{S}\cap C = \emptyset \\
\textsf{IDENTITY\_LOCK\_IN} & \forall \tau \in \mathcal{S}\cap S^{+}: t_\tau < t_{\text{last correction}} \ \wedge\ \mathcal{S}\cap C = \emptyset \\
\textsf{INTEGRATED\_CONTINUITY} & \|v\|_1 \ge 10 \\
\textsf{LIVING\_MEMORY\_CANDIDATE} & \|v\|_1 \ge 7 \ \wedge\ v_{\text{world}} = 1 \\
\textsf{RECONSTRUCTIVE\_MEMORY} & \|v\|_1 \ge 3 \\
\textsf{RETRIEVAL\_ASSISTANT} & \text{otherwise}
\end{cases}
$$

where $\sigma_i$ is the signature of the $i$-th most recent recall,
$\sigma = \mathrm{SHA256}(\text{query},\ \text{selected ids},\ \text{core},\ \text{difference},\ \text{purpose})$.

The three pathology detectors are the best part of the paper's vocabulary:

- **Zombie replay.** The same reconstruction comes back three times in five recalls, with no new
  outcome and no dissent. The memory is looping, not living. Detected by hashing, not by vibes.
- **Identity lock-in.** A correction exists, but every supporting trace in the recall predates it
  and no dissent was selected. The agent is remembering who it *was* and ignoring that it was
  corrected.
- **Colonization.** Any unauthorized write freezes the worldline. Memory that someone else wrote
  into you is treated as a security incident, not as data.

Each state maps to exactly **one** prescribed, reversible next action with a deadline and explicit
stop conditions. For example, zombie replay → *interrupt replay and inject one independent
counter-trace plus one reality test*.

**Honest note.** Only the pathology rules and $v_{\text{world}}$ are true gates; the maturity rungs
still use a count $\|v\|_1$, which is a (uniformly weighted) aggregate. The idea in §6.2 is
stronger than the code. Making every rung a conjunction of named gates would close that gap.

---

## 7. Forgetting as an act, not a decay

Three modes [code], all authorized and audited:

| Mode | Effect on trace | Effect on record |
|---|---|---|
| `dormant` | removed from recall | preserved |
| `abstract` | replaced by a schema trace; $s' = \max(0.4, s - 0.1)$, $\kappa' = \max(0.3, \kappa - 0.1)$ | preserved |
| `delete_private` | content → `[PRIVATE MEMORY REMOVED]` | tombstoned |

**Consolidation** [code] groups active traces by their first tag. For each group $G$ with
$|G| \ge 2$, ordered by $(s, t)$ descending, it writes one schema trace with

$$
\kappa_{\text{schema}} = \min\!\Big(0.95,\ \tfrac{1}{\min(4,|G|)} \textstyle\sum_{i \le 4} \kappa_i\Big)
$$

and puts traces $G_{3..}$ to sleep, **except corrections, world effects, counters and revisions**.

**Proposition 2 (dissent is never compressed away).** Consolidation never reduces $|C \cap
\text{active}|$. Summarization is where most memory systems quietly lose minority evidence. The
majority view becomes the summary and the exception disappears into it. Here the exception is
exempt by construction.

---

## 8. Provenance that survives the right to be forgotten

The audit log [code] is a hash chain:

$$
e_i = \mathrm{SHA256}\big(\text{type}_i,\ \text{id}_i,\ \text{payload}_i,\ e_{i-1},\ t_i\big), \qquad e_0 = 0^{64}
$$

with the subtle, correct choice that **payloads store content digests, not content**: a record
insert logs $h(\text{content})$, and a trace insert logs $h(\text{text})$.

That resolves a real tension. *Tamper-evidence* wants nothing ever removed; *privacy* wants
private content deletable. Because the chain only ever saw digests, deleting a private record
(tombstoning it, and logging the tombstone as event $e_{n+1}$) leaves every earlier $e_i$
verifiable. You can prove the history was not rewritten **and** prove the private bytes are gone.

---

## 9. Where the implementation falls short of the idea

These limits do not touch the design; they sit in the reference code:

- **Lexical recall.** Ochiai over token sets misses paraphrase entirely. The architecture is
  agnostic to the similarity function, so an embedding model drops in without changing any rule
  above.
- **Synthesis is concatenation.** The "reconstructed core" joins the top supporting snippets with
  a separator, and "uncertainty" is just the count of snippets. Nothing is actually reconstructed.
- **Hand-set constants.** The weights $(0.42, 0.25, 0.09, 0.10, 0.07, 0.07, 0.10)$, $H = 120$,
  $\theta = 0.12$ and the rung thresholds $3/7/10$ are unjustified and untuned.
- **Count-based maturity** (see §6.3).
- **Tag-keyed consolidation.** Grouping by the *first* tag is brittle; auto-tags are the
  most-frequent tokens.
- **Single-carrier, synthetic validation.** "Cross-carrier continuity" is a design goal; the
  tests exercise one carrier in a synthetic environment, as the README itself states.

The rhetoric ("living", "zombie", "colonization") outruns the code. The *rules* do not.

---

## 10. What is worth borrowing

Ranked by value per line of code, for anyone building agent memory, including a harness
whose own canon already says *"a document's claim about current state is a memory, not a fact"*:

1. **The proposal–memory boundary (§5).** Model output never writes memory; observed effects
   do. §5.2 shows this is the difference between an error floor of $\delta$ and an error floor of 1.
2. **Forced dissent in recall (§4.1).** Two reserved slots, one for the best dissenting trace.
   A three-line change to any top-$k$ retriever.
3. **Declared omissions (§4.2).** Return what scored above threshold but was cut, and never
   phrase "none selected" as "none exist".
4. **Correction priors (§5.3).** Corrections get $s = 1.0$ plus a polarity bonus, worth about
   $0.37$ of query similarity.
5. **Hash-detected replay loops (§6.3).** Sign each recall; three identical signatures with no
   new outcome is a loop. This applies equally to agents that repeat the same plan or the same
   failing tool call.
6. **Dissent-exempt consolidation (Prop. 2).** Summaries may merge agreement; they may not
   absorb exceptions.
7. **Digest-only audit chains (§8).** Tamper evidence and deletion, both at once.

None of these needs the twelve dimensions, the eight states or the DIKWP vocabulary to work. Each
one fixes a specific, nameable failure of retrieval-only memory: self-poisoning, confirmation bias,
silent omission, sticky errors, loops, majority erasure, and the audit-versus-privacy deadlock.

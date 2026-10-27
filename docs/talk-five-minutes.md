# A machine that was never broken

**A five-minute talk, built on the two canonical runs.**

The script below is written to be spoken, not read aloud from the page. Roughly 780 words of speech, which is
about five minutes at a relaxed pace. The timings are cues, not a stopwatch.

Before you start, put two things on screen and nothing else: a terminal, and one slide with three lines —
**truth**, **belief**, **the gap between them**. No bullet lists, no architecture diagram. The talk is two
stories and three words.

---

## 0:00 — Open with a wrong alarm

> This is a story about a machine that was never broken, and the system that was right to distrust it anyway.

*(beat)*

> It's also about your monitoring. When it says a server is down, it is telling you something much narrower than
> you think. It means *I haven't heard from it.* That's all. And the distance between those two sentences is
> where outages get interesting.

## 0:30 — Why this is genuinely hard

> Every machine in a distributed system is blind in exactly one direction: it can only see what arrives. It
> cannot see the network. It cannot see the other machines. When silence happens, it has to guess between four
> things — the machine is dead, the network is broken, *my* end is broken, or it's just slow.
>
> And it can't wait to find out. It has to decide *now* whether to keep sending traffic there. So it decides on
> incomplete information, every time, forever.
>
> Which means every one of these systems keeps two records. **Truth** — what actually happened. And **belief** —
> what each machine concluded. Production systems usually log only the second one, then wonder why the incident
> review is an argument.

## 1:15 — Story one: the machine dies

`make demo`, or the table already on screen ([README](../README.md#what-one-run-shows)).

> Three machines. B sends a heartbeat to A and C every hundred milliseconds. The link to A takes twenty
> milliseconds. The link to C takes twenty-five.
>
> At 215 milliseconds, B dies. Hard. *(point at the 215 line)*
>
> Now look at 220 and 225. A and C both receive a heartbeat **from a dead machine.** B sent it at 200, it was
> already in flight, and the network doesn't care that the sender stopped existing.
>
> Then nothing. Silence. And after 350 milliseconds of silence, each observer gives up: A at 570, C at 575.
>
> So the detection took 355 milliseconds, not the 350 we configured. That five-millisecond difference is exactly
> the last message arriving after the crash. And it's why people say "the timeout is 350 milliseconds" and then
> the graphs say something else.
>
> *(pause)* One crash. Two observers. Two different times. Neither of them wrong.

## 2:30 — Story two: nothing dies

`go run ./cmd/aegis run --scenario one-way-partition --format table` ([timeline](../README.md#what-the-partition-run-shows)).

> Now the real one. Same cluster. Nobody dies. At 300 milliseconds we cut **one direction** of one link: B can't
> reach A. A's messages to B are untouched, B's to C are untouched.
>
> A starts getting silence. Six heartbeats in a row vanish — 320, 420, 520, on to 820. A does the only thing it
> can: at 570 milliseconds, it stops trusting B.
>
> Meanwhile B is sending, and B is receiving from A the whole time. So B thinks A is perfectly healthy. C, on its
> own link, sees nothing at all. One physical break. Three machines. Two of them completely fine with the
> situation.
>
> *(beat — this is the hinge)* At 900 milliseconds, we repair the link.
>
> And **nothing happens.**
>
> A still distrusts B. Why would it change its mind? A's evidence is heartbeats, and no heartbeat has arrived.
> Repairing a wire is not evidence. The belief only flips at 920 — when round nine, sent at 900, actually lands.
>
> So: a perfectly healthy machine was marked unhealthy for 350 milliseconds. Its logs are internally consistent
> the entire time. Every observer was right about everything it could observe. And the repair fixed the network,
> not the opinion.

## 4:00 — Why you should believe any of this

> Two things make this more than an animation.
>
> First, the clock is fake and exact. Time is an integer count of nanoseconds. "Did the timeout expire before or
> after this message arrived" has one answer, forever. Nothing consults the wall clock.
>
> Second, the component that forms beliefs **cannot see ground truth.** It's a separate package with a one-way
> dependency — the compiler forbids it, not a code review. So a correct belief is correct because the evidence
> supported it.
>
> Because of both, each run's log is a committed file — sixteen events for the crash, seventy-five for the
> partition — compared byte for byte on every commit. Change the simulation and the diff tells you what you
> changed.

## 4:45 — Close

> So, two failures. One where the truth changed and the beliefs followed late. One where the truth never changed
> and the beliefs were wrong anyway.
>
> If you take one thing: when something tells you a node is down, it's telling you about a *wire*, not a
> *machine*. The gap between those is measurable — and if you don't measure it, you'll be guessing during your
> worst hour.
>
> Next, we make the log explain *why* each belief changed, not just when. That's the point where you can start
> doing recovery that's actually justified.

---

## Delivery notes

**The three numbers to land cleanly:** 215 and 570 in story one, and 900 → 920 in story two. If you fumble
anything else, get those right; the whole argument lives in them.

**The pause that matters** is after "the repair does nothing." Don't fill it. That's the moment the room gets it.

**If you're running short,** cut the determinism section down to one line: "the log is a committed file, compared
byte for byte." Keep both stories.

**If someone asks how this differs from a real system,** the honest answer: it's a simulation, single-threaded,
and a message caught in a cut is lost rather than delayed. It models the reasoning faithfully; it doesn't model
TCP.

**If the demo breaks,** the tables in the [README](../README.md) carry the same numbers, and
[doc_m2.md](../doc_m2.md) has the timeline written out. Talk over those instead; nobody will know.

## Facts used

| claim | where it comes from |
| --- | --- |
| B crashes at 215 ms; last evidence arrives at 220 ms and 225 ms | `node-crash` run, events 11–13 |
| A and C suspect at 570 ms and 575 ms | `node-crash` run, events 14–15 |
| 355 ms latency versus a configured 350 ms timeout | suspicion tick minus crash tick |
| the B→A cut runs 300–900 ms; six heartbeats are dropped | `one-way-partition` run, events 17–55 |
| A suspects at 570 ms and recovers at 920 ms | `one-way-partition` run, events 36 and 61 |
| round nine departs at 900 ms, exactly as the path heals, and arrives | `one-way-partition` run, events 57–60 |
| 16 and 75 events, compared byte for byte | `make replay` |

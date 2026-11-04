## What this changes

<!-- One paragraph. What was wrong or missing, and why this is the fix. -->

## Evidence

<!--
The project's rule is that a claim comes with the check that would contradict
it. Paste the command and its result, or the log line that changed.
-->

```
$ make check-all
```

<!--
If this adds or changes a guard: what did you break to prove the guard fails?
"The test passed" is not evidence until the test has been seen to fail.
-->

## Checklist

- [ ] `make check-all` passes (or `make check`, if the page is not checked out)
- [ ] If a log changed, `internal/scenario/testdata/` changes with it, generated
      by the command and not edited by hand
- [ ] If a scenario was added, it is in `runnable` and the page's fixtures are
      synced (`npm run sync-fixture` in `aegis-lab`)
- [ ] If the log format changed, `domain.EventLogSchema` was bumped and the
      TypeScript port changed in the same breath
- [ ] No new copy of a fact that already exists somewhere else
- [ ] New guards were shown to fail before being trusted
- [ ] Commits have a scoped, imperative subject and a body that says why

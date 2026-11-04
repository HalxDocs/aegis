# Security policy

## What this program does

Aegis is a deterministic simulator. It runs a discrete-event world on a virtual
clock, writes a JSONL log to stdout, and reads two kinds of file: a scenario
file, and a previously written log. It opens no sockets, listens on no port,
spawns no process other than itself, and talks to no service. There is no daemon
and no server component.

That narrows the attack surface to the parsers and to anything that consumes the
output, but it does not remove it. Reading a file is still reading a file.

## Supported versions

The latest tagged release receives fixes. Older tags are kept for readers
following a milestone and are not patched.

| version | supported |
| --- | --- |
| v0.4.x | yes |
| v0.3.x and earlier | no |

## Reporting a vulnerability

Please report privately rather than in a public issue:

- Open a [private security advisory](https://github.com/HalxDocs/aegis/security/advisories/new)
  on this repository, or
- email **halxdocs@gmail.com** with the details.

Include what you did, what happened, and what you expected, plus the output of
`aegis version` so the build and the log format are known. A minimal scenario
file or log that reproduces it is the most useful thing you can send.

You can expect an acknowledgement within a few days. If the report is valid, the
fix and the advisory are published together, and you are credited unless you ask
otherwise.

## In scope

- A malformed scenario file or log that crashes the program, hangs it, or makes it
  accept input it should refuse.
- A crafted log that produces a proof for an action that never happened, or an
  explanation that contradicts the events it cites. The verifier and the causal
  index treat a file as untrusted input on purpose; a way around either one is a
  real finding.
- Anything that makes a run non-deterministic from the same inputs, since the
  project's guarantees rest on two runs producing identical bytes.

## Out of scope

- The simulator reporting a world you did not intend because the scenario file
  says so. A scenario is a description, not a sandbox: it runs what it is given.
- Memory exhaustion from an enormous horizon or a hand-written log with millions
  of lines. There is no size limit and none is claimed.
- Vulnerabilities in the explainer page at
  [aegis-lab](https://github.com/HalxDocs/aegis-lab) — report those there.

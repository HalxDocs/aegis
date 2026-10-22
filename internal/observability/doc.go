// Package observability owns the append-only history a run produces.
//
// The recorder is the single writer: it assigns sequence numbers, refuses to
// append an event that is invalid or earlier than its predecessor, and renders
// the history as JSONL. Nothing else may renumber, reorder or rewrite an event,
// because the log is the artifact later milestones reason over and a log that
// can be edited is not evidence.
package observability

// Package helper holds the building blocks the rest of the toolkit shares.
//
// [Process] runs a child process and hands back its output one line at a time,
// owning the child from [NewProcess] until [Process.Close]. [Run] wraps it for
// the common case: launch a command, collect everything it writes, and report
// the status it exited with. [Search] walks a folder and returns the lines
// matching an expression, skipping what is not text. [WithTimeout] applies a
// default deadline to a context that has none, leaving one the caller already
// chose in place.
package helper

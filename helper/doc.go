// Package helper holds the building blocks the rest of the toolkit shares, and
// that a caller may reach for directly.
//
// [Process] runs a child process and hands back its output one line at a time,
// owning the child from [NewProcess] until [Process.Close]. [Run] wraps it for
// the common case: launch a command, collect everything it writes, and report
// the status it exited with.
package helper

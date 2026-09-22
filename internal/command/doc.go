// Package command runs child processes on the toolkit's behalf.
//
// [Process] runs a child process and hands back its output one line at a time,
// owning the child from [NewProcess] until [Process.Close]. [Run] wraps it for
// the common case: launch a command, collect everything it writes, and report
// the status it exited with. [InheritedEnv] builds a child's environment from an
// allowlist, so a child does not receive the credentials the calling process
// holds in its own.
package command

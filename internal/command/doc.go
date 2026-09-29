// Package command runs child processes for the toolkit.
//
// [Process] runs a child and delivers its output one line at a time; it owns
// the child from [NewProcess] until [Process.Close]. [Run] covers the common
// case: launch a command, collect all its output, and report its exit status.
// [InheritedEnv] builds a child's environment from an allowlist, so the child
// does not get the credentials in the caller's environment.
package command

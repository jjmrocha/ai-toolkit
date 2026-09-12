package helper

import (
	"os"
	"slices"
	"strings"

	"github.com/jjmrocha/go-algo/sets"
)

var defaultEnvNames = []string{
	"HOME",
	"LOGNAME",
	"PATH",
	"SHELL",
	"TERM",
	"USER",
	"TMPDIR",
	"LANG",
	"TZ",
	"SSL_CERT_DIR",
	"SSL_CERT_FILE",
	"HTTP_PROXY",
	"HTTPS_PROXY",
	"NO_PROXY",
	"http_proxy",
	"https_proxy",
	"no_proxy",
}

var defaultEnvPrefixes = []string{"LC_"}

// InheritedEnv builds a child process's environment from the calling process's,
// copying only what a child is expected to need: the variables that locate the
// user and the tools ([DefaultEnvNames]), the ones that keep TLS, proxies and
// locale working, every LC_ variable, and the variables extra names. Everything
// else is left behind, so a child does not receive the credentials the calling
// process holds in its environment.
//
// A name in extra that the calling process does not set is skipped rather than
// passed on empty, so a child cannot tell an unset variable from one that was
// never allowed. The result is sorted, and suitable for [ProcessConfig.Env] and
// [RunConfig.Env].
func InheritedEnv(extra []string) []string {
	allowed := sets.New(append(slices.Clone(defaultEnvNames), extra...)...)

	var env []string

	for _, entry := range os.Environ() {
		name, _, found := strings.Cut(entry, "=")
		if !found {
			continue
		}

		if allowed.Contains(name) || hasAllowedPrefix(name) {
			env = append(env, entry)
		}
	}

	slices.Sort(env)

	return env
}

func hasAllowedPrefix(name string) bool {
	return slices.ContainsFunc(defaultEnvPrefixes, func(prefix string) bool {
		return strings.HasPrefix(name, prefix)
	})
}

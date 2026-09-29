package command

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

// InheritedEnv builds a child's environment from the caller's, copying only
// what a child needs: the variables that locate the user and the tools, those
// that keep TLS, proxies and locale working, every LC_ variable, and the
// variables named in extra. Nothing else is copied, so the child does not get
// the credentials in the caller's environment.
//
// A name in extra that the caller does not set is skipped, not passed empty.
// The result is sorted, ready for [ProcessConfig.Env] and [RunConfig.Env].
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

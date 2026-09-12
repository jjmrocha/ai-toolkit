package helper

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInheritedEnv(t *testing.T) {
	t.Run("copies a variable from the default set", func(t *testing.T) {
		// given
		t.Setenv("PATH", "/usr/bin")
		// when
		result := InheritedEnv(nil)
		// then
		assert.Contains(t, result, "PATH=/usr/bin")
	})

	t.Run("copies the wider defaults that break TLS and proxies", func(t *testing.T) {
		// given
		t.Setenv("HTTPS_PROXY", "http://proxy:3128")
		t.Setenv("SSL_CERT_FILE", "/etc/ssl/cert.pem")
		t.Setenv("TZ", "Europe/Lisbon")
		// when
		result := InheritedEnv(nil)
		// then
		assert.Contains(t, result, "HTTPS_PROXY=http://proxy:3128")
		assert.Contains(t, result, "SSL_CERT_FILE=/etc/ssl/cert.pem")
		assert.Contains(t, result, "TZ=Europe/Lisbon")
	})

	t.Run("copies any variable with an allowed prefix", func(t *testing.T) {
		// given
		t.Setenv("LC_CTYPE", "pt_PT.UTF-8")
		// when
		result := InheritedEnv(nil)
		// then
		assert.Contains(t, result, "LC_CTYPE=pt_PT.UTF-8")
	})

	t.Run("leaves out a variable outside the default set", func(t *testing.T) {
		// given
		t.Setenv("ANTHROPIC_API_KEY", "sk-secret")
		// when
		result := InheritedEnv(nil)
		// then
		assert.NotContains(t, result, "ANTHROPIC_API_KEY=sk-secret")
		assert.False(t, slices.ContainsFunc(result, func(entry string) bool {
			return len(entry) >= 9 && entry[:9] == "ANTHROPIC"
		}))
	})

	t.Run("copies a variable named in extra", func(t *testing.T) {
		// given
		t.Setenv("GITHUB_TOKEN", "ghp-secret")
		// when
		result := InheritedEnv([]string{"GITHUB_TOKEN"})
		// then
		assert.Contains(t, result, "GITHUB_TOKEN=ghp-secret")
	})

	t.Run("skips a named variable that is not set", func(t *testing.T) {
		// given
		expected := InheritedEnv(nil)
		// when
		result := InheritedEnv([]string{"DEFINITELY_NOT_SET_ANYWHERE"})
		// then
		assert.Equal(t, expected, result)
	})

	t.Run("returns entries sorted so the environment is stable", func(t *testing.T) {
		// given
		t.Setenv("PATH", "/usr/bin")
		t.Setenv("HOME", "/home/test")
		// when
		result := InheritedEnv(nil)
		// then
		require.True(t, slices.IsSorted(result))
	})
}

func TestProcessEnv(t *testing.T) {
	t.Run("runs the command with only the given environment", func(t *testing.T) {
		// given
		t.Setenv("ANTHROPIC_API_KEY", "sk-secret")
		cfg := RunConfig{
			Path: "sh",
			Args: []string{"-c", "echo ${ANTHROPIC_API_KEY:-absent}; echo ${MARKER:-absent}"},
			Env:  []string{"MARKER=present"},
		}
		// when
		result, err := Run(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := []string{"absent", "present"}
		assert.Equal(t, expected, result.Output)
	})

	t.Run("inherits the parent environment when Env is nil", func(t *testing.T) {
		// given
		t.Setenv("MARKER", "inherited")
		cfg := RunConfig{Path: "sh", Args: []string{"-c", "echo ${MARKER:-absent}"}}
		// when
		result, err := Run(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := []string{"inherited"}
		assert.Equal(t, expected, result.Output)
	})
}

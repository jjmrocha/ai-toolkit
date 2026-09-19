package packs

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePack struct {
	closed int
	err    error
}

func (p *fakePack) Close() error {
	p.closed++

	return p.err
}

func TestSerenaMCPConfig(t *testing.T) {
	t.Run("starts Serena with the query-projects mode", func(t *testing.T) {
		// given
		expected := []string{
			"--from", "git+https://github.com/oraios/serena",
			"serena", "start-mcp-server",
			"--context", "desktop-app",
			"--add-mode", "query-projects",
		}
		// when
		result := SerenaMCPConfig()
		// then
		assert.Equal(t, expected, result.Args)
	})

	t.Run("leaves Serena's shell tool out", func(t *testing.T) {
		// given
		expected := []string{"execute_shell_command"}
		// when
		result := SerenaMCPConfig()
		// then
		assert.Equal(t, expected, result.ExcludedTools)
	})

	t.Run("shares nothing between calls", func(t *testing.T) {
		// given
		expected := SerenaMCPConfig()
		// when
		result := SerenaMCPConfig()
		result.ExcludedTools[0] = "find_symbol"
		// then
		assert.NotEqual(t, result.ExcludedTools, expected.ExcludedTools)
	})
}

func TestCodingPackClose(t *testing.T) {
	t.Run("closes both the shell and the server", func(t *testing.T) {
		// given
		shell := &fakePack{}
		server := &fakePack{}
		pack := codingPack{shell: shell, serena: server}
		// when
		err := pack.Close()
		// then
		require.NoError(t, err)
		assert.Equal(t, 1, shell.closed)
		assert.Equal(t, 1, server.closed)
	})

	t.Run("closes the server even when the shell fails", func(t *testing.T) {
		// given
		expected := errors.New("shell")
		server := &fakePack{}
		pack := codingPack{shell: &fakePack{err: expected}, serena: server}
		// when
		err := pack.Close()
		// then
		assert.ErrorIs(t, err, expected)
		assert.Equal(t, 1, server.closed)
	})

	t.Run("reports both failures", func(t *testing.T) {
		// given
		shellErr := errors.New("shell")
		serverErr := errors.New("server")
		pack := codingPack{shell: &fakePack{err: shellErr}, serena: &fakePack{err: serverErr}}
		// when
		err := pack.Close()
		// then
		assert.ErrorIs(t, err, shellErr)
		assert.ErrorIs(t, err, serverErr)
	})
}

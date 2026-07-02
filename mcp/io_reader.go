package mcp

import (
	"context"
	"io"
)

// readBytes reads from r until delim is found, returning the bytes read up to
// and including delim. ctx is checked before each read, so a cancelled context
// stops it between reads; a read already blocked inside r is not interrupted. On
// error it returns the bytes read so far together with the error, which is often
// io.EOF for a final line that lacks a trailing delim.
func readBytes(ctx context.Context, r io.Reader, delim byte) ([]byte, error) {
	var line []byte
	b := make([]byte, 1)

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		n, err := r.Read(b)
		if n > 0 {
			line = append(line, b[0])

			if b[0] == delim {
				return line, nil
			}
		}

		if err != nil {
			return line, err
		}
	}
}

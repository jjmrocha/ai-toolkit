package mcp

import (
	"context"
	"io"
)

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

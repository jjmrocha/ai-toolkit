package skills

import "strings"

const fence = "---"

func parseFrontmatter(content string) (string, string, string, error) {
	lines := strings.Split(content, "\n")

	if strings.TrimSpace(lines[0]) != fence {
		return "", "", "", ErrInvalidFrontmatter
	}

	var name, description string

	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])

		if line == fence {
			return name, description, bodyFrom(lines[i+1:]), nil
		}

		if line == "" {
			continue
		}

		key, rawValue, found := strings.Cut(line, ":")
		if !found {
			return "", "", "", ErrInvalidFrontmatter
		}

		value := strings.TrimSpace(rawValue)
		if value == "" || strings.HasPrefix(value, ">") || strings.HasPrefix(value, "|") {
			return "", "", "", ErrInvalidFrontmatter
		}

		switch strings.TrimSpace(key) {
		case "name":
			name = unquote(value)
		case "description":
			description = unquote(value)
		}
	}

	return "", "", "", ErrInvalidFrontmatter
}

func bodyFrom(lines []string) string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}

	return strings.Join(lines, "\n")
}

func unquote(value string) string {
	if len(value) < 2 {
		return value
	}

	quote := value[0]
	if quote != '"' && quote != '\'' {
		return value
	}

	if value[len(value)-1] != quote {
		return value
	}

	return value[1 : len(value)-1]
}

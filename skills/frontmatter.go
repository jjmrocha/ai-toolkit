package skills

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const fence = "---"

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func parseFrontmatter(content string) (string, string, string, error) {
	block, body, err := splitFrontmatter(content)
	if err != nil {
		return "", "", "", err
	}

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(block), &fm); err != nil {
		return "", "", "", fmt.Errorf("%w: %w", ErrInvalidFrontmatter, err)
	}

	return fm.Name, fm.Description, body, nil
}

func splitFrontmatter(content string) (string, string, error) {
	lines := strings.Split(content, "\n")

	if strings.TrimSpace(lines[0]) != fence {
		return "", "", ErrInvalidFrontmatter
	}

	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == fence {
			return strings.Join(lines[1:i], "\n"), bodyFrom(lines[i+1:]), nil
		}
	}

	return "", "", ErrInvalidFrontmatter
}

func bodyFrom(lines []string) string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}

	return strings.Join(lines, "\n")
}

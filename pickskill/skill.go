package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Skill struct {
	Name        string `json:"name"`
	Frontmatter string `json:"frontmatter"`
}

func loadSkills(root string) ([]Skill, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read skills directory %q: %w", root, err)
	}

	var skills []Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name(), "SKILL.md")
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %q: %w", path, err)
		}
		frontmatter, err := extractFrontmatter(data)
		if err != nil {
			return nil, fmt.Errorf("parse %q: %w", path, err)
		}
		skills = append(skills, Skill{Name: entry.Name(), Frontmatter: frontmatter})
	}

	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	if len(skills) == 0 {
		return nil, fmt.Errorf("no top-level <skill-name>/SKILL.md files found in %q", root)
	}
	return skills, nil
}

func extractFrontmatter(data []byte) (string, error) {
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return "", fmt.Errorf("file must start with a YAML frontmatter delimiter (---)")
	}
	for i := 1; i < len(lines); i++ {
		if lines[i] != "---" {
			continue
		}
		frontmatter := strings.TrimSpace(strings.Join(lines[1:i], "\n"))
		if frontmatter == "" {
			return "", fmt.Errorf("frontmatter is empty")
		}
		return frontmatter, nil
	}
	return "", fmt.Errorf("frontmatter has no closing delimiter (---)")
}

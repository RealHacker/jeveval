package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExtractFrontmatter(t *testing.T) {
	got, err := extractFrontmatter([]byte("\xef\xbb\xbf---\r\nname: demo\r\ndescription: Does things\r\n---\r\n# Body\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := "name: demo\ndescription: Does things"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExtractFrontmatterRejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"missing opener", "name: demo\n---\n"},
		{"missing closer", "---\nname: demo\n"},
		{"empty", "---\n---\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := extractFrontmatter([]byte(test.data)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadSkillsUsesOnlyImmediateChildDirectories(t *testing.T) {
	root := t.TempDir()
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "zeta", "SKILL.md"), "---\ndescription: z\n---\nbody")
	write(filepath.Join(root, "alpha", "SKILL.md"), "---\ndescription: a\n---\nbody")
	write(filepath.Join(root, "ignored", "nested", "SKILL.md"), "---\ndescription: nested\n---\n")
	write(filepath.Join(root, "SKILL.md"), "---\ndescription: root\n---\n")

	skills, err := loadSkills(root)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := []string{skills[0].Name, skills[1].Name}, []string{"alpha", "zeta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

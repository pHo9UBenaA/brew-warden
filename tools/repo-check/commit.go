package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"unicode/utf8"
)

var subjectPattern = regexp.MustCompile(`^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([a-z0-9][a-z0-9._/-]*\))?!?: [^[:space:]].*$`)

func validateMessage(message string) error {
	message = strings.ReplaceAll(message, "\r\n", "\n")
	// Ignore Git's default template comments, including status lines.
	var lines []string
	for _, line := range strings.Split(message, "\n") {
		if !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	message = strings.TrimRight(strings.Join(lines, "\n"), "\n")
	lines = strings.Split(message, "\n")
	if err := textHygiene([]byte(message)); err != nil {
		return err
	}
	if len(lines) == 0 || !subjectPattern.MatchString(lines[0]) || strings.TrimSpace(lines[0]) != lines[0] {
		return fmt.Errorf("expected a Conventional Commit: type(scope): description")
	}
	if utf8.RuneCountInString(lines[0]) > 100 {
		return fmt.Errorf("commit subject exceeds 100 characters")
	}
	if len(lines) > 1 && lines[1] != "" {
		return fmt.Errorf("separate commit body from subject with a blank line")
	}
	for _, line := range lines {
		for _, prefix := range []string{"BREAKING CHANGE:", "BREAKING-CHANGE:"} {
			if strings.HasPrefix(line, prefix) && strings.TrimSpace(strings.TrimPrefix(line, prefix)) == "" {
				return fmt.Errorf("breaking change footer requires a description")
			}
		}
	}
	return nil
}
func gitOutput(args ...string) (string, error) {
	b, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, b)
	}
	return string(b), nil
}
func checkCommits(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return fmt.Errorf("commits requires a tip and optional base")
	}
	sha := regexp.MustCompile(`^[0-9a-fA-F]{40}([0-9a-fA-F]{24})?$`)
	for _, a := range args {
		if !sha.MatchString(a) {
			return fmt.Errorf("commit ranges require full object IDs")
		}
	}
	revs := []string{"rev-list", args[0]}
	if len(args) == 2 {
		revs = append(revs, "^"+args[1])
	}
	revs = append(revs, "--")
	out, err := gitOutput(revs...)
	if err != nil {
		return err
	}
	for _, id := range strings.Fields(out) {
		message, err := gitOutput("show", "-s", "--format=%B", id, "--")
		if err != nil {
			return err
		}
		if err = validateMessage(message); err != nil {
			return fmt.Errorf("commit %s: %w", id, err)
		}
	}
	return nil
}

package main

import "testing"

func TestCommitMessages(t *testing.T) {
	good := []string{"feat: add planning\n", "fix(policy)!: bind exceptions\n\nBREAKING CHANGE: require a digest\n", "docs: explain verification\n\nRefs: #12\n", "chore: update checks\n# template comment\n"}
	bad := []string{"", "update stuff\n", "feat: \n", "feat:  extra space\n", "Feat: casing\n", "feat(bad scope): text\n", "fix: text\nbody\n", "fixup! feat: text\n", "fix: text\n\nBREAKING CHANGE:\n", "docs: \u65e5\u672c\u8a9e\n", "fix: \x1b[31mtext\n"}
	for _, s := range good {
		if err := validateMessage(s); err != nil {
			t.Errorf("valid message %q rejected: %v", s, err)
		}
	}
	for _, s := range bad {
		if err := validateMessage(s); err == nil {
			t.Errorf("invalid message accepted: %q", s)
		}
	}
}
func TestTextHygiene(t *testing.T) {
	for _, s := range []string{"English text\n", "A developer’s choice\n"} {
		if err := textHygiene([]byte(s)); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []string{"\u65e5\u672c\u8a9e", "a\u202eb", "\xff"} {
		if err := textHygiene([]byte(s)); err == nil {
			t.Errorf("unsafe text accepted: %q", s)
		}
	}
}

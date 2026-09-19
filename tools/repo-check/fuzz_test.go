package main

import "testing"

// Arbitrary template text must not panic or allow control bytes into the subject.
// Git's leading status comment must not change whether a message is accepted.
func FuzzCommitMessage(f *testing.F) {
	for _, seed := range []string{
		"fix: reject stale plans\n",
		"feat(policy)!: bind evidence\n\nBREAKING CHANGE: require identity\n",
		"# status\nfix: message\r\n",
		"fix: \xff\n",
		"fix: \u202etext\n",
		"fix: text\n\nBREAKING CHANGE:\n",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, message string) {
		accepted := validateMessage(message) == nil
		if (validateMessage("# Git status\n"+message) == nil) != accepted {
			t.Fatal("a template comment changed message acceptance")
		}
		if validateMessage("fix: \x1b"+message) == nil {
			t.Fatal("terminal control byte accepted in subject")
		}
	})
}

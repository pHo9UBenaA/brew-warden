// Command repo-check validates repository contracts without third-party modules.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fatal(fmt.Errorf("usage: repo-check architecture | hygiene | commit-msg <file> | commits <tip> [base]"))
	}
	var err error
	switch os.Args[1] {
	case "architecture":
		err = architecture(".")
	case "hygiene":
		err = hygiene(".")
	case "commit-msg":
		if len(os.Args) != 3 {
			err = fmt.Errorf("commit-msg requires one message file")
			break
		}
		var b []byte
		b, err = os.ReadFile(os.Args[2])
		if err == nil {
			err = validateMessage(string(b))
		}
	case "commits":
		err = checkCommits(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fatal(err)
	}
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }

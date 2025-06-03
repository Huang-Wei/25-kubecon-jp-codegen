package git

import (
	"bytes"
	"os/exec"
	"strings"
)

func gitCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.String(), err
}

func commit(dir, commitMsg string) error {
	if _, err := gitCmd(dir, "add", "."); err != nil {
		return err
	}
	if output, _ := gitCmd(dir, "status"); strings.Contains(output, "nothing to commit") {
		return ErrNothingToCommit
	}
	_, err := gitCmd(dir, "commit", "-m", commitMsg)
	return err
}

// Package gitops provides git helpers for committing and pushing xmuggle
// tasks/results within a target repo.
package gitops

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func git(cwd string, args ...string) (stdout, stderr string, code int, err error) {
	cmd := exec.Command("git", args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	var so, se bytes.Buffer
	cmd.Stdout = &so
	cmd.Stderr = &se
	err = cmd.Run()
	stdout, stderr = so.String(), se.String()
	if err == nil {
		return stdout, stderr, 0, nil
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return stdout, stderr, ee.ExitCode(), nil
	}
	return stdout, stderr, 1, err
}

func gitOrFail(cwd string, args ...string) (string, error) {
	stdout, stderr, code, err := git(cwd, args...)
	if err != nil || code != 0 {
		return stdout, fmt.Errorf("git %s: exit %d: %s: %w", strings.Join(args, " "), code, stderr, err)
	}
	return stdout, nil
}

// FindRepoRoot walks up from cwd looking for a .git/ directory.
// Returns the repo root path or an error if not in a git repo.
func FindRepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repo")
	}
	return strings.TrimSpace(string(out)), nil
}

// RemoteURL returns the origin remote URL for the repo, or "" if none.
func RemoteURL(repoDir string) string {
	out, err := exec.Command("git", "-C", repoDir, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// PullRebase pulls with rebase+autostash; tolerates missing remote refs.
func PullRebase(repoDir string) error {
	_, se, code, _ := git(repoDir, "pull", "--rebase=true", "--autostash")
	if code == 0 {
		return nil
	}
	if regexp.MustCompile(`couldn't find remote ref|no such ref|no tracking`).MatchString(se) {
		return nil
	}
	return fmt.Errorf("git pull: %s", se)
}

// CommitAndPush stages paths, commits, pushes. Retries once on non-fast-forward.
func CommitAndPush(repoDir string, paths []string, message string) error {
	if len(paths) == 0 {
		return nil
	}
	for _, p := range paths {
		if _, err := gitOrFail(repoDir, "add", "--all", "--", p); err != nil {
			return err
		}
	}

	status, err := gitOrFail(repoDir, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) == "" {
		return nil
	}

	if _, err := gitOrFail(repoDir, "commit", "-m", message); err != nil {
		return err
	}

	_, se, code, _ := git(repoDir, "push")
	if code == 0 {
		return nil
	}
	if regexp.MustCompile(`non-fast-forward|rejected|fetch first`).MatchString(se) {
		if err := PullRebase(repoDir); err != nil {
			return err
		}
		if _, err := gitOrFail(repoDir, "push"); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("git push: %s", se)
}

// GitRm removes a path from the git index and working tree.
func GitRm(repoDir string, paths ...string) error {
	args := append([]string{"rm", "-rf", "--"}, paths...)
	_, err := gitOrFail(repoDir, args...)
	return err
}

// EnsureDir creates a directory if it doesn't exist.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

// WriteFile writes data to a file, creating parent dirs as needed.
func WriteFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

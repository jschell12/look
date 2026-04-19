// Package aq shells out to agent-queue scripts for task coordination.
package aq

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ScriptsDir returns the agent-queue scripts directory. Override via AQ_SCRIPTS.
func ScriptsDir() string {
	if p := os.Getenv("AQ_SCRIPTS"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "development/github.com/jschell12/agent-queue/scripts")
}

type aqResult struct {
	stdout string
	stderr string
	code   int
}

func aqRun(args ...string) (aqResult, error) {
	cmd := exec.Command(filepath.Join(ScriptsDir(), "agent-queue"), args...)
	var so, se bytes.Buffer
	cmd.Stdout = &so
	cmd.Stderr = &se
	err := cmd.Run()
	if err == nil {
		return aqResult{stdout: so.String(), stderr: se.String(), code: 0}, nil
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return aqResult{stdout: so.String(), stderr: se.String(), code: ee.ExitCode()}, nil
	}
	return aqResult{stdout: so.String(), stderr: se.String(), code: 1}, err
}

// aq is a convenience wrapper that discards stderr (use aqRun when you need it).
func aq(args ...string) (string, int, error) {
	r, err := aqRun(args...)
	return r.stdout, r.code, err
}

// Init creates the queue for a project (idempotent).
func Init(project string) error {
	_, _, err := aq("init", "-p", project)
	return err
}

// Add enqueues a task item with tags.
func Add(project, title, description string, tags []string) error {
	args := []string{"add", title, description, "-p", project}
	if len(tags) > 0 {
		args = append(args, "--tags")
		args = append(args, joinTags(tags))
	}
	_, _, err := aq(args...)
	return err
}

func joinTags(tags []string) string {
	var b bytes.Buffer
	for i, t := range tags {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(t)
	}
	return b.String()
}

// Clone returns the clone_dir and branch for a newly provisioned agent workspace.
type CloneInfo struct {
	CloneDir string `json:"clone_dir"`
	Branch   string `json:"branch"`
}

func Clone(repoURL, agentID, parentDir string) (*CloneInfo, error) {
	r, err := aqRun("clone", repoURL, agentID, "--parent", parentDir)
	if err != nil {
		return nil, err
	}
	if r.code != 0 {
		return nil, fmt.Errorf("aq clone exited %d: %s%s", r.code, r.stdout, r.stderr)
	}
	// stdout may contain status lines before the JSON object
	out := r.stdout
	if idx := strings.Index(out, "{"); idx >= 0 {
		out = out[idx:]
	}
	info := &CloneInfo{}
	if err := json.Unmarshal([]byte(out), info); err != nil {
		return nil, err
	}
	return info, nil
}

type Item struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Status      string   `json:"status"`
	Agent       string   `json:"agent"`
	AssignedTo  string   `json:"assigned_to"`
	Tags        []string `json:"tags"`
	Priority    string   `json:"priority"`
	Branch      string   `json:"branch"`
	Description string   `json:"description"`
}

// List returns items in a project, optionally filtered by status.
func List(project, status string) ([]Item, error) {
	args := []string{"list", "-p", project, "--json"}
	if status != "" {
		args = append(args, "--status", status)
	}
	out, code, err := aq(args...)
	if err != nil || code != 0 {
		return nil, err
	}
	var items []Item
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		return nil, err
	}
	return items, nil
}

var agentIDRE = regexp.MustCompile(`^agent-(\d+)$`)

// NextAgentID returns the next unused "agent-N" id for the project.
func NextAgentID(project string) (string, error) {
	items, err := List(project, "")
	if err != nil {
		// Empty queue is fine
		return "agent-1", nil
	}
	max := 0
	for _, it := range items {
		m := agentIDRE.FindStringSubmatch(it.Agent)
		if m == nil {
			continue
		}
		if n, err := strconv.Atoi(m[1]); err == nil && n > max {
			max = n
		}
	}
	return "agent-" + strconv.Itoa(max+1), nil
}

// ActiveWorkerCount returns the number of distinct agents with in-progress items.
func ActiveWorkerCount(project string) (int, error) {
	items, err := List(project, "in-progress")
	if err != nil {
		return 0, err
	}
	seen := map[string]struct{}{}
	for _, it := range items {
		if it.Agent != "" {
			seen[it.Agent] = struct{}{}
		}
	}
	return len(seen), nil
}

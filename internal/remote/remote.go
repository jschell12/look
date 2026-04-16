// Package remote implements the SSH/rsync task transport between Macs on a LAN.
package remote

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jschell12/xmuggle/internal/queue"
)

type Target struct {
	Host       string
	User       string
	QueueDir   string // remote path
	ResultsDir string // remote path
}

func (t Target) sshAddr() string {
	if t.User != "" {
		return t.User + "@" + t.Host
	}
	return t.Host
}

func (t Target) queueDir() string {
	if t.QueueDir != "" {
		return t.QueueDir
	}
	return "~/.xmuggle/queue"
}

func (t Target) resultsDir() string {
	if t.ResultsDir != "" {
		return t.ResultsDir
	}
	return "~/.xmuggle/results"
}

func run(name string, args ...string) (stdout, stderr string, code int, err error) {
	cmd := exec.Command(name, args...)
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

// SendTask rsyncs a task directory to the remote queue.
func SendTask(t Target, taskDir, taskID string) error {
	remote := fmt.Sprintf("%s/%s/", t.queueDir(), taskID)

	// Ensure remote dir
	if _, se, code, err := run("ssh", t.sshAddr(), fmt.Sprintf("mkdir -p %s/%s", t.queueDir(), taskID)); err != nil || code != 0 {
		return fmt.Errorf("ssh mkdir failed: %s: %w", se, err)
	}

	// rsync
	_, se, code, err := run(
		"rsync",
		"-az",
		taskDir+"/",
		t.sshAddr()+":"+remote,
	)
	if err != nil || code != 0 {
		return fmt.Errorf("rsync failed (exit %d): %s", code, se)
	}
	return nil
}

// PollForResult polls the remote machine for a task result JSON.
func PollForResult(t Target, taskID string, timeout, interval time.Duration) (*queue.Result, error) {
	if timeout == 0 {
		timeout = 10 * time.Minute
	}
	if interval == 0 {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(timeout)
	remotePath := fmt.Sprintf("%s/%s/result.json", t.resultsDir(), taskID)

	for time.Now().Before(deadline) {
		out, _, code, _ := run("ssh", t.sshAddr(), fmt.Sprintf("cat %s 2>/dev/null", remotePath))
		if code == 0 && strings.TrimSpace(out) != "" {
			r := &queue.Result{}
			if err := json.Unmarshal([]byte(out), r); err == nil {
				return r, nil
			}
		}
		_, _ = io.WriteString(os.Stderr, ".")
		time.Sleep(interval)
	}
	return nil, fmt.Errorf("timed out waiting for result after %s. Is the daemon running on %s?", timeout, t.Host)
}

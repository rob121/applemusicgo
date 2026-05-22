package music

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxBuffer = 100 * 1024 * 1024

func RunJXA(script string) (string, error) {
	cmd := exec.Command("osascript", "-l", "JavaScript", "-e", script)
	cmd.Stderr = &bytes.Buffer{}
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if b, ok := cmd.Stderr.(*bytes.Buffer); ok {
			stderr = strings.TrimSpace(b.String())
		}
		if stderr != "" {
			return "", fmt.Errorf("%w: %s", err, stderr)
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func RunJXAFile(path string, args ...string) (string, error) {
	argv := append([]string{"-l", "JavaScript", path}, args...)
	cmd := exec.Command("osascript", argv...)
	cmd.Stderr = &bytes.Buffer{}
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if b, ok := cmd.Stderr.(*bytes.Buffer); ok {
			stderr = strings.TrimSpace(b.String())
		}
		if stderr != "" {
			return "", fmt.Errorf("%w: %s", err, stderr)
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func RunAppleScript(script string) (string, error) {
	cmd := exec.Command("osascript", "-e", script)
	cmd.Stderr = &bytes.Buffer{}
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if b, ok := cmd.Stderr.(*bytes.Buffer); ok {
			stderr = strings.TrimSpace(b.String())
		}
		if stderr != "" {
			return "", fmt.Errorf("%w: %s", err, stderr)
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func RunAppleScriptFile(path string, args ...string) (string, error) {
	argv := append([]string{path}, args...)
	cmd := exec.Command("osascript", argv...)
	cmd.Stderr = &bytes.Buffer{}
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if b, ok := cmd.Stderr.(*bytes.Buffer); ok {
			stderr = strings.TrimSpace(b.String())
		}
		if stderr != "" {
			return "", fmt.Errorf("%w: %s", err, stderr)
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func RunJXAWithBuffer(script string, maxBuf int) (string, error) {
	cmd := exec.Command("osascript", "-l", "JavaScript", "-e", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	out := stdout.String()
	if maxBuf > 0 && len(out) > maxBuf {
		return "", fmt.Errorf("osascript output exceeded buffer (%d bytes)", maxBuf)
	}
	return strings.TrimSpace(out), nil
}

func RunJXAFromTemp(script string) error {
	tmp, err := os.CreateTemp("", "applemusicgo-*.js")
	if err != nil {
		return err
	}
	path := tmp.Name()
	defer os.Remove(path)

	if _, err := tmp.WriteString(script); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	_, err = RunJXAFile(path)
	return err
}

func LibDir() string {
	if dir := os.Getenv("APPLEMUSICGO_LIB"); dir != "" {
		return dir
	}
	if wd, err := os.Getwd(); err == nil {
		p := filepath.Join(wd, "lib")
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	exe, err := os.Executable()
	if err == nil {
		p := filepath.Join(filepath.Dir(exe), "lib")
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	return "lib"
}

func IsNotRunning(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "-600") || strings.Contains(msg, "isn't running")
}

func IsObjectError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "-1728")
}

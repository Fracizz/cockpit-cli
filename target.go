package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	gorun "runtime"
	"strings"
)

type runtimeTarget struct {
	local  bool
	distro string
	user   string
	values map[string]any
}

func targetFrom(spec map[string]any, name string) (string, runtimeTarget, error) {
	runtime := asMap(spec["runtime"])
	chosen := name
	if chosen == "" {
		targets := asMap(runtime["targets"])
		if gorun.GOOS == "linux" {
			if _, ok := targets["linux"]; ok {
				chosen = "linux"
			}
		}
		if chosen == "" {
			chosen = asString(runtime["default_target"])
		}
	}
	targets := asMap(runtime["targets"])
	raw, ok := targets[chosen].(map[string]any)
	if !ok {
		return "", runtimeTarget{}, fmt.Errorf("unknown target %q", chosen)
	}
	target := runtimeTarget{values: raw, local: asString(raw["distro"]) == ""}
	target.distro = asString(raw["distro"])
	target.user = asString(raw["user"])
	return chosen, target, nil
}

func expandPath(template string) string {
	return os.ExpandEnv(template)
}

func readText(target runtimeTarget, path string) (string, bool, error) {
	if !target.local {
		out, err := wslOutput(target, fmt.Sprintf("if [ -f %s ]; then cat %s; else exit 3; fi", shellQuote(path), shellQuote(path)))
		if err != nil {
			if exit3(err) {
				return "", false, nil
			}
			return "", false, err
		}
		return string(out), true, nil
	}
	local := expandPath(path)
	data, err := os.ReadFile(local)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(data), true, nil
}

func writeText(target runtimeTarget, path, content string) (string, error) {
	if !target.local {
		parent := path
		if i := strings.LastIndex(path, "/"); i >= 0 {
			parent = path[:i]
		}
		script := fmt.Sprintf("umask 077; mkdir -p %s; cat > %s; chmod 600 %s", shellQuote(parent), shellQuote(path), shellQuote(path))
		if err := wslInput(target, script, []byte(content)); err != nil {
			return "", err
		}
		return path, nil
	}
	local := expandPath(path)
	if err := os.MkdirAll(filepath.Dir(local), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(local, []byte(content), 0o600); err != nil {
		return "", err
	}
	return local, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func wslOutput(target runtimeTarget, script string) ([]byte, error) {
	cmd := exec.Command("wsl", "-d", target.distro, "-u", target.user, "--", "bash", "-lc", script)
	return cmd.Output()
}

func wslInput(target runtimeTarget, script string, input []byte) error {
	cmd := exec.Command("wsl", "-d", target.distro, "-u", target.user, "--", "bash", "-lc", script)
	cmd.Stdin = strings.NewReader(string(input))
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			return fmt.Errorf("%s", strings.TrimSpace(string(out)))
		}
		return err
	}
	return nil
}

func exit3(err error) bool {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode() == 3
	}
	return false
}

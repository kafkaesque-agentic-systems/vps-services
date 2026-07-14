package docker

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ParseEnvirons reads a shell-style .environs file and returns KEY=VALUE strings
// suitable for appending to exec.Cmd.Env.
//
// Supported format (one variable per line):
//
//	export KEY=value
//	export KEY="quoted value"
//
// Blank lines and lines beginning with # are ignored. Lines without the export
// prefix are skipped.
func ParseEnvirons(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open environs file %q: %w", path, err)
	}
	defer f.Close()

	var pairs []string
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		const prefix = "export "
		if !strings.HasPrefix(line, prefix) {
			continue
		}

		assignment := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		key, value, ok := strings.Cut(assignment, "=")
		if !ok {
			return nil, fmt.Errorf("parse environs file %q line %d: invalid assignment %q", path, lineNum, assignment)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("parse environs file %q line %d: empty variable name", path, lineNum)
		}

		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		pairs = append(pairs, key+"="+value)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read environs file %q: %w", path, err)
	}

	return pairs, nil
}

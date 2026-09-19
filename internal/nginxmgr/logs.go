package nginxmgr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	MaxLogLines = 1000
	MaxLogBytes = 512 << 10
)

var logDirectiveRE = regexp.MustCompile(`(?m)^\s*(access_log|error_log)\s+([^;\s]+)`)

type LogFile struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type LogTail struct {
	File      LogFile `json:"file"`
	Lines     int     `json:"lines"`
	Truncated bool    `json:"truncated"`
	Content   string  `json:"content"`
}

func (m *Manager) ListLogs(ctx context.Context) ([]LogFile, error) {
	output, err := m.Runner.Run(ctx, m.Runtime.Path, "-T")
	if err != nil && strings.TrimSpace(output) == "" {
		return nil, fmt.Errorf("inspect nginx config for logs: %w", err)
	}

	prefix := m.runtimePrefix(ctx)
	allowedRoots := allowedLogRoots(prefix)
	seen := map[string]bool{}
	logs := make([]LogFile, 0)
	for _, match := range logDirectiveRE.FindAllStringSubmatch(output, -1) {
		if len(match) != 3 {
			continue
		}
		kind := strings.TrimSuffix(match[1], "_log")
		raw := strings.Trim(strings.TrimSpace(match[2]), "'\"")
		if raw == "" || raw == "off" || raw == "stderr" || strings.HasPrefix(raw, "syslog:") {
			continue
		}
		path := raw
		if !filepath.IsAbs(path) {
			path = filepath.Join(prefix, path)
		}
		path = filepath.Clean(path)
		if !pathWithinRoots(path, allowedRoots) {
			continue
		}
		key := kind + "\x00" + path
		if seen[key] {
			continue
		}
		seen[key] = true
		logs = append(logs, LogFile{ID: logFileID(kind, path), Kind: kind, Path: path})
	}

	// Common distro defaults are used only when they exist on disk. This keeps
	// logging useful even when nginx -T does not contain explicit directives.
	defaults := []struct {
		kind string
		path string
	}{
		{"access", "/var/log/nginx/access.log"},
		{"error", "/var/log/nginx/error.log"},
		filepathDefault(prefix, "access"),
		filepathDefault(prefix, "error"),
	}
	for _, candidate := range defaults {
		if candidate.path == "" || !pathWithinRoots(candidate.path, allowedRoots) ||
			seen[candidate.kind+"\x00"+candidate.path] {
			continue
		}
		info, statErr := os.Stat(candidate.path)
		if statErr != nil || info.IsDir() {
			continue
		}
		seen[candidate.kind+"\x00"+candidate.path] = true
		logs = append(logs, LogFile{
			ID: logFileID(candidate.kind, candidate.path), Kind: candidate.kind, Path: candidate.path,
		})
	}

	sort.Slice(logs, func(i, j int) bool {
		if logs[i].Kind == logs[j].Kind {
			return logs[i].Path < logs[j].Path
		}
		return logs[i].Kind < logs[j].Kind
	})
	return logs, nil
}

func allowedLogRoots(prefix string) []string {
	candidates := []string{"/var/log/nginx", "/var/log/openresty"}
	if prefix != "" {
		candidates = append(candidates, filepath.Join(prefix, "logs"))
	}
	roots := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" || !filepath.IsAbs(candidate) {
			continue
		}
		candidate = filepath.Clean(candidate)
		duplicate := false
		for _, existing := range roots {
			if existing == candidate {
				duplicate = true
				break
			}
		}
		if !duplicate {
			roots = append(roots, candidate)
		}
	}
	return roots
}

func pathWithinRoots(candidate string, roots []string) bool {
	if !filepath.IsAbs(candidate) {
		return false
	}
	candidate = filepath.Clean(candidate)
	for _, root := range roots {
		rel, err := filepath.Rel(root, candidate)
		if err != nil {
			continue
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return true
		}
	}
	return false
}

func filepathDefault(prefix, kind string) struct{ kind, path string } {
	if prefix == "" {
		return struct{ kind, path string }{kind, ""}
	}
	name := kind + ".log"
	return struct{ kind, path string }{kind, filepath.Clean(filepath.Join(prefix, "logs", name))}
}

func (m *Manager) runtimePrefix(ctx context.Context) string {
	output, _ := m.Runner.Run(ctx, m.Runtime.Path, "-V")
	prefix := extractConfigureArg(output, "--prefix=")
	if prefix == "" {
		if m.Runtime.Kind == "openresty" {
			prefix = "/usr/local/openresty/nginx"
		} else {
			prefix = filepath.Dir(m.Layout.MainConfig)
		}
	}
	return filepath.Clean(prefix)
}

func logFileID(kind, path string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + path))
	return kind + "-" + hex.EncodeToString(sum[:8])
}

func (m *Manager) TailLog(ctx context.Context, logID string, lines int) (LogTail, error) {
	if lines <= 0 {
		lines = 200
	}
	if lines > MaxLogLines {
		return LogTail{}, fmt.Errorf("lines must be at most %d", MaxLogLines)
	}
	logs, err := m.ListLogs(ctx)
	if err != nil {
		return LogTail{}, err
	}
	var selected *LogFile
	for i := range logs {
		if logs[i].ID == logID {
			selected = &logs[i]
			break
		}
	}
	if selected == nil {
		return LogTail{}, errors.New("log file is not in the current nginx configuration")
	}

	content, actualLines, truncated, err := tailTextFile(selected.Path, lines, MaxLogBytes)
	if err != nil {
		return LogTail{}, fmt.Errorf("read nginx log: %w", err)
	}
	return LogTail{File: *selected, Lines: actualLines, Truncated: truncated, Content: content}, nil
}

func tailTextFile(path string, lineLimit, byteLimit int) (string, int, bool, error) {
	if lineLimit <= 0 || byteLimit <= 0 {
		return "", 0, false, errors.New("invalid tail limits")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", 0, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", 0, false, errors.New("refusing to read a symlinked log file")
	}
	if !info.Mode().IsRegular() {
		return "", 0, false, errors.New("log path is not a regular file")
	}

	file, err := os.Open(path)
	if err != nil {
		return "", 0, false, err
	}
	defer file.Close()

	info, err = file.Stat()
	if err != nil {
		return "", 0, false, err
	}
	if info.IsDir() {
		return "", 0, false, errors.New("log path is a directory")
	}
	if info.Size() == 0 {
		return "", 0, false, nil
	}

	const blockSize int64 = 16 << 10
	position := info.Size()
	buffer := make([]byte, 0, minInt(byteLimit, int(info.Size())))
	newlines := 0
	truncated := false

	for position > 0 && len(buffer) < byteLimit && newlines <= lineLimit {
		readSize := blockSize
		if position < readSize {
			readSize = position
		}
		if int(readSize) > byteLimit-len(buffer) {
			readSize = int64(byteLimit - len(buffer))
			truncated = true
		}
		position -= readSize

		block := make([]byte, int(readSize))
		n, readErr := file.ReadAt(block, position)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return "", 0, false, readErr
		}
		block = block[:n]
		for _, b := range block {
			if b == '\n' {
				newlines++
			}
		}
		buffer = append(block, buffer...)
		if newlines > lineLimit {
			break
		}
	}
	if position > 0 {
		truncated = true
	}

	text := string(buffer)
	rows := strings.Split(text, "\n")
	if len(rows) > 0 && rows[len(rows)-1] == "" {
		rows = rows[:len(rows)-1]
	}
	if len(rows) > lineLimit {
		rows = rows[len(rows)-lineLimit:]
		truncated = true
	}
	return strings.Join(rows, "\n"), len(rows), truncated, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

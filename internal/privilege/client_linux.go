//go:build linux

package privilege

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func Apply(ctx context.Context, request ApplyRequest) (ApplyResponse, error) {
	helper, err := helperExecutable()
	if err != nil {
		return ApplyResponse{}, err
	}

	data, err := json.Marshal(request)
	if err != nil {
		return ApplyResponse{}, fmt.Errorf("encode privileged request: %w", err)
	}

	cmd := exec.CommandContext(ctx, "sudo", "-n", helper, "privileged", "apply")
	cmd.Stdin = bytes.NewReader(data)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return ApplyResponse{}, fmt.Errorf("privileged helper failed: %s", message)
	}

	var response ApplyResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return ApplyResponse{}, fmt.Errorf("decode privileged response: %w", err)
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "privileged operation failed"
		}
		return response, errors.New(response.Error)
	}
	return response, nil
}

func helperExecutable() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("NGINX_MANAGER_PRIVILEGED_HELPER")); configured != "" {
		if !strings.HasPrefix(configured, "/") {
			return "", errors.New("NGINX_MANAGER_PRIVILEGED_HELPER must be an absolute path")
		}
		return configured, nil
	}
	const installedHelper = "/usr/local/bin/nginx-manager"
	if info, err := os.Stat(installedHelper); err == nil && !info.IsDir() {
		return installedHelper, nil
	}
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve current executable: %w", err)
	}
	return path, nil
}

func requireRoot() error {
	if os.Geteuid() != 0 {
		return errors.New("privileged apply must run as root")
	}
	return nil
}

func RefuseRootServer() error {
	if os.Geteuid() == 0 {
		return errors.New("refusing to run HTTP API as root; run nginx-manager serve as a dedicated user and configure the privileged helper")
	}
	return nil
}

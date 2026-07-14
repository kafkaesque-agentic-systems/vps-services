package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	envDeploySSHTarget  = "DEPLOY_SSH_TARGET"
	envDeployRemotePath = "DEPLOY_REMOTE_PATH"
	envDeployLocalRoot  = "DEPLOY_LOCAL_ROOT"
	envDeployMCPURL     = "DEPLOY_MCP_URL"
	envMCPSecretToken   = "MCP_SECRET_TOKEN"

	defaultRemotePath = "/opt/micro-services.d/services/"
	defaultMCPURL     = "https://api.thirdeye.live/sse"

	composeMarker = "docker-compose.yml"
)

// deployConfig holds resolved deployment settings for push_codebase.
type deployConfig struct {
	sshTarget  string
	remotePath string
	localRoot  string
	mcpURL     string
	token      string
}

// resolveDeployConfig reads deployment settings from the environment.
//
// DEPLOY_SSH_TARGET and MCP_SECRET_TOKEN are required. DEPLOY_REMOTE_PATH,
// DEPLOY_MCP_URL, and DEPLOY_LOCAL_ROOT fall back to documented defaults when
// unset. Local root auto-detection walks upward from the current working
// directory looking for docker-compose.yml.
func resolveDeployConfig() (*deployConfig, error) {
	sshTarget := strings.TrimSpace(os.Getenv(envDeploySSHTarget))
	if sshTarget == "" {
		return nil, fmt.Errorf(
			"%s is required: set it to the SSH destination (e.g. deploy@your-vps); "+
				"push_codebase runs only on a locally started mcp-server with rsync and SSH access",
			envDeploySSHTarget,
		)
	}

	token := strings.TrimSpace(os.Getenv(envMCPSecretToken))
	if token == "" {
		return nil, fmt.Errorf(
			"%s is required for pre-flight snapshot_create against production MCP",
			envMCPSecretToken,
		)
	}

	remotePath := strings.TrimSpace(os.Getenv(envDeployRemotePath))
	if remotePath == "" {
		remotePath = defaultRemotePath
	}
	remotePath = ensureTrailingSlash(remotePath)

	mcpURL := strings.TrimSpace(os.Getenv(envDeployMCPURL))
	if mcpURL == "" {
		mcpURL = defaultMCPURL
	}

	localRoot := strings.TrimSpace(os.Getenv(envDeployLocalRoot))
	if localRoot == "" {
		detected, err := detectLocalRoot()
		if err != nil {
			return nil, fmt.Errorf("detect %s: %w", envDeployLocalRoot, err)
		}
		localRoot = detected
	}
	localRoot = filepath.Clean(localRoot)

	return &deployConfig{
		sshTarget:  sshTarget,
		remotePath: remotePath,
		localRoot:  localRoot,
		mcpURL:     mcpURL,
		token:      token,
	}, nil
}

// detectLocalRoot walks upward from the current working directory until it
// finds docker-compose.yml, indicating the micro-services repository root.
func detectLocalRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	dir := filepath.Clean(cwd)
	for {
		if _, err := os.Stat(filepath.Join(dir, composeMarker)); err == nil {
			return dir, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat %q: %w", filepath.Join(dir, composeMarker), err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf(
		"could not find %q walking up from %q; set %s explicitly",
		composeMarker,
		cwd,
		envDeployLocalRoot,
	)
}

// ensureTrailingSlash guarantees rsync destination semantics for directory roots.
func ensureTrailingSlash(path string) string {
	if strings.HasSuffix(path, "/") {
		return path
	}
	return path + "/"
}

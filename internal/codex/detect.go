package codex

import "os/exec"

// Available returns true if the default codex binary is in PATH.
func Available() bool {
	_, found := DetectBinary("codex")
	return found
}

// DetectBinary checks if the given command is available in PATH.
// Returns the resolved path and true if found, empty string and false otherwise.
func DetectBinary(command string) (string, bool) {
	path, err := exec.LookPath(command)
	if err != nil {
		return "", false
	}
	return path, true
}

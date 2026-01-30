package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/spf13/cobra"

	"github.com/worksonmyai/programmator/internal/debug"
	"github.com/worksonmyai/programmator/internal/permission"
)

var socketPath string

var hookCmd = &cobra.Command{
	Use:    "hook",
	Short:  "Internal hook command for Claude Code permission handling",
	Long:   `This command is used internally by programmator as a PreToolUse hook for Claude Code.`,
	Hidden: true,
	RunE:   runHook,
}

func init() {
	hookCmd.Flags().StringVar(&socketPath, "socket", "", "Unix socket path for IPC")
	_ = hookCmd.MarkFlagRequired("socket")
	rootCmd.AddCommand(hookCmd)
}

type hookInput struct {
	SessionID string         `json:"session_id"`
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
	ToolUseID string         `json:"tool_use_id"`
}

type hookOutput struct {
	HookSpecificOutput hookDecision `json:"hookSpecificOutput"`
}

type hookDecision struct {
	PermissionDecision string `json:"permissionDecision"`
}

func runHook(_ *cobra.Command, _ []string) error {
	debug.Logf("hook: started, socket=%s", socketPath)

	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		debug.Logf("hook: failed to read stdin: %v", err)
		return writeHookResponse("deny")
	}

	debug.Logf("hook: received input (%d bytes)", len(input))

	var hi hookInput
	if err := json.Unmarshal(input, &hi); err != nil {
		debug.Logf("hook: failed to parse input: %v", err)
		return writeHookResponse("deny")
	}

	debug.Logf("hook: tool=%s session=%s", hi.ToolName, hi.SessionID)

	req := &permission.Request{
		SessionID: hi.SessionID,
		ToolName:  hi.ToolName,
		ToolInput: hi.ToolInput,
		ToolUseID: hi.ToolUseID,
	}

	debug.Logf("hook: sending to server...")
	decision := sendToServer(socketPath, req)
	debug.Logf("hook: server responded with %s", decision)

	claudeDecision := "deny"
	if decision == permission.DecisionAllow {
		claudeDecision = "allow"
	}

	debug.Logf("hook: returning %s to Claude", claudeDecision)
	return writeHookResponse(claudeDecision)
}

func sendToServer(socket string, req *permission.Request) permission.Decision {
	debug.Logf("hook: connecting to socket %s", socket)
	conn, err := net.Dial("unix", socket)
	if err != nil {
		debug.Logf("hook: failed to connect: %v", err)
		return permission.DecisionDeny
	}
	defer conn.Close()

	debug.Logf("hook: connected, sending request...")
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(req); err != nil {
		debug.Logf("hook: failed to send request: %v", err)
		return permission.DecisionDeny
	}

	debug.Logf("hook: waiting for response...")
	var resp permission.Response
	decoder := json.NewDecoder(conn)
	if err := decoder.Decode(&resp); err != nil {
		debug.Logf("hook: failed to read response: %v", err)
		return permission.DecisionDeny
	}

	debug.Logf("hook: got response: %s", resp.Decision)
	return resp.Decision
}

func writeHookResponse(decision string) error {
	output := hookOutput{
		HookSpecificOutput: hookDecision{
			PermissionDecision: decision,
		},
	}

	data, err := json.Marshal(output)
	if err != nil {
		return err
	}

	fmt.Println(string(data))
	return nil
}

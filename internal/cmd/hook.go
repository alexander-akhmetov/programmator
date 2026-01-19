package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/spf13/cobra"

	"github.com/alexander-akhmetov/programmator/internal/permission"
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
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return writeHookResponse("deny")
	}

	var hi hookInput
	if err := json.Unmarshal(input, &hi); err != nil {
		return writeHookResponse("deny")
	}

	req := &permission.Request{
		SessionID: hi.SessionID,
		ToolName:  hi.ToolName,
		ToolInput: hi.ToolInput,
		ToolUseID: hi.ToolUseID,
	}

	decision := sendToServer(socketPath, req)

	claudeDecision := "deny"
	if decision == permission.DecisionAllow {
		claudeDecision = "allow"
	}

	return writeHookResponse(claudeDecision)
}

func sendToServer(socket string, req *permission.Request) permission.Decision {
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return permission.DecisionDeny
	}
	defer conn.Close()

	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(req); err != nil {
		return permission.DecisionDeny
	}

	var resp permission.Response
	decoder := json.NewDecoder(conn)
	if err := decoder.Decode(&resp); err != nil {
		return permission.DecisionDeny
	}

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

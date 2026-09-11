package tools

// Action tools (semantic modeling module): let the agent discover and run
// declarative action types attached to Cube models. An action has an input
// schema, optional preconditions, and a webhook backing. The engine enforces
// data-group permissions, type coercion, and full audit — the same identity
// model as cube_query. Typical flow: cube_meta → cube_query → action_meta →
// action_run.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/semantic"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/utils"
)

// Tool name constants (consumed by agent_service.go registration).
const (
	ToolActionMeta = "action_meta"
	ToolActionRun  = "action_run"
)

// ---- action_meta ----

const actionMetaDescription = `List the declarative actions visible to the current identity in the
semantic modeling module.

## Usage
- Call this after querying data (cube_query) when you need to perform an
  operation (e.g. create a ticket, update a status)
- Each action lists its title, description, input fields (name/type/required),
  and preconditions — use these to decide which action to run and what
  arguments to assemble
- Results are filtered by the current user's data permissions; actions you
  cannot see are ones you have no access to, so do not guess action names

## Output
Each action lists its object_types (linked Cube models), input fields with
type/description, and any preconditions that must hold before execution.`

// ActionMetaInput is the input for the action_meta tool.
type ActionMetaInput struct {
	Action string `json:"action" jsonschema:"optional, filter action name by keyword"`
}

// ActionMetaTool lists the actions visible to the caller.
type ActionMetaTool struct {
	BaseTool
	boundModels []string
}

// NewActionMetaTool returns nil when the semantic module is disabled.
func NewActionMetaTool(boundModels []string) *ActionMetaTool {
	if !semantic.Enabled() {
		return nil
	}
	return &ActionMetaTool{
		BaseTool:    NewBaseTool(ToolActionMeta, actionMetaDescription, utils.GenerateSchema[ActionMetaInput]()),
		boundModels: boundModels,
	}
}

// Execute lists permission-filtered actions.
func (t *ActionMetaTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	var input ActionMetaInput
	_ = json.Unmarshal(args, &input)

	tenantID, userID, ok := cubeIdentity(ctx)
	if !ok {
		return &types.ToolResult{Success: false, Error: "missing user identity context"}, fmt.Errorf("missing identity")
	}
	resp, err := semantic.ActionMetaForUser(ctx, tenantID, userID, t.boundModels)
	if err != nil {
		return &types.ToolResult{Success: false, Error: err.Error()}, err
	}
	var b strings.Builder
	if len(resp.Actions) == 0 {
		b.WriteString("The current identity has no visible actions. Either no actions have been\n")
		b.WriteString("configured, or the current user has not been granted any data group.\n")
	}
	for _, a := range resp.Actions {
		if input.Action != "" && !strings.Contains(strings.ToLower(a.Name), strings.ToLower(input.Action)) {
			continue
		}
		fmt.Fprintf(&b, "### %s\n", a.Name)
		if a.Title != "" {
			fmt.Fprintf(&b, "title: %s\n", a.Title)
		}
		if a.Description != "" {
			fmt.Fprintf(&b, "description: %s\n", a.Description)
		}
		if len(a.ObjectTypes) > 0 {
			fmt.Fprintf(&b, "models: %s\n", strings.Join(a.ObjectTypes, ", "))
		}
		if len(a.InputFields) > 0 {
			b.WriteString("inputs:\n")
			for _, f := range a.InputFields {
				req := "optional"
				if f.Required {
					req = "required"
				}
				desc := f.Description
				if len(f.Enum) > 0 {
					desc += " (options: " + strings.Join(f.Enum, " / ") + ")"
				}
				fmt.Fprintf(&b, "  - %s (%s, %s): %s\n", f.Column, f.Type, req, desc)
			}
		}
		if len(a.Preconditions) > 0 {
			b.WriteString("preconditions:\n")
			for _, pc := range a.Preconditions {
				fmt.Fprintf(&b, "  - %s\n", pc.Description)
			}
		}
		b.WriteString("\n")
	}
	return &types.ToolResult{
		Success: true,
		Output:  b.String(),
		Data: map[string]interface{}{
			"actions":      resp.Actions,
			"display_type": "action_meta",
		},
	}, nil
}

// ---- action_run ----

const actionRunDescription = `Execute a declarative action via the semantic layer.

## Usage
1. Call action_meta first to discover available actions and their input fields
2. Assemble the arguments: each field must match the declared type; required
   fields must be provided; enum fields must use one of the allowed values
3. The action runs under the current user's data-group permissions — if you
   get a "permission denied" error, the action is not available to this user

## Examples
{"action": "create_ticket", "fields": {"part_sn": "SN001", "issue_desc": "power failure"}}
{"action": "update_status", "fields": {"id": 42, "status": "已解决"}}

## Notes
- Preconditions (if any) are evaluated before execution; if a precondition
  is not satisfied the action will be rejected with an explanation
- The webhook response is returned as output; large responses are truncated
- All executions are fully audited (who / what / params / result)`

// ActionRunInput is the input for the action_run tool.
type ActionRunInput struct {
	Action string                 `json:"action" jsonschema:"action name to execute"`
	Fields map[string]interface{} `json:"fields" jsonschema:"input arguments for the action"`
}

// ActionRunTool executes an action under the caller's identity.
type ActionRunTool struct {
	BaseTool
	boundModels []string
}

// NewActionRunTool returns nil when the semantic module is disabled.
func NewActionRunTool(boundModels []string) *ActionRunTool {
	if !semantic.Enabled() {
		return nil
	}
	return &ActionRunTool{
		BaseTool:    NewBaseTool(ToolActionRun, actionRunDescription, utils.GenerateSchema[ActionRunInput]()),
		boundModels: boundModels,
	}
}

// Execute runs the action under the caller's identity.
func (t *ActionRunTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	logger.Infof(ctx, "[Tool][ActionRun] Execute started")
	var input ActionRunInput
	if len(args) > 0 {
		// Tolerate stringified object args (same client quirk as cube_query).
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(args, &raw); err == nil {
			if v, ok := raw["fields"]; ok {
				var str string
				if json.Unmarshal(v, &str) == nil && str != "" {
					var decoded interface{}
					if json.Unmarshal([]byte(str), &decoded) == nil {
						if nv, err := json.Marshal(decoded); err == nil {
							raw["fields"] = nv
						}
					}
				}
			}
			if fixed, err := json.Marshal(raw); err == nil {
				args = fixed
			}
		}
		if err := json.Unmarshal(args, &input); err != nil {
			return &types.ToolResult{Success: false, Error: fmt.Sprintf("failed to parse arguments: %v", err)}, err
		}
	}
	if input.Action == "" {
		return &types.ToolResult{Success: false, Error: "action name is required"}, fmt.Errorf("action name is required")
	}

	tenantID, userID, ok := cubeIdentity(ctx)
	if !ok {
		return &types.ToolResult{Success: false, Error: "missing user identity context"}, fmt.Errorf("missing identity")
	}

	resp, err := semantic.ActionRunForUser(ctx, tenantID, userID, input.Action, input.Fields)
	if err != nil {
		return &types.ToolResult{Success: false, Error: err.Error()}, err
	}
	logger.Infof(ctx, "[Tool][ActionRun] action=%s http_status=%d success=%v", input.Action, resp.HTTPStatus, resp.Success)
	return &types.ToolResult{
		Success: resp.Success,
		Output:  resp.Output,
		Data: map[string]interface{}{
			"action":       input.Action,
			"http_status":  resp.HTTPStatus,
			"display_type": "action_run",
		},
	}, nil
}

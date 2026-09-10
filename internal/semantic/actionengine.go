package semantic

// Action execution engine: input coercion, precondition evaluation, and
// webhook dispatch. The engine runs under the caller's data-group identity
// (same SecCtxFor / WithUserContext used by Cube queries) and audits every
// execution. See README.md §Actions.
//
// Security boundaries (enforced in-engine, declaration cannot bypass):
//   - Webhook URL validated for SSRF at save + dispatch time;
//   - HTTP client uses the platform SSRF-safe transport (private IP blocking,
//     DNS rebinding protection, redirect re-validation);
//   - Auth secret AES-256-GCM encrypted at rest, decrypted only at dispatch;
//   - Input types coerced + enum-enforced before dispatch;
//   - Permission checked in-engine (group intersection, admin always passes);
//   - Preconditions evaluated before the backing runs.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// actionHTTPTimeout caps a single webhook dispatch.
const actionHTTPTimeout = 30 * time.Second

// actionHTTPClient is the SSRF-safe client shared across dispatches.
var actionHTTPClient = secutils.NewSSRFSafeHTTPClient(secutils.SSRFSafeHTTPClientConfig{
	Timeout:      actionHTTPTimeout,
	MaxRedirects: 3,
})

// ---- agent entry points ----

// ActionMetaResponse is the action_meta tool output shape.
type ActionMetaResponse struct {
	Actions []ActionMetaEntry `json:"actions"`
}

// ActionMetaEntry is one visible action in the meta listing.
type ActionMetaEntry struct {
	Name          string        `json:"name"`
	Title         string        `json:"title"`
	Description   string        `json:"description"`
	ObjectTypes   []string      `json:"object_types"`
	InputFields   []ActionField `json:"input_fields"`
	Preconditions []Precondition `json:"preconditions,omitempty"`
}

// ActionRunResponse is the action_run tool output shape.
type ActionRunResponse struct {
	Success  bool   `json:"success"`
	Output   string `json:"output"`
	HTTPStatus int  `json:"http_status,omitempty"`
}

// ActionMetaForUser lists actions visible to the caller, filtered by the
// bound-model scope (empty boundModels = all visible).
func ActionMetaForUser(ctx context.Context, tenantID uint64, userID string, boundModels []string) (*ActionMetaResponse, error) {
	e := Default()
	if e == nil {
		return nil, fmt.Errorf("semantic modeling module is not enabled")
	}
	sec, err := e.SecCtxFor(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	groupSet := make(map[string]bool, len(sec.Groups))
	for _, g := range sec.Groups {
		groupSet[g] = true
	}
	isAdmin := groupSet[AdminGroup]

	actions, err := e.repo.ActiveActionsByModels(ctx, tenantID, boundModels)
	if err != nil {
		return nil, err
	}
	out := make([]ActionMetaEntry, 0, len(actions))
	for _, a := range actions {
		if !isAdmin && !actionAllowsGroup(a, groupSet) {
			continue
		}
		var fields []ActionField
		_ = json.Unmarshal(a.InputSchema, &fields)
		var preconds []Precondition
		_ = json.Unmarshal(a.Preconditions, &preconds)
		out = append(out, ActionMetaEntry{
			Name: a.Name, Title: a.Title, Description: a.Description,
			ObjectTypes: StringList(a.ObjectTypes),
			InputFields: fields, Preconditions: preconds,
		})
	}
	return &ActionMetaResponse{Actions: out}, nil
}

// ActionRunForUser executes one action under the caller's identity.
func ActionRunForUser(
	ctx context.Context,
	tenantID uint64,
	userID string,
	actionName string,
	args map[string]interface{},
) (*ActionRunResponse, error) {
	e := Default()
	if e == nil {
		return nil, fmt.Errorf("semantic modeling module is not enabled")
	}
	a, err := e.repo.FindActionByName(ctx, tenantID, actionName)
	if err != nil {
		return nil, err
	}
	if a.Status != "active" {
		return nil, fmt.Errorf("action %s is not active", actionName)
	}
	sec, err := e.SecCtxFor(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	groupSet := make(map[string]bool, len(sec.Groups))
	for _, g := range sec.Groups {
		groupSet[g] = true
	}
	if !groupSet[AdminGroup] && !actionAllowsGroup(a, groupSet) {
		return nil, fmt.Errorf("permission denied: you are not in any group allowed to run action %s", actionName)
	}
	// 1. coerce inputs
	var fields []ActionField
	_ = json.Unmarshal(a.InputSchema, &fields)
	input, err := coerceActionInputs(fields, args)
	if err != nil {
		return nil, fmt.Errorf("input validation failed: %w", err)
	}
	// 2. preconditions
	var preconds []Precondition
	_ = json.Unmarshal(a.Preconditions, &preconds)
	if len(preconds) > 0 {
		if err := e.evalPreconditions(ctx, tenantID, userID, preconds, input); err != nil {
			e.audit(ctx, tenantID, userID, AuditActionExecute, "action:"+a.Name,
				map[string]interface{}{"result": "precondition_failed", "error": err.Error()})
			return nil, err
		}
	}
	// 3. dispatch backing
	result, err := e.dispatchWebhook(ctx, a, input)
	// 4. audit
	detail := map[string]interface{}{
		"input":  input,
		"result": "ok",
	}
	if err != nil {
		detail["result"] = "error"
		detail["error"] = err.Error()
	} else {
		detail["http_status"] = result.HTTPStatus
	}
	e.audit(ctx, tenantID, userID, AuditActionExecute, "action:"+a.Name, detail)
	return result, err
}

// actionAllowsGroup checks whether the action's allowed_groups intersect
// the caller's group set.
func actionAllowsGroup(a *SemanticAction, groups map[string]bool) bool {
	for _, g := range StringList(a.AllowedGroups) {
		if groups[g] {
			return true
		}
	}
	return false
}

// ---- input coercion (ported from cube-mcp bizdb, datasource-agnostic) ----

// coerceActionInputs validates, type-converts, and applies defaults to the
// declared input fields. Returns a map of column→coerced value.
func coerceActionInputs(fields []ActionField, args map[string]interface{}) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	for _, f := range fields {
		v, ok := args[f.Column]
		if !ok || v == nil {
			if f.Default != nil {
				cv, err := coerceValue(f, f.Default)
				if err != nil {
					return nil, fmt.Errorf("field %s default invalid: %v", f.Column, err)
				}
				out[f.Column] = cv
				continue
			}
			if f.Required {
				return nil, fmt.Errorf("missing required field: %s", f.Column)
			}
			continue
		}
		cv, err := coerceValue(f, v)
		if err != nil {
			return nil, fmt.Errorf("field %s: %v", f.Column, err)
		}
		out[f.Column] = cv
	}
	return out, nil
}

// coerceValue converts a single value to the declared field type and
// enforces enum constraints. Mirrors cube-mcp bizdb's coerce().
func coerceValue(f ActionField, v interface{}) (interface{}, error) {
	var result interface{}
	switch f.Type {
	case "string", "date":
		result = anyToString(v)
	case "int":
		switch t := v.(type) {
		case int:
			result = int64(t)
		case int64:
			result = t
		case float64:
			if t != float64(int64(t)) {
				return nil, fmt.Errorf("expected integer, got %v", v)
			}
			result = int64(t)
		case string:
			n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("expected integer, got %q", t)
			}
			result = n
		default:
			return nil, fmt.Errorf("expected integer, got %T", v)
		}
	case "float":
		switch t := v.(type) {
		case float64:
			result = t
		case int64:
			result = float64(t)
		case int:
			result = float64(t)
		case string:
			n, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
			if err != nil {
				return nil, fmt.Errorf("expected number, got %q", t)
			}
			result = n
		default:
			return nil, fmt.Errorf("expected number, got %T", v)
		}
	case "bool":
		switch t := v.(type) {
		case bool:
			result = t
		case string:
			b, err := strconv.ParseBool(strings.TrimSpace(t))
			if err != nil {
				return nil, fmt.Errorf("expected boolean, got %q", t)
			}
			result = b
		default:
			return nil, fmt.Errorf("expected boolean, got %T", v)
		}
	case "json":
		if s, ok := v.(string); ok {
			if !json.Valid([]byte(s)) {
				return nil, fmt.Errorf("expected valid JSON string")
			}
			result = s
		} else {
			b, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("failed to serialize JSON: %v", err)
			}
			result = string(b)
		}
	default:
		return nil, fmt.Errorf("unsupported type: %s", f.Type)
	}
	// enum enforcement (string type only)
	if len(f.Enum) > 0 {
		s := anyToString(result)
		found := false
		for _, allowed := range f.Enum {
			if s == allowed {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("value %q is not in allowed values: %s", s, strings.Join(f.Enum, ", "))
		}
	}
	return result, nil
}

// anyToString stringifies any scalar for enum checks.
func anyToString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(t, 10)
	case int:
		return strconv.Itoa(t)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprint(t)
	}
}

// ---- precondition evaluation ----

// evalPreconditions runs each precondition as a Cube query (with input
// template substitution) and checks the expect clause.
func (e *Engine) evalPreconditions(
	ctx context.Context,
	tenantID uint64,
	userID string,
	preconds []Precondition,
	input map[string]interface{},
) error {
	for _, pc := range preconds {
		resolved, err := resolvePreconditionQuery(pc.Query, input)
		if err != nil {
			return fmt.Errorf("precondition %q: %w", pc.Description, err)
		}
		resp, err := e.QueryForUser(ctx, tenantID, userID, resolved)
		if err != nil {
			return fmt.Errorf("precondition %q query failed: %w", pc.Description, err)
		}
		rows := 0
		if resp != nil {
			rows = len(resp.Data)
		}
		switch pc.Expect {
		case "rows_gt_0":
			if rows == 0 {
				return fmt.Errorf("precondition not satisfied: %s", pc.Description)
			}
		case "rows_eq_0":
			if rows > 0 {
				return fmt.Errorf("precondition not satisfied: %s", pc.Description)
			}
		default:
			return fmt.Errorf("precondition %q has invalid expect %q", pc.Description, pc.Expect)
		}
	}
	return nil
}

// resolvePreconditionQuery substitutes {{input.field}} templates in the
// precondition's filter values with the coerced input values.
func resolvePreconditionQuery(q PreviewQuery, input map[string]interface{}) (*PreviewQuery, error) {
	for i := range q.Filters {
		for j, v := range q.Filters[i].Values {
			resolved, err := resolveTemplate(v, input)
			if err != nil {
				return nil, err
			}
			q.Filters[i].Values[j] = resolved
		}
	}
	return &q, nil
}

// resolveTemplate substitutes {{input.field}} references in a precondition
// filter value with the coerced input values. Plain string replacement (not
// text/template): Go templates would treat the bare `input` identifier as a
// function, and the documented {{input.field}} syntax must stay literal.
var precondVarRe = regexp.MustCompile(`\{\{input\.([a-zA-Z0-9_]+)\}\}`)

func resolveTemplate(s string, input map[string]interface{}) (string, error) {
	if !strings.Contains(s, "{{input.") {
		return s, nil
	}
	var firstErr error
	out := precondVarRe.ReplaceAllStringFunc(s, func(m string) string {
		key := precondVarRe.FindStringSubmatch(m)[1]
		v, ok := input[key]
		if !ok {
			if firstErr == nil {
				firstErr = fmt.Errorf("input field %q not provided", key)
			}
			return ""
		}
		return anyToString(v)
	})
	return out, firstErr
}

// ---- webhook dispatch ----

// dispatchWebhook renders the body template, injects the auth secret, and
// sends the HTTP request through the SSRF-safe client.
func (e *Engine) dispatchWebhook(ctx context.Context, a *SemanticAction, input map[string]interface{}) (*ActionRunResponse, error) {
	var cfg WebhookConfig
	if err := json.Unmarshal(a.Backend, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse backend config: %w", err)
	}
	// Decrypt secret.
	if cfg.SecretEncrypted != "" {
		plain, err := decryptString(cfg.SecretEncrypted)
		if err == nil {
			// Inject into headers that contain {{secret}}.
			for k, v := range cfg.Headers {
				if strings.Contains(v, "{{secret}}") {
					cfg.Headers[k] = strings.ReplaceAll(v, "{{secret}}", plain)
				}
			}
		} else {
			logger.Warnf(ctx, "[semantic] action %s secret decrypt failed: %v", a.Name, err)
		}
	}
	// Re-validate URL at dispatch time (SSRF defense in depth).
	if err := validateWebhookURL(cfg.URL); err != nil {
		return nil, err
	}
	// Build request body: use the template if provided, otherwise auto-
	// assemble a JSON object from the coerced input fields (field name → value).
	// The template is an advanced override for non-trivial structures (nested,
	// renamed, fixed values); the common case needs no template at all.
	var bodyReader io.Reader
	if cfg.BodyTemplate != "" {
		t, err := template.New("body").Parse(cfg.BodyTemplate)
		if err != nil {
			return nil, fmt.Errorf("body template parse error: %w", err)
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, input); err != nil {
			return nil, fmt.Errorf("body template render error: %w", err)
		}
		bodyReader = &buf
	} else if len(input) > 0 && (cfg.Method == "" || cfg.Method == http.MethodPost ||
		cfg.Method == http.MethodPut || cfg.Method == http.MethodPatch) {
		// Auto-assemble: {"field_name": value, ...}
		bodyBytes, err := json.Marshal(input)
		if err != nil {
			return nil, fmt.Errorf("failed to build request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}
	method := cfg.Method
	if method == "" {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, cfg.URL, bodyReader)
	if err != nil {
		return nil, err
	}
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	resp, err := actionHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webhook request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB cap

	// Check success status.
	successStatuses := cfg.SuccessStatus
	if len(successStatuses) == 0 {
		successStatuses = []int{200, 201}
	}
	ok := false
	for _, code := range successStatuses {
		if resp.StatusCode == code {
			ok = true
			break
		}
	}
	bodyStr := truncateBody(string(body), 4096)
	if !ok {
		return &ActionRunResponse{
			Success: false, Output: bodyStr, HTTPStatus: resp.StatusCode,
		}, fmt.Errorf("webhook returned HTTP %d: %s", resp.StatusCode, bodyStr)
	}
	return &ActionRunResponse{
		Success: true, Output: bodyStr, HTTPStatus: resp.StatusCode,
	}, nil
}

// truncateBody caps a response body string for the agent output.
func truncateBody(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// ---- helpers used by service.go ----

// parseWebhookConfig extracts a WebhookConfig from the backend map.
func parseWebhookConfig(backend map[string]interface{}) (*WebhookConfig, error) {
	raw, err := json.Marshal(backend)
	if err != nil {
		return nil, err
	}
	var cfg WebhookConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse webhook config: %w", err)
	}
	if cfg.URL == "" {
		return nil, fmt.Errorf("webhook URL is required")
	}
	return &cfg, nil
}

// validateWebhookURL checks the URL scheme and runs SSRF validation.
func validateWebhookURL(rawURL string) error {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return fmt.Errorf("webhook URL is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("webhook URL must be a valid http(s) URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("webhook URL must use http or https")
	}
	if err := secutils.ValidateURLForSSRF(trimmed); err != nil {
		if hint := secutils.FormatSSRFError("Action webhook URL", trimmed, err); hint != "" {
			return fmt.Errorf("%s", hint)
		}
		return err
	}
	return nil
}

// encryptString encrypts a plaintext string with the platform AES key.
func encryptString(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	if key := secutils.GetAESKey(); key != nil {
		return secutils.EncryptAESGCM(plain, key)
	}
	return plain, nil // no key configured = store plaintext (platform convention)
}

// decryptString is the inverse of encryptString.
func decryptString(enc string) (string, error) {
	if enc == "" {
		return "", nil
	}
	if key := secutils.GetAESKey(); key != nil {
		return secutils.DecryptAESGCM(enc, key)
	}
	return enc, nil
}

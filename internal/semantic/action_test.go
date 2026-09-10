package semantic

// Tests for the Action feature: CRUD validation, input coercion, webhook
// dispatch (secret injection + body assembly), permission checks,
// precondition evaluation, and the agent-facing meta listing. Runs against
// the same in-memory sqlite + fakeCube harness as service_test.go.

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	secutils "github.com/Tencent/WeKnora/internal/utils"
)

func newTestEngineWithActions(t *testing.T) (*Engine, *fakeCube) {
	t.Helper()
	e, f := newTestEngine(t)
	require.NoError(t, e.repo.db.AutoMigrate(&SemanticAction{}))
	// ActionRunForUser / ActionMetaForUser resolve the module singleton via
	// Default() — publish it for the test's lifetime, mirroring Register().
	mu.Lock()
	prev := engine
	engine = e
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		engine = prev
		mu.Unlock()
	})
	return e, f
}

// allowTestSSRF whitelists the hosts used by action tests: 127.0.0.1 for
// httptest webhook servers, erp.example.com as the stand-in business API.
func allowTestSSRF(t *testing.T) {
	t.Helper()
	secutils.SetSSRFWhitelistFromRaw("127.0.0.1,::1,erp.example.com")
	t.Cleanup(func() { secutils.SetSSRFWhitelistFromRaw("") })
}

func validActionInput(name string, webhookURL string) *ActionInput {
	return &ActionInput{
		Name: name, Title: "Create Ticket",
		Description: "creates a ticket via ERP",
		ObjectTypes: []string{"orders"},
		InputSchema: []ActionField{
			{Column: "part_sn", Type: "string", Required: true, Description: "part serial"},
			{Column: "quantity", Type: "int", Default: 1},
			{Column: "status", Type: "string", Enum: []string{"待处理", "已解决"}, Default: "待处理"},
		},
		Backend: map[string]interface{}{
			"type": "webhook", "url": webhookURL, "method": "POST",
		},
		AllowedGroups: []string{"sales"},
	}
}

// seedGroupMember puts user u1 into data group "sales".
func seedGroupMember(t *testing.T, e *Engine, tenant uint64) {
	t.Helper()
	g, err := e.CreateGroup(context.Background(), "u1", tenant, &DataGroup{Name: "sales", Title: "Sales"})
	require.NoError(t, err)
	require.NoError(t, e.repo.ReplaceGroupMembers(context.Background(), tenant, g.ID, []string{"u1"}))
}

func TestActionCRUDValidation(t *testing.T) {
	allowTestSSRF(t)
	e, _ := newTestEngineWithActions(t)
	ctx := context.Background()
	const tenant = uint64(1)

	// Seed the referenced model so object_types validation passes.
	seedConnectionAndModel(t, e, tenant, testDraftYAML, []string{"sales"}, "")

	// Invalid slug rejected.
	_, err := e.CreateAction(ctx, "u1", tenant, validActionInput("Bad-Slug", "https://erp.example.com/hook"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "action name must match")

	// Unknown object type rejected.
	bad := validActionInput("create_ticket", "https://erp.example.com/hook")
	bad.ObjectTypes = []string{"no_such_model"}
	_, err = e.CreateAction(ctx, "u1", tenant, bad)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a known Cube model")

	// Non-webhook backend rejected.
	bad = validActionInput("create_ticket", "https://erp.example.com/hook")
	bad.Backend = map[string]interface{}{"type": "sql_write"}
	_, err = e.CreateAction(ctx, "u1", tenant, bad)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported backend type")

	// Invalid field type rejected.
	bad = validActionInput("create_ticket", "https://erp.example.com/hook")
	bad.InputSchema = []ActionField{{Column: "x", Type: "vector"}}
	_, err = e.CreateAction(ctx, "u1", tenant, bad)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid type")

	// Missing webhook URL rejected.
	bad = validActionInput("create_ticket", "https://erp.example.com/hook")
	bad.Backend = map[string]interface{}{"type": "webhook"}
	_, err = e.CreateAction(ctx, "u1", tenant, bad)
	require.Error(t, err)

	// Happy path + duplicate slug conflict.
	a, err := e.CreateAction(ctx, "u1", tenant, validActionInput("create_ticket", "https://erp.example.com/hook"))
	require.NoError(t, err)
	assert.Equal(t, "active", a.Status)
	assert.Equal(t, []string{"orders"}, StringList(a.ObjectTypes))
	assert.Equal(t, []string{"sales"}, StringList(a.AllowedGroups))

	_, err = e.CreateAction(ctx, "u1", tenant, validActionInput("create_ticket", "https://erp.example.com/hook"))
	require.Error(t, err)
	var ce *ConflictError
	assert.ErrorAs(t, err, &ce)

	// The plaintext secret never reaches any JSON payload, and the
	// client-facing redaction strips the encrypted blob while flagging
	// HasSecret for the UI. A 32-byte SYSTEM_AES_KEY turns on real
	// encryption (without it the platform stores plaintext by design).
	t.Setenv("SYSTEM_AES_KEY", "0123456789abcdef0123456789abcdef")
	withSecret := validActionInput("with_secret", "https://erp.example.com/hook")
	withSecret.SecretValue = "super-token-xyz"
	a2, err := e.CreateAction(ctx, "u1", tenant, withSecret)
	require.NoError(t, err)
	body, err := json.Marshal(a2)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "super-token-xyz", "plaintext secret must never serialize")
	assert.True(t, redactActionSecret(a2).HasSecret)
	redacted, err := json.Marshal(a2)
	require.NoError(t, err)
	assert.NotContains(t, string(redacted), "secret_encrypted", "encrypted secret must be redacted for clients")

	// Update with empty secret keeps the stored one; rename conflict surfaced.
	upd := validActionInput("", "https://erp.example.com/hook2")
	upd.Name = "create_ticket2"
	upd.Title = "Renamed"
	_, err = e.UpdateAction(ctx, "u1", tenant, a.ID, upd)
	require.NoError(t, err)

	err = e.DeleteAction(ctx, "u1", tenant, a.ID)
	require.NoError(t, err)
	_, err = e.repo.FindAction(ctx, tenant, a.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestCoerceActionInputs(t *testing.T) {
	fields := []ActionField{
		{Column: "sn", Type: "string", Required: true},
		{Column: "qty", Type: "int", Default: 1},
		{Column: "price", Type: "float"},
		{Column: "active", Type: "bool"},
		{Column: "level", Type: "string", Enum: []string{"low", "high"}},
		{Column: "payload", Type: "json"},
	}

	// Missing required → error.
	_, err := coerceActionInputs(fields, map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")

	// Defaults applied, stringified ints/bools coerced, enum enforced.
	out, err := coerceActionInputs(fields, map[string]interface{}{
		"sn": "SN1", "qty": "3", "price": "9.5", "active": "true", "level": "high",
		"payload": map[string]interface{}{"k": "v"},
	})
	require.NoError(t, err)
	assert.Equal(t, "SN1", out["sn"])
	assert.Equal(t, int64(3), out["qty"])
	assert.Equal(t, 9.5, out["price"])
	assert.Equal(t, true, out["active"])
	assert.Equal(t, "high", out["level"])
	assert.Equal(t, `{"k":"v"}`, out["payload"])

	// Float int rejection and enum violation.
	_, err = coerceActionInputs(fields, map[string]interface{}{"sn": "s", "qty": 1.5})
	require.Error(t, err)
	_, err = coerceActionInputs(fields, map[string]interface{}{"sn": "s", "level": "medium"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in allowed values")
}

func TestActionWebhookDispatch(t *testing.T) {
	allowTestSSRF(t)
	e, _ := newTestEngineWithActions(t)

	var gotAuth, gotBody string
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotAuth = r.Header.Get("Authorization")
		gotBody = string(b)
		gotMethod = r.Method
		if r.URL.Path == "/fail" {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("upstream broken"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ticket_id": 42}`))
	}))
	t.Cleanup(srv.Close)

	// Secret round-trips through storage: encrypt → marshal → the dispatch
	// path unmarshals and injects it into headers containing {{secret}}.
	backend := WebhookConfig{
		URL: srv.URL + "/hook", Method: "POST",
		Headers: map[string]string{"Authorization": "Bearer {{secret}}"},
	}
	enc, err := encryptString("tok-123")
	require.NoError(t, err)
	backend.SecretEncrypted = enc

	a := &SemanticAction{
		Name:         "create_ticket",
		Backend: mustJSON(backend),
	}
	require.Contains(t, string(a.Backend), "secret_encrypted", "storage JSON must persist the encrypted secret")
	input := map[string]interface{}{"part_sn": "SN1", "quantity": int64(2)}

	// Success: secret injected into header, body auto-assembled from input.
	resp, err := e.dispatchWebhook(context.Background(), a, input)
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, http.StatusOK, resp.HTTPStatus)
	assert.Equal(t, "Bearer tok-123", gotAuth)
	assert.Equal(t, "POST", gotMethod)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(gotBody), &body))
	assert.Equal(t, "SN1", body["part_sn"])
	assert.Equal(t, float64(2), body["quantity"])
	assert.Contains(t, resp.Output, "ticket_id")

	// Non-2xx → error with the response body surfaced.
	failAction := &SemanticAction{
		Name:    "create_ticket",
		Backend: mustJSON(map[string]interface{}{"type": "webhook", "url": srv.URL + "/fail", "method": "POST"}),
	}
	resp, err = e.dispatchWebhook(context.Background(), failAction, input)
	require.Error(t, err)
	assert.False(t, resp.Success)
	assert.Equal(t, http.StatusBadGateway, resp.HTTPStatus)
	assert.Contains(t, err.Error(), "upstream broken")
}

func TestActionRunForUserPermissionAndAudit(t *testing.T) {
	allowTestSSRF(t)
	e, _ := newTestEngineWithActions(t)
	ctx := context.Background()
	const tenant = uint64(1)

	seedConnectionAndModel(t, e, tenant, testDraftYAML, []string{"sales"}, "")
	seedGroupMember(t, e, tenant)

	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	in := validActionInput("create_ticket", srv.URL)
	_, err := e.CreateAction(ctx, "u1", tenant, in)
	require.NoError(t, err)

	// A user outside "sales" is denied before the webhook fires.
	_, err = ActionRunForUser(ctx, tenant, "u2", "create_ticket", map[string]interface{}{"part_sn": "SN1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
	assert.False(t, hit)

	// Missing required input rejected before the webhook fires.
	_, err = ActionRunForUser(ctx, tenant, "u1", "create_ticket", map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
	assert.False(t, hit)

	// Authorized caller with valid input executes and is audited.
	resp, err := ActionRunForUser(ctx, tenant, "u1", "create_ticket", map[string]interface{}{"part_sn": "SN1"})
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.True(t, hit)

	audits, err := e.ListAudits(ctx, tenant, 10)
	require.NoError(t, err)
	found := false
	for _, a := range audits {
		if a.Action == AuditActionExecute && a.Target == "action:create_ticket" {
			found = true
		}
	}
	assert.True(t, found, "expected an action.execute audit record")

	// Unknown action → not found.
	_, err = ActionRunForUser(ctx, tenant, "u1", "no_such_action", nil)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestActionPreconditions(t *testing.T) {
	allowTestSSRF(t)
	e, f := newTestEngineWithActions(t)
	ctx := context.Background()
	const tenant = uint64(1)

	seedConnectionAndModel(t, e, tenant, testDraftYAML, []string{"sales"}, "")
	seedGroupMember(t, e, tenant)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	preconds := []Precondition{{
		Description: "open tickets for this SN must exist",
		Query: PreviewQuery{
			Measures: []string{"orders.count"},
			Filters:  []QueryFilter{{Member: "orders.part_sn", Operator: "equals", Values: []string{"{{input.part_sn}}"}}},
		},
		Expect: "rows_gt_0",
	}}

	in := validActionInput("close_ticket", srv.URL)
	in.Preconditions = preconds
	_, err := e.CreateAction(ctx, "u1", tenant, in)
	require.NoError(t, err)

	args := map[string]interface{}{"part_sn": "SN1"}

	// Query returns no rows → precondition blocks execution.
	f.setLoadRows(nil)
	_, err = ActionRunForUser(ctx, tenant, "u1", "close_ticket", args)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "precondition not satisfied")

	// Query returns rows → passes, webhook fires.
	f.setLoadRows([]map[string]interface{}{{"orders.count": 3}})
	_, err = ActionRunForUser(ctx, tenant, "u1", "close_ticket", args)
	require.NoError(t, err)

	// Missing template variable surfaced as a validation error.
	_, err = ActionRunForUser(ctx, tenant, "u1", "close_ticket", map[string]interface{}{"part_sn": "SN1", "extra": "x"})
	require.NoError(t, err) // part_sn provided, fine

	_, err = ActionRunForUser(ctx, tenant, "u1", "close_ticket", nil)
	require.Error(t, err) // required part_sn missing → coercion error first
}

func TestResolvePreconditionTemplate(t *testing.T) {
	input := map[string]interface{}{"part_sn": "SN1", "qty": int64(3)}

	out, err := resolveTemplate("orders.sn = {{input.part_sn}}", input)
	require.NoError(t, err)
	assert.Equal(t, "orders.sn = SN1", out)

	out, err = resolveTemplate("no template here", input)
	require.NoError(t, err)
	assert.Equal(t, "no template here", out)

	out, err = resolveTemplate("qty {{input.qty}} and {{input.part_sn}}", input)
	require.NoError(t, err)
	assert.Equal(t, "qty 3 and SN1", out)

	_, err = resolveTemplate("{{input.missing}}", input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not provided")
}

func TestActionMetaForUser(t *testing.T) {
	allowTestSSRF(t)
	e, _ := newTestEngineWithActions(t)
	ctx := context.Background()
	const tenant = uint64(1)

	seedConnectionAndModel(t, e, tenant, testDraftYAML, []string{"sales"}, "")
	seedGroupMember(t, e, tenant)

	// A second model so the second action has a valid object type.
	conn, err := e.repo.FindConnectionByName(ctx, tenant, "sales_db")
	require.NoError(t, err)
	otherYAML := strings.Replace(testDraftYAML, "name: orders", "name: other_model", 1)
	_, err = e.CreateModel(ctx, "u1", tenant, &SaveModelInput{
		Name: "other_model", Title: "Other", ConnectionID: conn.ID, DraftYAML: otherYAML,
	})
	require.NoError(t, err)

	srvURL := "https://erp.example.com/hook"
	_, err = e.CreateAction(ctx, "u1", tenant, validActionInput("create_ticket", srvURL))
	require.NoError(t, err)
	in2 := validActionInput("update_status", srvURL)
	in2.ObjectTypes = []string{"other_model"}
	_, err = e.CreateAction(ctx, "u1", tenant, in2)
	require.NoError(t, err)

	// u1 (sales) sees create_ticket; boundModels narrows further.
	meta, err := ActionMetaForUser(ctx, tenant, "u1", nil)
	require.NoError(t, err)
	names := []string{}
	for _, a := range meta.Actions {
		names = append(names, a.Name)
	}
	assert.Equal(t, []string{"create_ticket", "update_status"}, names)

	meta, err = ActionMetaForUser(ctx, tenant, "u1", []string{"orders"})
	require.NoError(t, err)
	require.Len(t, meta.Actions, 1)
	assert.Equal(t, "create_ticket", meta.Actions[0].Name)
	assert.Equal(t, "orders", meta.Actions[0].ObjectTypes[0])
	require.Len(t, meta.Actions[0].InputFields, 3)

	// Bound to other_model → only update_status (attached to it) is visible.
	meta, err = ActionMetaForUser(ctx, tenant, "u1", []string{"other_model"})
	require.NoError(t, err)
	require.Len(t, meta.Actions, 1)
	assert.Equal(t, "update_status", meta.Actions[0].Name)

	// Bound to a model with no attached actions → nothing visible.
	meta, err = ActionMetaForUser(ctx, tenant, "u1", []string{"unrelated_model"})
	require.NoError(t, err)
	assert.Empty(t, meta.Actions)

	// u2 has no groups → sees nothing even unbound.
	meta, err = ActionMetaForUser(ctx, tenant, "u2", nil)
	require.NoError(t, err)
	assert.Empty(t, meta.Actions)
}

package semantic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// fakeCube serves /v1/meta responses for the publish compile-verification
// poll. The measure count per model is mutable so a test can simulate the
// schema changing between publishes.
type fakeCube struct {
	mu       sync.Mutex
	measures map[string]int
	srv      *httptest.Server
}

func newFakeCube(t *testing.T) *fakeCube {
	t.Helper()
	f := &fakeCube{measures: map[string]int{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/meta") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		cubes := make([]map[string]interface{}, 0, len(f.measures))
		for name, n := range f.measures {
			ms := make([]map[string]interface{}, 0, n)
			for i := 0; i < n; i++ {
				ms = append(ms, map[string]interface{}{"name": fmt.Sprintf("m%d", i), "type": "number"})
			}
			cubes = append(cubes, map[string]interface{}{"name": name, "type": "cube", "measures": ms})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"cubes": cubes})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeCube) setMeasures(name string, n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.measures[name] = n
}

// newTestEngine builds an Engine over an in-memory sqlite database with a
// fake Cube server, exercising the real publish/rollback code paths.
func newTestEngine(t *testing.T) (*Engine, *fakeCube) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&CubeConnection{}, &SemanticModel{}, &SemanticModelVersion{},
		&DataGroup{}, &DataGroupMember{}, &AuditLog{},
	))

	f := newFakeCube(t)
	dir := t.TempDir()
	e, err := NewEngine(Config{
		Enabled:         true,
		APIURL:          f.srv.URL + "/v1",
		APISecret:       "test-secret",
		ModelDir:        filepath.Join(dir, "model"),
		DatasourcesFile: filepath.Join(dir, "datasources.yaml"),
		PublishTimeout:  10 * time.Second,
		HTTPTimeout:     2 * time.Second,
		MaxPreviewRows:  10,
		AuditLimit:      100,
	}, db)
	require.NoError(t, err)
	return e, f
}

const testDraftYAML = `cubes:
  - name: orders
    title: Orders
    sql: SELECT * FROM public.orders
    data_source: some_other_tenant_db
    measures:
      - name: count
        type: count
    dimensions:
      - name: status
        sql: status
        type: string
`

const testDraftYAMLTwoMeasures = `cubes:
  - name: orders
    title: Orders
    sql: SELECT * FROM public.orders
    data_source: some_other_tenant_db
    measures:
      - name: count
        type: count
      - name: total
        sql: amount
        type: sum
    dimensions:
      - name: status
        sql: status
        type: string
`

// seedConnectionAndModel creates one connection and one model bound to it.
func seedConnectionAndModel(t *testing.T, e *Engine, tenant uint64, draftYAML string, groups []string, visibility string) *SemanticModel {
	t.Helper()
	ctx := context.Background()
	conn, err := e.CreateConnection(ctx, "u1", tenant, &ConnectionInput{
		Name: "sales_db", Title: "Sales DB", Type: "postgres",
		Host: "127.0.0.1", Port: 5432, Database: "sales", Username: "ro", Password: "pw",
	})
	require.NoError(t, err)
	m, err := e.CreateModel(ctx, "u1", tenant, &SaveModelInput{
		Name: "orders", Title: "Orders", ConnectionID: conn.ID,
		DraftYAML: draftYAML, AllowedGroups: groups, MemberVisibility: visibility,
	})
	require.NoError(t, err)
	return m
}

func TestPublishFullFlowForcesOverridesAndSnapshots(t *testing.T) {
	e, f := newTestEngine(t)
	ctx := context.Background()
	m := seedConnectionAndModel(t, e, 7, testDraftYAML, []string{"sales"}, "")

	f.setMeasures("orders", 1)
	res, err := e.Publish(ctx, "u1", 7, m.ID, "first release")
	require.NoError(t, err)
	assert.Equal(t, 1, res.Version)
	assert.Equal(t, ModelStatusPublished, res.Status)

	// Publish re-reads the row; reload to observe persisted state.
	reloaded, err := e.GetModel(ctx, 7, m.ID)
	require.NoError(t, err)
	// data_source was force-overridden with the bound connection slug
	// (a contributor must not be able to point at another database).
	assert.Contains(t, reloaded.PublishedYAML, "data_source: sales_db")
	assert.NotContains(t, reloaded.PublishedYAML, "some_other_tenant_db")
	// access_policy was injected with the default-deny rule.
	assert.Contains(t, reloaded.PublishedYAML, "access_policy")

	// The published file landed in the model directory's auto/ section.
	if _, err := os.Stat(filepath.Join(e.cfg.ModelDir, "auto", "t7_orders.yaml")); err != nil {
		t.Errorf("published model file missing: %v", err)
	}

	// A version snapshot was recorded.
	versions, err := e.ListVersions(ctx, 7, m.ID)
	require.NoError(t, err)
	require.Len(t, versions, 1)
	assert.Equal(t, 1, versions[0].Version)
	assert.Equal(t, "first release", versions[0].Note)
	assert.Equal(t, "u1", versions[0].PublishedBy)
}

func TestPublishCompileFailureSelfHealsAndMarksFailed(t *testing.T) {
	e, _ := newTestEngine(t)
	// Shorten the compile poll so the test does not wait out the default.
	e.cfg.PublishTimeout = 2 * time.Second
	ctx := context.Background()
	m := seedConnectionAndModel(t, e, 7, testDraftYAML, nil, "")

	// The fake Cube never reports the model as compiled: the publish must
	// fail, remove the staged file, and mark the model publish_failed.
	res, err := e.Publish(ctx, "u1", 7, m.ID, "")
	require.Error(t, err)
	require.Nil(t, res)

	reloaded, err := e.GetModel(ctx, 7, m.ID)
	require.NoError(t, err)
	assert.Equal(t, ModelStatusPublishFailed, reloaded.Status)
	assert.NotEmpty(t, reloaded.LastError)
	if _, err := os.Stat(filepath.Join(e.cfg.ModelDir, "auto", "t7_orders.yaml")); !os.IsNotExist(err) {
		t.Errorf("staged file should be removed after failed compile, stat err: %v", err)
	}
}

func TestRollbackRestoresGroupsAndVisibility(t *testing.T) {
	e, f := newTestEngine(t)
	ctx := context.Background()
	m := seedConnectionAndModel(t, e, 7, testDraftYAML, []string{"sales"}, `{"sales":["count"]}`)

	f.setMeasures("orders", 1)
	_, err := e.Publish(ctx, "u1", 7, m.ID, "v1")
	require.NoError(t, err)

	// v2: different groups and member visibility.
	f.setMeasures("orders", 2)
	_, err = e.UpdateModel(ctx, "u1", 7, m.ID, &SaveModelInput{
		DraftYAML:        testDraftYAMLTwoMeasures,
		AllowedGroups:    []string{"finance"},
		MemberVisibility: `{"finance":["count","total"]}`,
		ExpectedVersion:  1,
	})
	require.NoError(t, err)
	_, err = e.Publish(ctx, "u1", 7, m.ID, "v2")
	require.NoError(t, err)

	reloaded, err := e.GetModel(ctx, 7, m.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"finance"}, StringList(reloaded.AllowedGroups))

	// The fake schema goes back to v1's shape, otherwise the rollback's
	// compile verification would legitimately fail.
	f.setMeasures("orders", 1)
	versions, err := e.ListVersions(ctx, 7, m.ID)
	require.NoError(t, err)
	require.Len(t, versions, 2)
	// Newest first: the second snapshot carries the v2 groups.
	v1 := versions[1]
	assert.Equal(t, []string{"sales"}, StringList(v1.AllowedGroups))

	res, err := e.Rollback(ctx, "u1", 7, m.ID, v1.ID)
	require.NoError(t, err)
	assert.Greater(t, res.Version, reloaded.Version)

	rolled, err := e.GetModel(ctx, 7, m.ID)
	require.NoError(t, err)
	// Groups and member-level visibility must be restored, not just the YAML.
	assert.Equal(t, []string{"sales"}, StringList(rolled.AllowedGroups))
	assert.JSONEq(t, `{"sales":["count"]}`, string(rolled.MemberVisibility))
	assert.Contains(t, rolled.PublishedYAML, "data_source: sales_db")
}

func TestUpdateModelOptimisticLockConflict(t *testing.T) {
	e, _ := newTestEngine(t)
	ctx := context.Background()
	m := seedConnectionAndModel(t, e, 7, testDraftYAML, nil, "")

	// First save with no expected version: accepted, version becomes 1.
	_, err := e.UpdateModel(ctx, "u1", 7, m.ID, &SaveModelInput{Title: "renamed"})
	require.NoError(t, err)

	// Matching expected version: accepted, version becomes 2.
	_, err = e.UpdateModel(ctx, "u1", 7, m.ID, &SaveModelInput{
		Title: "by another user", ExpectedVersion: 1,
	})
	require.NoError(t, err)

	// Stale expected version: rejected as a conflict.
	_, err = e.UpdateModel(ctx, "u1", 7, m.ID, &SaveModelInput{
		Title: "stale write", ExpectedVersion: 1,
	})
	require.Error(t, err)
	var conflict *ConflictError
	require.True(t, errors.As(err, &conflict), "expected ConflictError, got %T: %v", err, err)
	assert.Contains(t, err.Error(), "modified by another user")
}

func TestDeleteGroupReferenceProtection(t *testing.T) {
	e, _ := newTestEngine(t)
	ctx := context.Background()
	g, err := e.CreateGroup(ctx, "u1", 7, &DataGroup{Name: "sales", Title: "Sales"})
	require.NoError(t, err)
	m := seedConnectionAndModel(t, e, 7, testDraftYAML, []string{"sales"}, "")

	// A group referenced by a model must not be deletable: the published
	// accessPolicy would dangle and silently revoke members.
	err = e.DeleteGroup(ctx, "u1", 7, g.ID)
	require.Error(t, err)
	var conflict *ConflictError
	require.True(t, errors.As(err, &conflict), "expected ConflictError, got %T: %v", err, err)
	assert.Contains(t, err.Error(), "referenced by 1 model(s) (orders)")

	// Clear the reference, then deletion succeeds.
	_, err = e.UpdateModel(ctx, "u1", 7, m.ID, &SaveModelInput{AllowedGroups: []string{}})
	require.NoError(t, err)
	require.NoError(t, e.DeleteGroup(ctx, "u1", 7, g.ID))

	_, err = e.repo.FindGroup(ctx, 7, g.ID)
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestAuditRecordsPublish(t *testing.T) {
	e, f := newTestEngine(t)
	ctx := context.Background()
	m := seedConnectionAndModel(t, e, 7, testDraftYAML, nil, "")
	f.setMeasures("orders", 1)
	_, err := e.Publish(ctx, "u1", 7, m.ID, "")
	require.NoError(t, err)

	audits, err := e.ListAudits(ctx, 7, 10)
	require.NoError(t, err)
	var found bool
	for _, a := range audits {
		if a.Action == AuditModelPublish {
			found = true
			assert.Equal(t, "u1", a.UserID)
			assert.Equal(t, uint64(7), a.TenantID)
		}
	}
	assert.True(t, found, "publish audit record missing")
}

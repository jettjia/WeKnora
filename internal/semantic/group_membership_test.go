package semantic

// Cross-workspace data group membership: group resolution is global by user
// UUID, and granting is validated against tenant/org relations.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

// seedTenantTopology builds the tenant/org relations the grant validation
// relies on:
//
//	tenant 1 (本空间): u1
//	tenant 2: u2       — shares org "org-1" with tenant 1
//	tenant 3: u3       — no org relation with tenant 1
func seedTenantTopology(t *testing.T, e *Engine) {
	t.Helper()
	require.NoError(t, e.repo.db.AutoMigrate(&types.TenantMember{}, &types.OrganizationTenantMember{}))

	rows := []types.TenantMember{
		{UserID: "u1", TenantID: 1, Role: "contributor", Status: "active"},
		{UserID: "u2", TenantID: 2, Role: "contributor", Status: "active"},
		{UserID: "u3", TenantID: 3, Role: "contributor", Status: "active"},
	}
	for i := range rows {
		require.NoError(t, e.repo.db.Create(&rows[i]).Error)
	}
	// OrganizationTenantMember.ID is app-generated upstream (no BeforeCreate
	// hook) — set explicit ids to avoid PK collisions.
	orgMembers := []types.OrganizationTenantMember{
		{ID: "otm-1", OrganizationID: "org-1", TenantID: 1, Role: "admin"},
		{ID: "otm-2", OrganizationID: "org-1", TenantID: 2, Role: "member"},
		{ID: "otm-3", OrganizationID: "org-2", TenantID: 3, Role: "admin"},
	}
	for i := range orgMembers {
		require.NoError(t, e.repo.db.Create(&orgMembers[i]).Error)
	}
}

func TestUserGroupNamesResolveGlobally(t *testing.T) {
	e, _ := newTestEngine(t)
	ctx := context.Background()

	g, err := e.CreateGroup(ctx, "u1", 1, &DataGroup{Name: "sales", Title: "Sales"})
	require.NoError(t, err)
	require.NoError(t, e.repo.ReplaceGroupMembers(ctx, 1, g.ID, []string{"u2"}))

	// u2 belongs to tenant 2 but resolves tenant 1's group from any context.
	names, err := e.repo.UserGroupNames(ctx, "u2")
	require.NoError(t, err)
	assert.Equal(t, []string{"sales"}, names)

	// u3 has no membership anywhere.
	names, err = e.repo.UserGroupNames(ctx, "u3")
	require.NoError(t, err)
	assert.Empty(t, names)
}

func TestFilterGrantableMemberIDs(t *testing.T) {
	e, _ := newTestEngine(t)
	ctx := context.Background()
	seedTenantTopology(t, e)

	// u1: 本空间成员 → 通过; u2: 共享空间成员 → 通过;
	// u3: 无组织关系 → 拒绝; ghost: 不存在于任何空间 → 拒绝。
	grantable, rejected, err := e.repo.FilterGrantableMemberIDs(ctx, 1, []string{"u1", "u2", "u3", "ghost"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"u1", "u2"}, grantable)
	assert.ElementsMatch(t, []string{"u3", "ghost"}, rejected)

	// 空入参安全。
	grantable, rejected, err = e.repo.FilterGrantableMemberIDs(ctx, 1, nil)
	require.NoError(t, err)
	assert.Empty(t, grantable)
	assert.Empty(t, rejected)
}

// SetGroupMembers 端到端: 跨空间成员可加入; 无关系用户被 409 拒绝。
func TestSetGroupMembersCrossWorkspace(t *testing.T) {
	e, _ := newTestEngine(t)
	seedTenantTopology(t, e)
	ctx := context.Background()

	g, err := e.CreateGroup(ctx, "u1", 1, &DataGroup{Name: "sales", Title: "Sales"})
	require.NoError(t, err)

	// u2 (共享空间成员) 加入成功, 且全局可解析。
	require.NoError(t, e.SetGroupMembers(ctx, "u1", 1, g.ID, []string{"u2"}))
	names, err := e.repo.UserGroupNames(ctx, "u2")
	require.NoError(t, err)
	assert.Equal(t, []string{"sales"}, names)

	// u3 (无组织关系) 被拒绝, 且 u2 的既有成员关系不受影响。
	err = e.SetGroupMembers(ctx, "u1", 1, g.ID, []string{"u3"})
	require.Error(t, err)
	var ce *ConflictError
	assert.ErrorAs(t, err, &ce)
	assert.Contains(t, err.Error(), "u3")
	names, err = e.repo.UserGroupNames(ctx, "u2")
	require.NoError(t, err)
	assert.Equal(t, []string{"sales"}, names)
}

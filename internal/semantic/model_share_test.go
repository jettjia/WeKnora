package semantic

// 模型共享给组织的端到端单元测试: SecCtxFor 合成组注入 → accessPolicy
// 重写 → 共享可见性 (meta 过滤) → 取消共享回收。

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cubeclient "github.com/Tencent/WeKnora/internal/semantic/cubeclient"
	"github.com/Tencent/WeKnora/internal/types"
)

// seedShareTopology: 空间 1 (模型方) 与空间 2 (接收方) 同属 org-1;
// 空间 3 与两者均无关。
func seedShareTopology(t *testing.T, e *Engine, m *SemanticModel) {
	t.Helper()
	require.NoError(t, e.repo.db.AutoMigrate(&types.OrganizationTenantMember{}))
	orgMembers := []types.OrganizationTenantMember{
		{ID: "otm-1", OrganizationID: "org-1", TenantID: 1, Role: "admin"},
		{ID: "otm-2", OrganizationID: "org-1", TenantID: 2, Role: "member"},
		{ID: "otm-3", OrganizationID: "org-2", TenantID: 3, Role: "admin"},
	}
	for i := range orgMembers {
		require.NoError(t, e.repo.db.Create(&orgMembers[i]).Error)
	}
	_ = m
}

func shareOrgRule(yamlText string, orgID string) bool {
	doc, err := ParseModelYAML(yamlText)
	if err != nil {
		return false
	}
	if len(doc.Cubes) == 0 {
		return false
	}
	for _, rule := range doc.Cubes[0].AccessPolicy {
		if rule.Group == OrgSharedGroup(orgID) {
			return true
		}
	}
	return false
}

func TestShareModelLifecycle(t *testing.T) {
	e, fake := newTestEngine(t)
	ctx := context.Background()

	m := seedConnectionAndModel(t, e, 1, testDraftYAML, []string{"sales"}, "")
	seedShareTopology(t, e, m)
	fake.setMeasures(m.Name, 1)

	// 发布后才能共享
	_, err := e.Publish(ctx, "u1", 1, m.ID, "test")
	require.NoError(t, err)

	// ① 非本组织成员空间不可共享 (org-2)
	_, err = e.ShareModel(ctx, "u1", 1, m.ID, "org-2")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不属于该共享空间")

	// ② 共享给 org-1 → PublishedYAML 追加合成组规则
	share, err := e.ShareModel(ctx, "u1", 1, m.ID, "org-1")
	require.NoError(t, err)
	assert.Equal(t, "org-1", share.OrganizationID)
	latest, err := e.repo.FindModel(ctx, 1, m.ID)
	require.NoError(t, err)
	assert.True(t, shareOrgRule(latest.PublishedYAML, "org-1"), "accessPolicy must carry the org-shared rule")

	// ③ SecCtxFor: 空间 2 的用户注入合成组; 空间 3 不注入
	sec2, err := e.SecCtxFor(ctx, 2, "u2")
	require.NoError(t, err)
	assert.Contains(t, sec2.Groups, OrgSharedGroup("org-1"))
	sec3, err := e.SecCtxFor(ctx, 3, "u3")
	require.NoError(t, err)
	assert.NotContains(t, sec3.Groups, OrgSharedGroup("org-1"))

	// ④ meta 可见性: 空间 2 用户可见共享模型 (数据组未授权也能看 — 共享即开放);
	//    空间 3 用户不可见。u2/u3 属于空间 2/3 (seedTenantTopology 里 u1→1)。
	//    这里直接验证过滤逻辑: 用合成组构造 allowed 集合。
	meta := &cubeclient.MetaResponse{Cubes: []cubeclient.MetaCube{{Name: m.Name}}}
	meta.Cubes = append(meta.Cubes, cubeclient.MetaCube{Name: "other_tenant_model"})
	// 空间 2 视角: allowed = {org-shared:org-1} → 仅共享的模型可见
	filtered := e.FilterMetaForUserWithGroups(ctx, meta, map[string]bool{OrgSharedGroup("org-1"): true})
	require.Len(t, filtered.Cubes, 1)
	assert.Equal(t, m.Name, filtered.Cubes[0].Name)
	// 空间 3 视角: allowed 不含 org-shared:org-1 → 共享模型不可见
	filtered = e.FilterMetaForUserWithGroups(ctx, meta, map[string]bool{})
	assert.Empty(t, filtered.Cubes)

	// ⑤ 取消共享 → 规则移除
	require.NoError(t, e.UnshareModel(ctx, "u1", 1, m.ID, "org-1"))
	latest, err = e.repo.FindModel(ctx, 1, m.ID)
	require.NoError(t, err)
	assert.False(t, shareOrgRule(latest.PublishedYAML, "org-1"), "unshare must remove the rule")

	// ⑥ 重复共享幂等 (唯一索引冲突 → ConflictError)
	_, err = e.ShareModel(ctx, "u1", 1, m.ID, "org-1")
	require.NoError(t, err)
	_, err = e.ShareModel(ctx, "u1", 1, m.ID, "org-1")
	require.Error(t, err)
	var ce *ConflictError
	assert.ErrorAs(t, err, &ce)

	// ⑦ 审计含 model.share
	audits, err := e.ListAudits(ctx, 1, 50)
	require.NoError(t, err)
	found := false
	for _, a := range audits {
		if a.Action == "model.share" && a.Target == "model:"+m.Name {
			found = true
		}
	}
	assert.True(t, found, "expected model.share audit record")
}

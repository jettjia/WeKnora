package semantic

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// Repository provides data access for the semantic module. All queries are
// tenant-scoped unless explicitly noted (publish verification needs the
// cross-tenant model name set because one Cube deployment is shared).
type Repository struct {
	db *gorm.DB
}

// NewRepository creates the module repository.
func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// ErrNotFound marks a missing row.
var ErrNotFound = errors.New("resource not found")

func translateNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

// ---- connections ----

// ListConnections lists the tenant's connections.
func (r *Repository) ListConnections(ctx context.Context, tenantID uint64) ([]*CubeConnection, error) {
	var out []*CubeConnection
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("created_at ASC").Find(&out).Error
	return out, err
}

// AllConnections lists connections across tenants (datasources.yaml is
// deployment-wide; v1 targets single-org deployments).
func (r *Repository) AllConnections(ctx context.Context) ([]*CubeConnection, error) {
	var out []*CubeConnection
	err := r.db.WithContext(ctx).Order("created_at ASC").Find(&out).Error
	return out, err
}

// FindConnection retrieves one connection by ID.
func (r *Repository) FindConnection(ctx context.Context, tenantID uint64, id string) (*CubeConnection, error) {
	var c CubeConnection
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return nil, translateNotFound(err)
	}
	return &c, nil
}

// FindConnectionByName retrieves one connection by slug.
func (r *Repository) FindConnectionByName(ctx context.Context, tenantID uint64, name string) (*CubeConnection, error) {
	var c CubeConnection
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND name = ?", tenantID, name).First(&c).Error; err != nil {
		return nil, translateNotFound(err)
	}
	return &c, nil
}

// SaveConnection upserts a connection.
func (r *Repository) SaveConnection(ctx context.Context, c *CubeConnection) error {
	return r.db.WithContext(ctx).Save(c).Error
}

// DeleteConnection soft-deletes a connection.
func (r *Repository) DeleteConnection(ctx context.Context, c *CubeConnection) error {
	return r.db.WithContext(ctx).Delete(c).Error
}

// CountModelsUsingConnection guards connection deletion.
func (r *Repository) CountModelsUsingConnection(ctx context.Context, connectionID string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&SemanticModel{}).
		Where("connection_id = ? AND deleted_at IS NULL", connectionID).
		Count(&n).Error
	return n, err
}

// ---- models ----

// ListModels lists the tenant's models.
func (r *Repository) ListModels(ctx context.Context, tenantID uint64) ([]*SemanticModel, error) {
	var out []*SemanticModel
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("created_at ASC").Find(&out).Error
	return out, err
}

// ListPublishedModelsAllTenants returns published models of the whole
// deployment. Startup file reconciliation uses this as the source of truth
// for what the auto/ model directory should contain.
func (r *Repository) ListPublishedModelsAllTenants(ctx context.Context) ([]*SemanticModel, error) {
	var out []*SemanticModel
	err := r.db.WithContext(ctx).
		Where("status = ?", ModelStatusPublished).
		Order("tenant_id, created_at ASC").Find(&out).Error
	return out, err
}

// AllModelNames returns every non-deleted model name of the deployment
// (any status). Publish-time join validation uses this: mutually-joined
// model pairs must be publishable in any order — compile verification via
// /v1/meta remains the authoritative check.
func (r *Repository) AllModelNames(ctx context.Context, excludeID string) (map[string]bool, error) {
	var names []string
	q := r.db.WithContext(ctx).Model(&SemanticModel{}).Select("name").Where("deleted_at IS NULL")
	if excludeID != "" {
		q = q.Where("id <> ?", excludeID)
	}
	err := q.Pluck("name", &names).Error
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[n] = true
	}
	return out, err
}

// FindModel retrieves one model by ID.
func (r *Repository) FindModel(ctx context.Context, tenantID uint64, id string) (*SemanticModel, error) {
	var m SemanticModel
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).First(&m).Error; err != nil {
		return nil, translateNotFound(err)
	}
	return &m, nil
}

// SaveModel upserts a model.
func (r *Repository) SaveModel(ctx context.Context, m *SemanticModel) error {
	return r.db.WithContext(ctx).Save(m).Error
}

// DeleteModel soft-deletes a model and its version history.
func (r *Repository) DeleteModel(ctx context.Context, m *SemanticModel) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(m).Error; err != nil {
			return err
		}
		return tx.Where("model_id = ?", m.ID).Delete(&SemanticModelVersion{}).Error
	})
}

// SaveModelVersion appends a publish snapshot.
func (r *Repository) SaveModelVersion(ctx context.Context, v *SemanticModelVersion) error {
	return r.db.WithContext(ctx).Create(v).Error
}

// ListModelVersions lists publish snapshots (newest first).
func (r *Repository) ListModelVersions(
	ctx context.Context,
	tenantID uint64,
	modelID string,
) ([]*SemanticModelVersion, error) {
	var out []*SemanticModelVersion
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND model_id = ?", tenantID, modelID).
		Order("version DESC").Limit(50).Find(&out).Error
	return out, err
}

// FindModelVersion retrieves one snapshot for rollback.
func (r *Repository) FindModelVersion(
	ctx context.Context,
	tenantID uint64,
	versionID string,
) (*SemanticModelVersion, error) {
	var v SemanticModelVersion
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, versionID).First(&v).Error; err != nil {
		return nil, translateNotFound(err)
	}
	return &v, nil
}

// ---- data groups ----

// ListGroups lists the tenant's data groups.
func (r *Repository) ListGroups(ctx context.Context, tenantID uint64) ([]*DataGroup, error) {
	var out []*DataGroup
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("created_at ASC").Find(&out).Error
	return out, err
}

// FindGroup retrieves one group by ID.
func (r *Repository) FindGroup(ctx context.Context, tenantID uint64, id string) (*DataGroup, error) {
	var g DataGroup
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).First(&g).Error; err != nil {
		return nil, translateNotFound(err)
	}
	return &g, nil
}

// SaveGroup upserts a group.
func (r *Repository) SaveGroup(ctx context.Context, g *DataGroup) error {
	return r.db.WithContext(ctx).Save(g).Error
}

// DeleteGroup soft-deletes a group and its memberships.
func (r *Repository) DeleteGroup(ctx context.Context, g *DataGroup) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(g).Error; err != nil {
			return err
		}
		return tx.Where("group_id = ?", g.ID).Delete(&DataGroupMember{}).Error
	})
}

// ReplaceGroupMembers atomically rewrites a group's membership.
func (r *Repository) ReplaceGroupMembers(ctx context.Context, tenantID uint64, groupID string, userIDs []string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", groupID).Delete(&DataGroupMember{}).Error; err != nil {
			return err
		}
		for _, uid := range userIDs {
			m := &DataGroupMember{TenantID: tenantID, GroupID: groupID, UserID: uid}
			if err := tx.Create(m).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ListGroupMembers lists one group's memberships.
// ListMemberCandidates returns the data-group member picker directory, one
// row per USER (the UI groups them by their home workspace). The effective
// home is users.tenant_id when set, else the user's lowest remaining active
// membership (cleanupRemovedMemberState nulls the home pointer when a user's
// original space removes them, so tenant_members is the fallback authority).
// The candidate SET is still membership-scoped: users of the tenant itself
// plus users of workspaces sharing an organization with it (system admins
// see every user in the deployment). Grouping is display-only — data-group
// membership resolves globally by user UUID, and grant validation still
// runs on tenant_members.
func (r *Repository) ListMemberCandidates(ctx context.Context, tenantID uint64, allUsers bool) ([]map[string]interface{}, error) {
	base := `
		SELECT u.id AS user_id,
		       COALESCE(NULLIF(u.tenant_id, 0), fb.tenant_id) AS tenant_id,
		       u.username, u.email,
		       ft.name AS tenant_name,
		       (COALESCE(NULLIF(u.tenant_id, 0), fb.tenant_id) = ?) AS is_current
		FROM users u
		LEFT JOIN (
		    SELECT tm.user_id, MIN(tm.tenant_id) AS tenant_id
		    FROM tenant_members tm
		    GROUP BY tm.user_id
		) fb ON fb.user_id = u.id
		LEFT JOIN tenants ft
		       ON ft.id = COALESCE(NULLIF(u.tenant_id, 0), fb.tenant_id)
		WHERE u.deleted_at IS NULL`
	if !allUsers {
		base += `
		  AND (
		    u.tenant_id = ?
		    OR u.id IN (
		        SELECT tm2.user_id FROM tenant_members tm2
		        WHERE tm2.tenant_id = ?
		           OR tm2.tenant_id IN (
		               SELECT tenant_id FROM organization_tenant_members
		               WHERE organization_id IN (
		                   SELECT organization_id FROM organization_tenant_members
		                   WHERE tenant_id = ?
		               )))
		  )`
	}
	base += `
		ORDER BY tenant_name, u.username`
	var out []map[string]interface{}
	var err error
	if allUsers {
		err = r.db.WithContext(ctx).Raw(base, tenantID).Scan(&out).Error
	} else {
		err = r.db.WithContext(ctx).Raw(base, tenantID, tenantID, tenantID, tenantID).Scan(&out).Error
	}
	return out, err
}

// ---- model shares (共享给组织) ----

// CreateShare records one model→organization share (idempotent per pair).
func (r *Repository) CreateShare(ctx context.Context, share *SemanticModelShare) error {
	return r.db.WithContext(ctx).Create(share).Error
}

// DeleteShare removes one model→organization share.
func (r *Repository) DeleteShare(ctx context.Context, tenantID uint64, modelID, orgID string) error {
	return r.db.WithContext(ctx).
		Where("tenant_id = ? AND model_id = ? AND organization_id = ?", tenantID, modelID, orgID).
		Delete(&SemanticModelShare{}).Error
}

// ListSharesForModel returns the org shares of one model.
func (r *Repository) ListSharesForModel(ctx context.Context, tenantID uint64, modelID string) ([]*SemanticModelShare, error) {
	var out []*SemanticModelShare
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND model_id = ?", tenantID, modelID).
		Order("created_at ASC").Find(&out).Error
	return out, err
}

// DeleteSharesForModel removes all shares of one model (model deleted).
func (r *Repository) DeleteSharesForModel(ctx context.Context, tenantID uint64, modelID string) error {
	return r.db.WithContext(ctx).
		Where("tenant_id = ? AND model_id = ?", tenantID, modelID).
		Delete(&SemanticModelShare{}).Error
}

// OrgIDsForTenant returns the organization ids the tenant belongs to.
func (r *Repository) OrgIDsForTenant(ctx context.Context, tenantID uint64) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).Raw(
		`SELECT organization_id FROM organization_tenant_members WHERE tenant_id = ?`,
		tenantID).Scan(&ids).Error
	return ids, err
}

// ShareExistsForOrgs reports whether the model (by cube name) is shared to
// any of the given organizations.
func (r *Repository) ShareExistsForOrgs(ctx context.Context, modelName string, orgIDs []string) (bool, error) {
	if len(orgIDs) == 0 {
		return false, nil
	}
	var found bool
	err := r.db.WithContext(ctx).Raw(
		`SELECT EXISTS(
			SELECT 1 FROM semantic_model_shares s
			JOIN semantic_models sm ON sm.id = s.model_id
			WHERE sm.name = ? AND s.organization_id IN ?)`,
		modelName, orgIDs).Scan(&found).Error
	return found, err
}

func (r *Repository) ListGroupMembers(
	ctx context.Context,
	tenantID uint64,
	groupID string,
) ([]*DataGroupMember, error) {
	var out []*DataGroupMember
	err := r.db.WithContext(ctx).Where("tenant_id = ? AND group_id = ?", tenantID, groupID).Find(&out).Error
	return out, err
}

// UserGroupNames resolves the data group slugs of one user. Membership is
// global by user UUID: a workspace-B user granted a workspace-A group (via a
// shared organization) resolves it from any workspace. What the group
// unlocks is still gated per model / action by their access policies.
func (r *Repository) UserGroupNames(ctx context.Context, userID string) ([]string, error) {
	var names []string
	err := r.db.WithContext(ctx).Raw(`
		SELECT g.name FROM semantic_data_groups g
		JOIN semantic_data_group_members m ON m.group_id = g.id
		WHERE m.user_id = ? AND g.deleted_at IS NULL`,
		userID).Scan(&names).Error
	return names, err
}

// FilterGrantableMemberIDs splits candidate data-group member ids into
// grantable and rejected. A user is grantable when they are a member of the
// tenant itself or of any workspace sharing at least one organization with
// it (shared-space governance, mirrors agent/KB sharing).
func (r *Repository) FilterGrantableMemberIDs(ctx context.Context, tenantID uint64, userIDs []string, includeAll bool) (grantable []string, rejected []string, err error) {
	if len(userIDs) == 0 {
		return []string{}, []string{}, nil
	}
	var rows []struct {
		UserID    string `gorm:"column:user_id"`
		Direct    int64  `gorm:"column:direct"`
		OrgShared int64  `gorm:"column:org_shared"`
	}
	err = r.db.WithContext(ctx).Raw(`
		SELECT m.user_id,
		       MAX(CASE WHEN m.tenant_id = ? THEN 1 ELSE 0 END) AS direct,
		       MAX(CASE WHEN ot.organization_id IN (
		           SELECT organization_id FROM organization_tenant_members
		           WHERE tenant_id = ?
		       ) THEN 1 ELSE 0 END) AS org_shared
		FROM tenant_members m
		LEFT JOIN organization_tenant_members ot ON ot.tenant_id = m.tenant_id
		WHERE m.user_id IN ?
		GROUP BY m.user_id`,
		tenantID, tenantID, userIDs).Scan(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[string]bool, len(rows))
	for _, row := range rows {
		byID[row.UserID] = includeAll || row.Direct == 1 || row.OrgShared == 1
	}
	grantable = []string{}
	rejected = []string{}
	for _, uid := range userIDs {
		if byID[uid] {
			grantable = append(grantable, uid)
		} else {
			rejected = append(rejected, uid)
		}
	}
	return grantable, rejected, nil
}

// IsTenantAdmin checks whether the user has an owner or admin role in the
// given tenant (via the tenant_members table).
func (r *Repository) IsTenantAdmin(ctx context.Context, tenantID uint64, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}
	var role string
	err := r.db.WithContext(ctx).Raw(
		`SELECT role FROM tenant_members WHERE tenant_id = ? AND user_id = ? AND deleted_at IS NULL`,
		tenantID, userID,
	).Scan(&role).Error
	if err != nil {
		return false, err
	}
	return role == "owner" || role == "admin", nil
}

// IsSystemAdmin looks up the platform admin flag on the users table.
func (r *Repository) IsSystemAdmin(ctx context.Context, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}
	var admin bool
	err := r.db.WithContext(ctx).Raw(`SELECT is_system_admin FROM users WHERE id = ?`, userID).Scan(&admin).Error
	return admin, err
}

// ---- actions ----

// ListActions lists the tenant's actions.
func (r *Repository) ListActions(ctx context.Context, tenantID uint64) ([]*SemanticAction, error) {
	var out []*SemanticAction
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("created_at ASC").Find(&out).Error
	return out, err
}

// FindAction retrieves one action by ID.
func (r *Repository) FindAction(ctx context.Context, tenantID uint64, id string) (*SemanticAction, error) {
	var a SemanticAction
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).First(&a).Error; err != nil {
		return nil, translateNotFound(err)
	}
	return &a, nil
}

// FindActionByName retrieves one action by slug.
func (r *Repository) FindActionByName(ctx context.Context, tenantID uint64, name string) (*SemanticAction, error) {
	var a SemanticAction
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND name = ?", tenantID, name).First(&a).Error; err != nil {
		return nil, translateNotFound(err)
	}
	return &a, nil
}

// SaveAction upserts an action.
func (r *Repository) SaveAction(ctx context.Context, a *SemanticAction) error {
	return r.db.WithContext(ctx).Save(a).Error
}

// DeleteAction soft-deletes an action.
func (r *Repository) DeleteAction(ctx context.Context, a *SemanticAction) error {
	return r.db.WithContext(ctx).Delete(a).Error
}

// ActiveActionsByModels returns active actions whose object_types contain at
// least one of the given model names. Used by the agent's action_meta tool to
// list only actions attached to the agent's bound models. Filtering happens
// in Go (JSONB containment is Postgres-only; this keeps SQLite tests and
// deployments working — tenant action counts are small).
func (r *Repository) ActiveActionsByModels(ctx context.Context, tenantID uint64, modelNames []string) ([]*SemanticAction, error) {
	var out []*SemanticAction
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND status = ?", tenantID, "active").
		Order("created_at ASC").Find(&out).Error
	if err != nil || len(modelNames) == 0 {
		return out, err
	}
	bound := make(map[string]bool, len(modelNames))
	for _, n := range modelNames {
		bound[n] = true
	}
	filtered := out[:0:0]
	for _, a := range out {
		for _, m := range StringList(a.ObjectTypes) {
			if bound[m] {
				filtered = append(filtered, a)
				break
			}
		}
	}
	return filtered, nil
}

// ---- audit ----

// SaveAudit appends one audit record.
func (r *Repository) SaveAudit(ctx context.Context, a *AuditLog) error {
	return r.db.WithContext(ctx).Create(a).Error
}

// ListAudits lists recent audit records.
func (r *Repository) ListAudits(ctx context.Context, tenantID uint64, limit int) ([]*AuditLog, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var out []*AuditLog
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).
		Order("created_at DESC").Limit(limit).Find(&out).Error
	return out, err
}

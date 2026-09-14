package automation

// Handler serves the automation REST API under /api/v1/automations.
// Role floors: reads = viewer+, create/update/run = contributor+,
// delete = admin.

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// Handler serves automation endpoints.
type Handler struct {
	service *Service
}

// NewHandler builds the automation handler.
func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// tenantOf extracts the workspace identity from the request context.
func (h *Handler) tenantOf(c *gin.Context) (uint64, string, bool) {
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		return 0, "", false
	}
	userID, _ := c.Get(types.UserIDContextKey.String())
	uid, _ := userID.(string)
	return tenantID, uid, true
}

func unauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized: workspace context missing"})
}

// respondError maps service errors to HTTP statuses. ConflictError carries a
// business-readable message (409); everything else logs server-side and
// returns a generic 500.
func (h *Handler) respondError(c *gin.Context, err error) {
	var ce *ConflictError
	if errors.As(err, &ce) {
		c.JSON(http.StatusConflict, gin.H{"error": ce.Msg})
		return
	}
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		return
	}
	logger.Errorf(c.Request.Context(), "[automation] internal error: %v", err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

// List godoc
// @Router /automations [GET]
func (h *Handler) List(c *gin.Context) {
	tenant, _, ok := h.tenantOf(c)
	if !ok {
		unauthorized(c)
		return
	}
	list, err := h.service.ListAutomations(c.Request.Context(), tenant)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"automations": list})
}

// Get godoc
// @Router /automations/{id} [GET]
func (h *Handler) Get(c *gin.Context) {
	tenant, _, ok := h.tenantOf(c)
	if !ok {
		unauthorized(c)
		return
	}
	a, err := h.service.GetAutomation(c.Request.Context(), tenant, c.Param("id"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, a)
}

// Create godoc
// @Router /automations [POST]
func (h *Handler) Create(c *gin.Context) {
	tenant, uid, ok := h.tenantOf(c)
	if !ok {
		unauthorized(c)
		return
	}
	var in AutomationInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	a, err := h.service.CreateAutomation(c.Request.Context(), uid, tenant, &in)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, a)
}

// Update godoc — partial updates supported (send only the fields to change;
// enabled toggles via {"enabled": false}).
// @Router /automations/{id} [PUT]
func (h *Handler) Update(c *gin.Context) {
	tenant, _, ok := h.tenantOf(c)
	if !ok {
		unauthorized(c)
		return
	}
	var in AutomationInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	a, err := h.service.UpdateAutomation(c.Request.Context(), tenant, c.Param("id"), &in)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, a)
}

// Delete godoc
// @Router /automations/{id} [DELETE]
func (h *Handler) Delete(c *gin.Context) {
	tenant, _, ok := h.tenantOf(c)
	if !ok {
		unauthorized(c)
		return
	}
	if err := h.service.DeleteAutomation(c.Request.Context(), tenant, c.Param("id")); err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// RunNow godoc — enqueue a manual run.
// @Router /automations/{id}/run [POST]
func (h *Handler) RunNow(c *gin.Context) {
	tenant, _, ok := h.tenantOf(c)
	if !ok {
		unauthorized(c)
		return
	}
	if err := h.service.RunNow(c.Request.Context(), tenant, c.Param("id")); err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"enqueued": true})
}

// Runs godoc — run history of one automation.
// @Router /automations/{id}/runs [GET]
func (h *Handler) Runs(c *gin.Context) {
	tenant, _, ok := h.tenantOf(c)
	if !ok {
		unauthorized(c)
		return
	}
	limit := 50
	if v, err := strconv.Atoi(c.DefaultQuery("limit", "50")); err == nil {
		limit = v
	}
	runs, err := h.service.ListRuns(c.Request.Context(), tenant, c.Param("id"), limit)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"runs": runs})
}

// RegisterRoutes mounts the automation API under the authenticated v1 group.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, viewer, contributor, admin gin.HandlerFunc) {
	g := rg.Group("/automations")
	g.GET("", viewer, h.List)
	g.POST("", contributor, h.Create)
	g.GET("/:id", viewer, h.Get)
	g.PUT("/:id", contributor, h.Update)
	g.DELETE("/:id", admin, h.Delete)
	g.POST("/:id/run", contributor, h.RunNow)
	g.GET("/:id/runs", viewer, h.Runs)
}

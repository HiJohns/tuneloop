package handlers

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// UserManagementHandler manages platform-wide registered users (#1545).
// Registered under authRequired with RequireSysPerm(SysPermTenantList).
type UserManagementHandler struct{}

func NewUserManagementHandler() *UserManagementHandler {
	return &UserManagementHandler{}
}

// platformDB returns a DB instance exempt from the tenant query scoping
// (database.addTenantScope). User management is a platform-level feature
// (RequireSysPerm SysPermTenant*) that must list ALL registered users,
// including customers with empty tenant_id (00000000-0000-...).
func (h *UserManagementHandler) platformDB(c *gin.Context) *gorm.DB {
	ctx := context.WithValue(c.Request.Context(), database.TenantIDKey, "")
	return database.GetDB().WithContext(ctx)
}

// ListUserManagement returns paginated registered users with search.
// GET /admin/user-management?page=1&pageSize=20&search=...
func (h *UserManagementHandler) List(c *gin.Context) {
	db := h.platformDB(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	search := c.Query("search")

	q := db.Model(&models.User{})
	if search != "" {
		like := "%" + search + "%"
		q = q.Where("username ILIKE ? OR phone ILIKE ? OR wx_openid ILIKE ?", like, like, like)
	}

	var total int64
	q.Count(&total)

	var users []models.User
	if err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query users"})
		return
	}

	// Preload membership level names to avoid N+1 queries.
	var levelIDs []int
	for _, u := range users {
		if u.MembershipLevelID != nil {
			levelIDs = append(levelIDs, *u.MembershipLevelID)
		}
	}
	levelNames := make(map[int]string, len(levelIDs))
	if len(levelIDs) > 0 {
		var levels []models.MembershipLevel
		db.Where("id IN ?", levelIDs).Find(&levels)
		for _, lv := range levels {
			levelNames[lv.ID] = lv.Name
		}
	}

	list := make([]gin.H, 0, len(users))
	for _, u := range users {
		list = append(list, userSummary(u, levelNames))
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":  list,
			"total": total,
		},
	})
}

// Get returns full detail of one user.
// GET /admin/user-management/:id
func (h *UserManagementHandler) Get(c *gin.Context) {
	db := h.platformDB(c)
	var user models.User
	if err := db.Where("id = ?", c.Param("id")).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": userDetail(user, db)})
}

// Update edits editable fields (membership_level_id, promo_points, status).
// PUT /admin/user-management/:id
func (h *UserManagementHandler) Update(c *gin.Context) {
	var req struct {
		MembershipLevelID *int     `json:"membership_level_id"`
		PromoPoints       *float64 `json:"promo_points"`
		Status            *string  `json:"status"`
		IdPhotoFront      *string  `json:"id_photo_front"`
		IdPhotoBack       *string  `json:"id_photo_back"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request: " + err.Error()})
		return
	}

	db := h.platformDB(c)
	var user models.User
	if err := db.Where("id = ?", c.Param("id")).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}

	updates := map[string]interface{}{}
	if req.MembershipLevelID != nil {
		updates["membership_level_id"] = *req.MembershipLevelID
	}
	if req.PromoPoints != nil {
		// #1757: admin edits in cents (1 点 = 1 分) — stored as-is.
		updates["promo_points"] = *req.PromoPoints
	}
	if req.Status != nil {
		if *req.Status != "active" && *req.Status != "disabled" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "status must be active or disabled"})
			return
		}
		updates["status"] = *req.Status
	}
	if req.IdPhotoFront != nil {
		updates["id_photo_front"] = *req.IdPhotoFront
	}
	if req.IdPhotoBack != nil {
		updates["id_photo_back"] = *req.IdPhotoBack
	}
	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "no editable fields provided"})
		return
	}

	if err := db.Model(&user).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "success"})
}

// Export returns all matching users as CSV.
// GET /admin/user-management/export?search=...
func (h *UserManagementHandler) Export(c *gin.Context) {
	db := h.platformDB(c)

	search := c.Query("search")
	q := db.Model(&models.User{})
	if search != "" {
		like := "%" + search + "%"
		q = q.Where("username ILIKE ? OR phone ILIKE ? OR wx_openid ILIKE ?", like, like, like)
	}

	var users []models.User
	if err := q.Order("created_at DESC").Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query users"})
		return
	}

	levelIDs := make([]int, 0)
	for _, u := range users {
		if u.MembershipLevelID != nil {
			levelIDs = append(levelIDs, *u.MembershipLevelID)
		}
	}
	exportLevelNames := make(map[int]string, len(levelIDs))
	if len(levelIDs) > 0 {
		var levels []models.MembershipLevel
		db.Where("id IN ?", levelIDs).Find(&levels)
		for _, lv := range levels {
			exportLevelNames[lv.ID] = lv.Name
		}
	}

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=users_%d.csv", time.Now().Unix()))

	w := csv.NewWriter(c.Writer)
	defer w.Flush()
	w.Write([]string{"username", "wx_openid", "phone", "level", "points", "registered_at", "last_active", "status"})
	for _, u := range users {
		s := userSummary(u, exportLevelNames)
		w.Write([]string{
			fmt.Sprintf("%v", s["username"]),
			fmt.Sprintf("%v", s["wx_openid"]),
			fmt.Sprintf("%v", s["phone"]),
			fmt.Sprintf("%v", s["level"]),
			fmt.Sprintf("%v", s["points"]),
			fmt.Sprintf("%v", s["registered_at"]),
			fmt.Sprintf("%v", s["last_active"]),
			fmt.Sprintf("%v", s["status"]),
		})
	}
}

func userSummary(u models.User, levelNames map[int]string) gin.H {
	levelName := ""
	if u.MembershipLevelID != nil {
		levelName = levelNames[*u.MembershipLevelID]
	}
	return gin.H{
		"id":                  u.ID,
		"username":            u.Username,
		"wx_openid":           u.WxOpenid,
		"phone":               u.Phone,
		"level":               levelName,
		"membership_level_id": u.MembershipLevelID,
		"points":              u.PromoPoints,
		"registered_at":       u.CreatedAt,
		"last_active":         u.UpdatedAt,
		"status":              u.Status,
	}
}

func userDetail(u models.User, db *gorm.DB) gin.H {
	levelNames := make(map[int]string)
	if u.MembershipLevelID != nil {
		var lv models.MembershipLevel
		if err := db.Where("id = ?", *u.MembershipLevelID).First(&lv).Error; err == nil {
			levelNames[*u.MembershipLevelID] = lv.Name
		}
	}
	s := userSummary(u, levelNames)
	s["name"] = u.Name
	s["email"] = u.Email
	s["nickname"] = u.Nickname
	s["is_shadow"] = u.IsShadow
	s["total_spending"] = u.TotalSpending
	s["role"] = u.Role
	s["tenant_id"] = u.TenantID
	s["org_id"] = u.OrgID
	s["created_at"] = u.CreatedAt
	s["id_photo_front"] = resolveStorageKey(db.Statement.Context, u.IdPhotoFront)
	s["id_photo_back"] = resolveStorageKey(db.Statement.Context, u.IdPhotoBack)
	return s
}

// resolveStorageKey converts a stored media storage key into an accessible URL.
func resolveStorageKey(ctx context.Context, key *string) string {
	if key == nil || *key == "" {
		return ""
	}
	storage := services.NewMediaStorage()
	url, err := storage.GetURL(ctx, *key)
	if err != nil || url == "" {
		return fmt.Sprintf("/uploads/media/%s", *key)
	}
	return url
}

var _ = middleware.GetTenantID

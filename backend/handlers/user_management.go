package handlers

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
		q = q.Where("nickname ILIKE ? OR name ILIKE ? OR username ILIKE ? OR phone ILIKE ? OR wx_openid ILIKE ?",
			like, like, like, like, like)
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

	userIDs := make([]string, 0, len(users))
	for _, u := range users {
		userIDs = append(userIDs, u.ID)
	}
	pointsByUser := sumPointsByUser(db, userIDs)

	list := make([]gin.H, 0, len(users))
	for _, u := range users {
		list = append(list, userSummary(u, levelNames, pointsByUser[u.ID]))
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
		MembershipLevelID *int    `json:"membership_level_id"`
		Status            *string `json:"status"`
		IdPhotoFront      *string `json:"id_photo_front"`
		IdPhotoBack       *string `json:"id_photo_back"`
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
	// #1983 阶段 2：管理员直接改 promo_points 已移除；人工加赠乐币改由
	// 「加赠乐币」批次入口承接（#1982）。
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
		q = q.Where("nickname ILIKE ? OR name ILIKE ? OR username ILIKE ? OR phone ILIKE ? OR wx_openid ILIKE ?",
			like, like, like, like, like)
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

	exportUserIDs := make([]string, 0, len(users))
	for _, u := range users {
		exportUserIDs = append(exportUserIDs, u.ID)
	}
	exportPoints := sumPointsByUser(db, exportUserIDs)

	w := csv.NewWriter(c.Writer)
	defer w.Flush()
	w.Write([]string{"nickname", "username", "wx_openid", "phone", "level", "points", "registered_at", "last_active", "status"})
	for _, u := range users {
		s := userSummary(u, exportLevelNames, exportPoints[u.ID])
		w.Write([]string{
			fmt.Sprintf("%v", s["nickname"]),
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

// sumPointsByUser 批量聚合用户乐币余额（未过期批次 remaining 之和，#1983）。
func sumPointsByUser(db *gorm.DB, userIDs []string) map[string]models.Cents {
	out := make(map[string]models.Cents, len(userIDs))
	if len(userIDs) == 0 {
		return out
	}
	var rows []struct {
		UserID string
		Total  int64
	}
	db.Model(&models.PointBatch{}).
		Where("user_id IN ? AND remaining_cents > 0 AND (expires_at IS NULL OR expires_at >= ?)", userIDs, time.Now()).
		Select("user_id, COALESCE(SUM(remaining_cents), 0) AS total").
		Group("user_id").Scan(&rows)
	for _, r := range rows {
		out[r.UserID] = models.Cents(r.Total)
	}
	return out
}

func userSummary(u models.User, levelNames map[int]string, points models.Cents) gin.H {
	levelName := ""
	if u.MembershipLevelID != nil {
		levelName = levelNames[*u.MembershipLevelID]
	}
	return gin.H{
		"id":                  u.ID,
		"nickname":            u.Nickname,
		"username":            u.Username,
		"wx_openid":           u.WxOpenid,
		"phone":               u.Phone,
		"level":               levelName,
		"membership_level_id": u.MembershipLevelID,
		"points":              points,
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
	points, _ := services.GetUserPointsBalance(db, u.ID)
	s := userSummary(u, levelNames, points)
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
	s["id_photo_other"] = resolveStorageKey(db.Statement.Context, u.IdPhotoOther)
	s["id_photo_other_type"] = u.IdPhotoOtherType
	// #1810: 实名核身区块字段（模块 1 身份证信息 + 模块 2 人脸信息）。
	s["real_name"] = u.RealName
	s["id_card_no"] = u.IdCardNo
	s["id_card_expire"] = u.IdCardExpire
	s["id_card_authority"] = u.IdCardAuthority
	s["id_card_address"] = u.IdCardAddress
	s["face_verified"] = u.FaceVerified
	s["face_verify_method"] = u.FaceVerifyMethod
	s["face_verified_at"] = u.FaceVerifiedAt
	s["id_verify_status"] = deriveIdVerifyStatus(db, &u)
	return s
}

// resolveStorageKey converts a stored media storage key into an accessible URL.
func resolveStorageKey(ctx context.Context, key *string) string {
	return resolveMediaURL(ctx, key)
}

var _ = middleware.GetTenantID

// GrantPoints adds a manual points batch (admin gift) for a user (#1982).
// POST /admin/user-management/:id/points-grant
// Body: {amount: <元>, reason: <必填>}
// 加赠进入 point_batches 台账（source=manual），受统一有效期政策约束；
// 操作人/原因写入 points_transactions 留痕。批次 SUM 为余额唯一真源（#1983）。
func (h *UserManagementHandler) GrantPoints(c *gin.Context) {
	var req struct {
		Amount float64 `json:"amount"` // yuan
		Reason string  `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "reason is required"})
		return
	}
	amountCents := models.FromYuan(req.Amount)
	if amountCents <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "amount must be positive"})
		return
	}

	db := h.platformDB(c)
	var user models.User
	if err := db.Where("id = ?", c.Param("id")).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}

	operator := middleware.GetUserID(c.Request.Context())
	sourceRef := req.Reason
	if len(sourceRef) > 64 {
		sourceRef = sourceRef[:64] // point_batches.source_ref is varchar(64)
	}

	var batch *models.PointBatch
	var balance models.Cents
	err := db.Transaction(func(tx *gorm.DB) error {
		b, berr := services.CreatePointsBatch(tx, user.ID, services.PointBatchSourceManual, sourceRef, amountCents)
		if berr != nil {
			return berr
		}
		batch = b
		bal, berr := services.GetUserPointsBalance(tx, user.ID)
		if berr != nil {
			return berr
		}
		balance = bal
		return tx.Create(&models.PointsTransaction{
			ID:                uuid.New().String(),
			UserID:            user.ID,
			TenantID:          user.TenantID,
			Type:              "manual_grant",
			Amount:            amountCents,
			BalanceAfterPromo: float64(bal),
			Description:       fmt.Sprintf("管理员加赠乐币: %s（操作人: %s）", req.Reason, operator),
			CreatedAt:         time.Now(),
		}).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to grant points: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
		"batch_id":     batch.ID,
		"amount_cents": amountCents,
		"expires_at":   batch.ExpiresAt,
		"balance":      balance,
	}})
}

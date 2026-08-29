package handlers

import (
	"log"
	"net/http"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PlatformStaffHandler (#1795 T6): 平台员工管理（角色识别 + IAM 绑定）。
// 平台员工 = PLATFORM_ROOT_ORG_ID 根组织成员，用户/审核队列全可见（无 org 过滤），
// 商户数据仍 tenant 隔离。权限：user 类 sys_perm（List/Create/Update）。
type PlatformStaffHandler struct{}

// platformStaffItem 平台员工列表项。
type platformStaffItem struct {
	ID           string `json:"id"`
	IAMSub       string `json:"iam_sub"`
	Name         string `json:"name"`
	Phone        string `json:"phone"`
	Username     string `json:"username"`
	Status       string `json:"status"`
	PendingCount int64  `json:"pending_review_count"` // #1791 批次表待审计数
}

// List handles GET /admin/platform-staff.
func (h *PlatformStaffHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	rootOrgID := middleware.PlatformRootOrgID()
	if rootOrgID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "PLATFORM_ROOT_ORG_ID not configured"})
		return
	}

	// 平台员工 = 本地 users 缓存中 org_id = 根组织的成员（IAM 为准，#685：
	// 绑定在 IAM 侧完成，本地是缓存快照）。
	var users []models.User
	if err := db.Where("org_id = ?", rootOrgID).Order("created_at DESC").Find(&users).Error; err != nil {
		log.Printf("[PlatformStaff] list failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to list platform staff"})
		return
	}

	// 批量统计待审批次（#1791 批次表，一次 IN 查询防 N+1）。
	pendingCounts := map[string]int64{}
	if len(users) > 0 {
		ids := make([]string, 0, len(users))
		for _, u := range users {
			ids = append(ids, u.ID)
		}
		rows := []struct {
			UserID string
			Count  int64
		}{}
		db.Raw(`SELECT user_id, COUNT(*) AS count FROM face_capture_batches
			WHERE status = 'pending' AND user_id IN ?
			GROUP BY user_id`, ids).Scan(&rows)
		for _, r := range rows {
			pendingCounts[r.UserID] = r.Count
		}
	}

	list := make([]platformStaffItem, 0, len(users))
	for _, u := range users {
		list = append(list, platformStaffItem{
			ID:           u.ID,
			IAMSub:       u.IAMSub,
			Name:         u.Name,
			Phone:        u.Phone,
			Username:     u.Username,
			Status:       u.Status,
			PendingCount: pendingCounts[u.ID],
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": list, "total": len(list)}})
}

// Create handles POST /admin/platform-staff.
// 流程（IAM 先、本地后，#685）：CreateOrGetUser → BindUserToOrganizationWithToken
// （PLATFORM_ROOT_ORG_ID）→ 本地 users 缓存同步。
func (h *PlatformStaffHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	operatorID := middleware.GetUserID(ctx)

	rootOrgID := middleware.PlatformRootOrgID()
	if rootOrgID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "PLATFORM_ROOT_ORG_ID not configured"})
		return
	}

	var req struct {
		Username string `json:"username" binding:"required"`
		Name     string `json:"name" binding:"required"`
		Phone    string `json:"phone"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request: " + err.Error()})
		return
	}

	iamClient := services.NewIAMClient()
	// 1. IAM 创建/获取用户。
	result, err := iamClient.CreateOrGetUser("", &services.CreateUserRequest{
		Username:       req.Username,
		Name:           req.Name,
		Phone:          req.Phone,
		Email:          req.Email,
		Password:       req.Password,
		SkipActivation: true,
	})
	if err != nil {
		log.Printf("[PlatformStaff] CreateOrGetUser failed for %s: %v", req.Username, err)
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": err.Error()})
		return
	}

	// 2. IAM 绑定到根组织。
	if err := iamClient.BindUserToOrganizationWithToken("", result.UserID, rootOrgID, "STAFF", operatorID); err != nil {
		log.Printf("[PlatformStaff] BindUserToOrganization failed for %s: %v", result.UserID, err)
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": err.Error()})
		return
	}

	// 3. 本地 users 缓存同步（IAM 先、本地后）。
	var existing models.User
	user := models.User{
		ID:       uuid.New().String(),
		IAMSub:   result.UserID,
		Username: req.Username,
		Name:     req.Name,
		Phone:    req.Phone,
		Email:    req.Email,
		OrgID:    rootOrgID,
		Status:   "active",
	}
	if err := db.Where("iam_sub = ?", result.UserID).First(&existing).Error; err == nil {
		user = existing
		user.OrgID = rootOrgID
		user.Name = req.Name
		user.Phone = req.Phone
		if err := db.Model(&user).Updates(map[string]interface{}{
			"org_id": rootOrgID, "name": req.Name, "phone": req.Phone, "email": req.Email,
		}).Error; err != nil {
			log.Printf("[PlatformStaff] local cache update failed: %v", err)
		}
	} else {
		if err := db.Create(&user).Error; err != nil {
			log.Printf("[PlatformStaff] local cache create failed: %v", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"id": user.ID, "iam_sub": result.UserID}})
}

// Disable handles DELETE /admin/platform-staff/:id（禁用优先，不硬删）。
func (h *PlatformStaffHandler) Disable(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	operatorID := middleware.GetUserID(ctx)
	staffID := c.Param("id")

	rootOrgID := middleware.PlatformRootOrgID()
	if rootOrgID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "PLATFORM_ROOT_ORG_ID not configured"})
		return
	}

	var user models.User
	if err := db.Where("id = ? AND org_id = ?", staffID, rootOrgID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "platform staff not found"})
		return
	}

	// IAM 解绑（禁用优先，保留用户记录）。
	if err := services.NewIAMClient().UnbindUserFromOrganizationWithToken("", user.IAMSub, rootOrgID, operatorID); err != nil {
		log.Printf("[PlatformStaff] unbind failed for %s: %v", user.IAMSub, err)
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": err.Error()})
		return
	}
	// 本地缓存标记禁用（org_id 置空，保留记录可追溯）。
	if err := db.Model(&user).Updates(map[string]interface{}{
		"org_id": "", "status": "disabled", "updated_at": time.Now(),
	}).Error; err != nil {
		log.Printf("[PlatformStaff] local disable failed: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "platform staff disabled"})
}

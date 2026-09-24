package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// #1974 T1 师傅档案（直属商户，2026-09-18 设计变更）：
// 管理端 CRUD（PC）+ 顾客侧列表/详情（RS-API-1/8）+ 活跃会话计数（RS-API-9）。

type TechnicianProfileHandler struct{}

func NewTechnicianProfileHandler() *TechnicianProfileHandler { return &TechnicianProfileHandler{} }

type technicianExperience struct {
	Craft string `json:"craft"`
	Years int    `json:"years"`
}

func marshalExperience(exp []technicianExperience) string {
	if exp == nil {
		return "[]"
	}
	b, err := json.Marshal(exp)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func technicianNameByUserID(db *gorm.DB, userID string) string {
	var u models.User
	if err := db.Select("name").Where("id = ?", userID).First(&u).Error; err == nil {
		return u.Name
	}
	return ""
}

// technicianThumbURL 由原图 URL 推导缩略图 URL（#2049，`{base}_thumb.jpg`）。
// 仅**本管线**命名的 `technician_{id}.webp` 才有 `_thumb.jpg` 变体；存量旧 `/upload` 键
// （`{UnixNano}_{hex}.webp`）没有缩略图 → 返回 ""（前端回退原图，避免裂图）。
// 空 / 无扩展名 / 非 technician_ 前缀 → ""。
func technicianThumbURL(photo string) string {
	if photo == "" {
		return ""
	}
	base := photo
	if slash := strings.LastIndex(photo, "/"); slash >= 0 {
		base = photo[slash+1:]
	}
	if !strings.HasPrefix(base, "technician_") {
		return ""
	}
	dot := strings.LastIndex(photo, ".")
	if dot < 0 {
		return ""
	}
	return photo[:dot] + "_thumb.jpg"
}

// List GET /api/technician-profiles（管理端，租户范围）
func (h *TechnicianProfileHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tid := middleware.GetTenantID(ctx)
	if tid == "" {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	query := db.Where("tenant_id = ?", tid)
	if st := c.Query("status"); st != "" {
		query = query.Where("status = ?", st)
	}
	var rows []models.TechnicianProfile
	if err := query.Order("created_at DESC").Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to list technician profiles"})
		return
	}
	list := make([]gin.H, 0, len(rows))
	for _, p := range rows {
		list = append(list, gin.H{
			"id": p.ID, "user_id": p.UserID, "tenant_id": p.TenantID,
			"name":  technicianNameByUserID(db, p.UserID),
			"photo": p.Photo, "photo_thumb": technicianThumbURL(p.Photo), "bio": p.Bio, "experience": p.Experience,
			"status": p.Status, "created_at": p.CreatedAt, "updated_at": p.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": list, "total": len(list)}})
}

// Create POST /api/technician-profiles（管理端）
func (h *TechnicianProfileHandler) Create(c *gin.Context) {
	var body struct {
		UserID     string                 `json:"user_id" binding:"required"`
		Photo      string                 `json:"photo"`
		Bio        string                 `json:"bio"`
		Experience []technicianExperience `json:"experience"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "user_id is required"})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tid := middleware.GetTenantID(ctx)
	if tid == "" {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	var u models.User
	if err := db.Where("id = ? AND tenant_id = ?", body.UserID, tid).First(&u).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "user not found in this merchant"})
		return
	}
	var cnt int64
	db.Model(&models.TechnicianProfile{}).Where("user_id = ? AND tenant_id = ?", body.UserID, tid).Count(&cnt)
	if cnt > 0 {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "technician profile already exists"})
		return
	}
	p := models.TechnicianProfile{
		ID: uuid.New().String(), UserID: body.UserID, TenantID: tid,
		Photo: body.Photo, Bio: body.Bio,
		Experience: marshalExperience(body.Experience), Status: "active",
	}
	if err := db.Create(&p).Error; err != nil {
		log.Printf("[TechnicianProfile.Create] failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create technician profile"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"id": p.ID}})
}

// Update PUT /api/technician-profiles/:id（管理端）
func (h *TechnicianProfileHandler) Update(c *gin.Context) {
	var body struct {
		Photo      *string                `json:"photo"`
		Bio        *string                `json:"bio"`
		Experience []technicianExperience `json:"experience"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request"})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tid := middleware.GetTenantID(ctx)
	var p models.TechnicianProfile
	if err := db.Where("id = ? AND tenant_id = ?", c.Param("id"), tid).First(&p).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "technician profile not found"})
		return
	}
	updates := map[string]interface{}{"updated_at": db.NowFunc()}
	if body.Photo != nil {
		updates["photo"] = *body.Photo
	}
	if body.Bio != nil {
		updates["bio"] = *body.Bio
	}
	if body.Experience != nil {
		updates["experience"] = marshalExperience(body.Experience)
	}
	if err := db.Model(&models.TechnicianProfile{}).Where("id = ?", p.ID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update technician profile"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "updated"})
}

// SetStatus PUT /api/technician-profiles/:id/status（管理端）
func (h *TechnicianProfileHandler) SetStatus(c *gin.Context) {
	var body struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || (body.Status != "active" && body.Status != "inactive") {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "status must be active or inactive"})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tid := middleware.GetTenantID(ctx)
	var p models.TechnicianProfile
	if err := db.Where("id = ? AND tenant_id = ?", c.Param("id"), tid).First(&p).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "technician profile not found"})
		return
	}
	if err := db.Model(&models.TechnicianProfile{}).Where("id = ?", p.ID).
		Updates(map[string]interface{}{"status": body.Status, "updated_at": db.NowFunc()}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update status"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"id": p.ID, "status": body.Status}})
}

// UploadPhoto POST /api/technician-profiles/:id/photo（管理端，#2049）
// 师傅照片走统一媒体管线（docs/topics/media/media_directory.md）：
//   - 原图 → technician_{profileID}.webp（≤800，WebP，详情/大图用）
//   - 缩略图 → technician_{profileID}_thumb.jpg（128，列表用）
//
// 两者登记 media_assets（source_type=technician）；photo 字段存原图 URL。
func (h *TechnicianProfileHandler) UploadPhoto(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tid := middleware.GetTenantID(ctx)
	if tid == "" {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	var p models.TechnicianProfile
	if err := db.Where("id = ? AND tenant_id = ?", c.Param("id"), tid).First(&p).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "technician profile not found"})
		return
	}
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "file required"})
		return
	}
	defer file.Close()
	fileData, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to read file"})
		return
	}
	displayData, err := services.GenerateThumbnailWebP(fileData, 800, 800)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to process image"})
		return
	}
	thumbData, err := services.GenerateThumbnail(fileData, 128)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to process thumbnail"})
		return
	}
	storage := services.NewMediaStorage()
	displayKey := "technician_" + p.ID + ".webp"
	thumbKey := "technician_" + p.ID + "_thumb.jpg"
	if err := storage.Upload(ctx, displayKey, bytes.NewReader(displayData), "image/webp"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to upload photo"})
		return
	}
	if err := storage.Upload(ctx, thumbKey, bytes.NewReader(thumbData), "image/jpeg"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to upload thumbnail"})
		return
	}
	registry := services.NewMediaRegistry()
	if err := registry.RegisterAsset(ctx, displayKey, services.SourceTypeTechnician, p.ID, int64(len(displayData)), "image"); err != nil {
		log.Printf("[MediaRegistry] register technician photo %s failed: %v", displayKey, err)
	}
	if err := registry.RegisterAsset(ctx, thumbKey, services.SourceTypeTechnician, p.ID, int64(len(thumbData)), "image"); err != nil {
		log.Printf("[MediaRegistry] register technician thumb %s failed: %v", thumbKey, err)
	}
	photoURL := "/uploads/media/" + displayKey
	if err := db.Model(&models.TechnicianProfile{}).Where("id = ?", p.ID).
		Updates(map[string]interface{}{"photo": photoURL, "updated_at": db.NowFunc()}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to save photo"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
		"photo": photoURL,
		"thumb": "/uploads/media/" + thumbKey,
	}})
}

// PublicList GET /api/common/repair-technicians（RS-API-1 重写，顾客上下文）
// 数据源 = technician_profiles(status='active')（**直属商户，去 site 维度**）；
// 顾客 JWT 无 tid/oid → 不按 JWT 过滤（公共口径，同 /common/sites/nearby）。
func (h *TechnicianProfileHandler) PublicList(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	var rows []models.TechnicianProfile
	if err := db.Where("status = ?", "active").Order("created_at ASC").Find(&rows).Error; err != nil {
		log.Printf("[TechnicianProfile.PublicList] query failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to list technicians"})
		return
	}
	list := make([]gin.H, 0, len(rows))
	for _, p := range rows {
		list = append(list, gin.H{
			"technician_id": p.UserID, // 与 Create(锁定) / select-technician 口径一致（users.id）
			"name":          technicianNameByUserID(db, p.UserID),
			"avatar":        p.Photo,
			"avatar_thumb":  technicianThumbURL(p.Photo),
			"bio":           p.Bio,
			"experience":    p.Experience,
			"tenant_id":     p.TenantID,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": list, "total": len(list)}})
}

// PublicGet GET /api/common/repair-technicians/:id（RS-API-8，顾客上下文）
// :id = technician_id（users.id，与列表输出一致）
func (h *TechnicianProfileHandler) PublicGet(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	var p models.TechnicianProfile
	if err := db.Where("user_id = ? AND status = ?", c.Param("id"), "active").First(&p).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "technician not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
		"technician_id": p.UserID,
		"name":          technicianNameByUserID(db, p.UserID),
		"avatar":        p.Photo,
		"avatar_thumb":  technicianThumbURL(p.Photo),
		"bio":           p.Bio,
		"experience":    p.Experience,
		"tenant_id":     p.TenantID,
	}})
}

// ActiveSessionCount GET /api/common/repair-technicians/active-session-count（RS-API-9）
// 师傅本人（JWT 必填）：technician_id=我 且 status != 'closed' 的服务单计数。
func (h *TechnicianProfileHandler) ActiveSessionCount(c *gin.Context) {
	ctx := c.Request.Context()
	sub := middleware.GetUserID(ctx)
	if sub == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "authentication required"})
		return
	}
	db := database.GetDB().WithContext(ctx)
	me := localUserIDBySub(db, sub)
	if me == "" {
		me = sub
	}
	var count int64
	if err := db.Model(&models.RepairRequest{}).
		Where("type = ? AND technician_id = ? AND status <> ?", "service", me, models.RepairReqStatusClosed).
		Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to count sessions"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"count": count}})
}

package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services/wechatpay"

	"github.com/gin-gonic/gin"
)

func ListPayments(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	db := database.GetDB().WithContext(ctx)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	orderType := c.Query("order_type")
	method := c.Query("method")
	status := c.Query("status")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	search := c.Query("search") // out_trade_no or transaction_id

	query := db.Model(&models.OrderPaymentRecord{}).Where("tenant_id = ?", tenantID)

	if orderType != "" {
		query = query.Where("order_type = ?", orderType)
	}
	if method != "" {
		query = query.Where("method = ?", method)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if startDate != "" {
		query = query.Where("created_at >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("created_at <= ?", endDate+" 23:59:59")
	}
	if search != "" {
		query = query.Where("out_trade_no ILIKE ? OR transaction_id ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	var total int64
	query.Count(&total)

	var records []models.OrderPaymentRecord
	if err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&records).Error; err != nil {
		log.Printf("[ListPayments] query failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "query failed"})
		return
	}

	// Fetch refund records for each payment
	type paymentWithRefunds struct {
		models.OrderPaymentRecord
		Refunds []models.OrderRefundRecord `json:"refunds,omitempty"`
	}

	result := make([]paymentWithRefunds, 0, len(records))
	for _, rec := range records {
		item := paymentWithRefunds{OrderPaymentRecord: rec}
		var refunds []models.OrderRefundRecord
		db.Where("payment_record_id = ?", rec.ID).Find(&refunds)
		if len(refunds) > 0 {
			item.Refunds = refunds
		}
		result = append(result, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":  result,
			"total": total,
		},
	})
}

func ExportPayments(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	db := database.GetDB().WithContext(ctx)

	var records []models.OrderPaymentRecord
	query := db.Model(&models.OrderPaymentRecord{}).Where("tenant_id = ?", tenantID)

	if t := c.Query("order_type"); t != "" {
		query = query.Where("order_type = ?", t)
	}
	if s := c.Query("status"); s != "" {
		query = query.Where("status = ?", s)
	}

	query.Order("created_at DESC").Find(&records)

	var sb strings.Builder
	sb.WriteString("时间,商户订单号,微信交易号,类别,金额,方式,状态,关联订单ID\n")
	for _, rec := range records {
		ts := rec.CreatedAt.Format("2006-01-02 15:04:05")
		tradeNo := ""
		if rec.OutTradeNo != nil {
			tradeNo = *rec.OutTradeNo
		}
		txID := ""
		if rec.TransactionID != nil {
			txID = *rec.TransactionID
		}
		oid := ""
		if rec.OrderID != nil {
			oid = *rec.OrderID
		}
		sb.WriteString(fmt.Sprintf("%s,%s,%s,%s,%.2f,%s,%s,%s\n", ts, tradeNo, txID, rec.OrderType, rec.Amount, safeStr(rec.Method), rec.Status, oid))
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=payments_"+time.Now().Format("20060102")+".csv")
	c.String(http.StatusOK, sb.String())
}

func QueryPaymentByTradeNo(c *gin.Context) {
	outTradeNo := c.Param("out_trade_no")
	if outTradeNo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "out_trade_no required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var record models.OrderPaymentRecord
	if err := db.Where("out_trade_no = ?", outTradeNo).First(&record).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "payment record not found"})
		return
	}

	client := wechatpay.GetClient()
	result, err := client.QueryOrder(ctx, outTradeNo)
	if err != nil {
		log.Printf("[QueryPaymentByTradeNo] query failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "query failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"record":         record,
			"wechat_state":   result.TradeState,
			"transaction_id": result.TransactionID,
		},
	})
}

func safeStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

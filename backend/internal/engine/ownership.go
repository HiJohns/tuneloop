package engine

import (
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

type OwnershipEngine struct{}

func NewOwnershipEngine() *OwnershipEngine {
	return &OwnershipEngine{}
}

type OwnershipInfo struct {
	CertificateID   string       `json:"certificate_id"`
	OrderID         string       `json:"order_id"`
	Instrument      InstrumentInfo `json:"instrument"`
	Owner           OwnerInfo    `json:"owner"`
	TransferDate    time.Time    `json:"transfer_date"`
	CertificateURL  string       `json:"certificate_url"`
}

type InstrumentInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	SN   string `json:"sn"`
}

type OwnerInfo struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Phone  string `json:"phone"`
}

func (e *OwnershipEngine) CheckAndTransfer(orderID string) error {
	db := database.GetDB()
	
	var order models.Order
	if err := db.First(&order, "id = ?", orderID).Error; err != nil {
		return err
	}
	
	if order.AccumulatedMonths < 12 || order.Status != "active" {
		return nil
	}
	
	tx := db.Begin()
	
	order.Status = "transferred"
	order.UpdatedAt = time.Now()
	if err := tx.Save(&order).Error; err != nil {
		tx.Rollback()
		return err
	}
	
	var instrument models.Instrument
	if err := tx.First(&instrument, "id = ?", order.InstrumentID).Error; err != nil {
		tx.Rollback()
		return err
	}
	
	instrument.StockStatus = "sold"
	instrument.UpdatedAt = time.Now()
	if err := tx.Save(&instrument).Error; err != nil {
		tx.Rollback()
		return err
	}
	
	cert := models.OwnershipCertificate{
		OrderID:      order.ID,
		UserID:       order.UserID,
		InstrumentID: order.InstrumentID,
		TransferDate: time.Now(),
	}
	if err := tx.Create(&cert).Error; err != nil {
		tx.Rollback()
		return err
	}
	
	tx.Commit()
	return nil
}

func (e *OwnershipEngine) GetOwnershipInfo(orderID string) (*OwnershipInfo, error) {
	db := database.GetDB()
	
	var cert models.OwnershipCertificate
	if err := db.First(&cert, "order_id = ?", orderID).Error; err != nil {
		return nil, err
	}
	
	var instrument models.Instrument
	db.First(&instrument, "id = ?", cert.InstrumentID)
	
	var user models.User
	db.First(&user, "id = ?", cert.UserID)
	
	return &OwnershipInfo{
		CertificateID:  cert.ID,
		OrderID:       cert.OrderID,
		Instrument:    InstrumentInfo{ID: instrument.ID, Name: instrument.Name},
		Owner:         OwnerInfo{UserID: user.ID, Name: user.Name, Phone: user.Phone},
		TransferDate:  cert.TransferDate,
		CertificateURL: cert.CertificateURL,
	}, nil
}

func (e *OwnershipEngine) ProcessEligibleOrders() error {
	db := database.GetDB()
	
	var orders []models.Order
	db.Where("accumulated_months >= 12 AND status = 'active'").Find(&orders)
	
	for _, order := range orders {
		e.CheckAndTransfer(order.ID)
	}
	
	return nil
}

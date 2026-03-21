package service

import (
	"bytes"
	"fmt"
	"time"
	
	"github.com/go-pdf/fpdf"
)

type CertificateData struct {
	CertificateID string
	InstrumentName string
	InstrumentSN  string
	OwnerName     string
	OwnerPhone    string
	TransferDate  time.Time
}

func GenerateOwnershipCertificate(data *CertificateData) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	
	pdf.SetFont("Arial", "B", 20)
	pdf.SetTextColor(99, 102, 241)
	pdf.CellFormat(0, 15, "TuneLoop", "", 1, "C", false, 0, "")
	
	pdf.SetFont("Arial", "B", 24)
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(5)
	pdf.CellFormat(0, 15, "所有权转移证明", "", 1, "C", false, 0, "")
	
	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(128, 128, 128)
	pdf.CellFormat(0, 8, "Ownership Transfer Certificate", "", 1, "C", false, 0, "")
	
	pdf.Ln(15)
	
	pdf.SetFont("Arial", "", 12)
	pdf.SetTextColor(0, 0, 0)
	
	certLine := fmt.Sprintf("证书编号: %s", data.CertificateID)
	pdf.CellFormat(0, 8, certLine, "", 1, "L", false, 0, "")
	
	pdf.Ln(10)
	
	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 10, "一、乐器信息", "", 1, "L", false, 0, "")
	
	pdf.SetFont("Arial", "", 12)
	pdf.CellFormat(0, 8, fmt.Sprintf("乐器名称: %s", data.InstrumentName), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 8, fmt.Sprintf("序列号(SN): %s", data.InstrumentSN), "", 1, "L", false, 0, "")
	
	pdf.Ln(8)
	
	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 10, "二、用户信息", "", 1, "L", false, 0, "")
	
	pdf.SetFont("Arial", "", 12)
	pdf.CellFormat(0, 8, fmt.Sprintf("用户姓名: %s", data.OwnerName), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 8, fmt.Sprintf("联系电话: %s", data.OwnerPhone), "", 1, "L", false, 0, "")
	
	pdf.Ln(8)
	
	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 10, "三、转移信息", "", 1, "L", false, 0, "")
	
	pdf.SetFont("Arial", "", 12)
	transferDate := data.TransferDate.Format("2006年01月02日")
	pdf.CellFormat(0, 8, fmt.Sprintf("转移日期: %s", transferDate), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 8, fmt.Sprintf("累计租期: 12个月"), "", 1, "L", false, 0, "")
	
	pdf.Ln(15)
	
	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(128, 128, 128)
	pdf.CellFormat(0, 6, "本证书由 TuneLoop 系统自动生成，具有法律效力。", "", 1, "C", false, 0, "")
	pdf.CellFormat(0, 6, fmt.Sprintf("生成时间: %s", time.Now().Format("2006-01-02 15:04:05")), "", 1, "C", false, 0, "")
	
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	
	return buf.Bytes(), nil
}

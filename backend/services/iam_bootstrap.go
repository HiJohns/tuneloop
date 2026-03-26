package services

import (
	"fmt"
	"os"
	"tuneloop-backend/models"

	"gorm.io/gorm"
)

func BootstrapIAM(db *gorm.DB) error {
	bootstrapClientID := os.Getenv("BOOTSTRAP_CLIENT_ID")
	if bootstrapClientID == "" {
		return nil
	}

	var count int64
	db.Model(&models.Client{}).Where("client_id = ?", bootstrapClientID).Count(&count)
	if count > 0 {
		fmt.Println("Bootstrap: Client already exists, skipping")
		return nil
	}

	bootstrapClientSecret := os.Getenv("BOOTSTRAP_CLIENT_SECRET")
	if bootstrapClientSecret == "" {
		bootstrapClientSecret = "bootstrap-secret-" + bootstrapClientID
	}

	redirectURIs := os.Getenv("BOOTSTRAP_REDIRECT_URIS")
	if redirectURIs == "" {
		wwwPort := os.Getenv("TUNELOOP_WWW_PORT")
		if wwwPort == "" {
			wwwPort = "5554"
		}
		wxPort := os.Getenv("TUNELOOP_WX_PORT")
		if wxPort == "" {
			wxPort = "5553"
		}
		redirectURIs = fmt.Sprintf("http://localhost:%s/callback,http://localhost:%s/callback", wwwPort, wxPort)
	}

	client := &models.Client{
		ClientID:     bootstrapClientID,
		ClientSecret: bootstrapClientSecret,
		Name:         "Bootstrap Client",
		RedirectURIs: redirectURIs,
	}

	if err := db.Create(client).Error; err != nil {
		return fmt.Errorf("failed to create bootstrap client: %w", err)
	}

	fmt.Printf("Bootstrap: Created IAM Client '%s'\n", bootstrapClientID)
	return nil
}

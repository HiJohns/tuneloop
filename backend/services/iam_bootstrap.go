package services

import (
	"fmt"
	"log"
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

	// Register OAuth apps (tuneloop_web / tuneloop_wechat) with IAM.
	// This ensures the frontend can authenticate via IAM's OAuth flow.
	iamNs := os.Getenv("IAM_NAMESPACE")
	iamSecret := os.Getenv("IAM_SECRET")
	if iamNs != "" && iamSecret != "" {
		iamClient := NewIAMClient()
		pcRedirect := "https://web.cadenzayueqi.com/callback"
		wxRedirect := "https://wx.cadenzayueqi.com/callback"
		if os.Getenv("APP_ENV") == "development" {
			pcRedirect = "http://localhost:5554/callback"
			wxRedirect = "http://localhost:5553/callback"
		}

		apps := []struct {
			appType     string
			redirectURI string
		}{
			{"web", pcRedirect},
			{"wechat", wxRedirect},
		}
		for _, app := range apps {
			resp, err := iamClient.RegisterNamespaceApp(iamNs, app.appType, app.redirectURI)
			if err != nil {
				log.Printf("[Bootstrap] Warning: failed to register IAM app %s_%s: %v", iamNs, app.appType, err)
			} else {
				log.Printf("[Bootstrap] Registered IAM app: client_id=%s", resp.ClientID)
			}
		}
	}
	return nil
}

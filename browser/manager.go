package browser

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/playwright-community/playwright-go"
)

const (
	DreaminaURL     = "https://dreamina.capcut.com/ai-tool/generate"
	SessionDir      = ".dreamina-session" // store cookies/localStorage to avoid logging in again
)

type Manager struct {
	pw      *playwright.Playwright
	browser playwright.Browser
	context playwright.BrowserContext
}

// New initializes Playwright and loads an existing session if available
func New(headless bool) (*Manager, error) {
	if err := playwright.Install(&playwright.RunOptions{Browsers: []string{"chromium"}}); err != nil {
		return nil, fmt.Errorf("install playwright: %w", err)
	}

	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("run playwright: %w", err)
	}

	sessionPath, _ := filepath.Abs(SessionDir)
	os.MkdirAll(sessionPath, 0700)

	// Persistent context keeps cookies/localStorage across runs
	// ByteDance is less suspicious when the browser has history and a stable fingerprint
	context, err := pw.Chromium.LaunchPersistentContext(sessionPath,
		playwright.BrowserTypeLaunchPersistentContextOptions{
			Headless: playwright.Bool(headless),
			// Mimic a real browser to reduce bot detection
			Args: []string{
				"--disable-blink-features=AutomationControlled",
				"--disable-infobars",
				"--no-sandbox",
			},
			UserAgent: playwright.String(
				"Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
					"AppleWebKit/537.36 (KHTML, like Gecko) " +
					"Chrome/122.0.0.0 Safari/537.36",
			),
			Viewport: &playwright.Size{Width: 1280, Height: 800},
			Locale:   playwright.String("en-US"),
		},
	)
	if err != nil {
		pw.Stop()
		return nil, fmt.Errorf("launch browser: %w", err)
	}

	return &Manager{pw: pw, context: context}, nil
}

// NewPage opens a new tab
func (m *Manager) NewPage() (playwright.Page, error) {
	return m.context.NewPage()
}

// Close performs cleanup
func (m *Manager) Close() {
	if m.context != nil {
		m.context.Close()
	}
	if m.pw != nil {
		m.pw.Stop()
	}
}

// IsLoggedIn checks whether the session is still valid
func IsLoggedIn(page playwright.Page) bool {
	// Dreamina redirects to /login if not authenticated
	url := page.URL()
	return url != "" && !contains(url, "login") && !contains(url, "signin")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

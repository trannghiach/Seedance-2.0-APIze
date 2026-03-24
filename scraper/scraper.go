package scraper

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

const (
	GenerateURL = "https://dreamina.capcut.com/ai-tool/home"
	Timeout     = 3 * time.Minute
)

type GenerateOptions struct {
	Prompt      string
	Duration    int
	Resolution  string
	AspectRatio string
}

type GenerateResult struct {
	VideoPath string
	VideoURL  string
}

type Scraper struct {
	page playwright.Page
}

func New(page playwright.Page) *Scraper {
	return &Scraper{page: page}
}

func (s *Scraper) Generate(opts GenerateOptions) (*GenerateResult, error) {
	// 1. Navigate
	if _, err := s.page.Goto(GenerateURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(30000),
	}); err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}
	time.Sleep(2 * time.Second)

	// Switch to AI Video mode
	if err := s.switchToAIVideo(); err != nil {
		return nil, fmt.Errorf("switch mode: %w", err)
	}

	if strings.Contains(s.page.URL(), "login") {
		return nil, fmt.Errorf("not logged in — run: go run . login")
	}

	// 2. Find prompt input
	promptSelectors := []string{
		"div[contenteditable='true']",
		"textarea[placeholder*='prompt']",
		"textarea[placeholder*='describe']",
		"textarea[placeholder*='Upload']",
		"textarea",
	}

	var promptEl playwright.ElementHandle
	for _, sel := range promptSelectors {
		el, err := s.page.QuerySelector(sel)
		if err == nil && el != nil {
			promptEl = el
			break
		}
	}
	if promptEl == nil {
		s.page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String("debug_ui.png")})
		return nil, fmt.Errorf("prompt input not found — check debug_ui.png")
	}

	// 3. Type prompt
	if err := promptEl.Click(); err != nil {
		return nil, fmt.Errorf("click prompt: %w", err)
	}
	if err := promptEl.Fill(opts.Prompt); err != nil {
		// fallback for contenteditable
		promptEl.Click(playwright.ElementHandleClickOptions{ClickCount: playwright.Int(3)})
		s.page.Keyboard().Press("Backspace")
		s.page.Keyboard().Type(opts.Prompt)
	}
	time.Sleep(500 * time.Millisecond)

	// 4. Submit with Enter
	if err := s.page.Keyboard().Press("Enter"); err != nil {
		return nil, fmt.Errorf("press enter: %w", err)
	}
	fmt.Println("  -> Job submitted, tracking progress...")

	time.Sleep(1 * time.Second)
	confirmBtn, _ := s.page.QuerySelector("button.lv-btn-primary:has-text('Confirm')")
	if confirmBtn != nil {
		confirmBtn.Click()
		fmt.Println("  -> Confirmed terms dialog")
	}
	
	// Wait a bit for job to register, then reload to get clean state
	time.Sleep(3 * time.Second)
	if _, err := s.page.Reload(playwright.PageReloadOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		return nil, fmt.Errorf("reload: %w", err)
	}
	time.Sleep(2 * time.Second)

	// 5. Wait for % progress to appear then disappear
	if err := s.waitForProgress(); err != nil {
		return nil, fmt.Errorf("wait for progress: %w", err)
	}

	// 6. Click the first video card
	if err := s.clickFirstCard(); err != nil {
		return nil, fmt.Errorf("click card: %w", err)
	}
	time.Sleep(1 * time.Second)

	// 7. Click Download and receive file
	outPath := fmt.Sprintf("%s/dreamina_%d.mp4", os.TempDir(), time.Now().Unix())
	if err := s.clickDownload(outPath); err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}

	return &GenerateResult{VideoPath: outPath}, nil
}

// waitForProgress polls for "X%Dreaming..." text, waits until it disappears
func (s *Scraper) waitForProgress() error {
	deadline := time.Now().Add(Timeout)
	appeared := false

	for time.Now().Before(deadline) {
		el, _ := s.page.QuerySelector("text=/\\d+%/")
		if el != nil {
			text, _ := el.TextContent()
			fmt.Printf("  %s\r", text)
			appeared = true
		} else if appeared {
			fmt.Println("  -> Generation complete")
			return nil
		} else {
			fmt.Print("  waiting for generation to start...\r")
		}
		time.Sleep(2 * time.Second)
	}

	s.page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String("debug_timeout.png")})
	return fmt.Errorf("timeout after %s — check debug_timeout.png", Timeout)
}

// clickFirstCard clicks the most recently generated video card
func (s *Scraper) clickFirstCard() error {
	cardSelectors := []string{
		"[class*='videoCard']:first-of-type",
		"[class*='card']:first-of-type",
		"[class*='item']:first-of-type",
		"[class*='result']:first-of-type",
	}

	for _, sel := range cardSelectors {
		el, err := s.page.QuerySelector(sel)
		if err == nil && el != nil {
			if err := el.Click(); err == nil {
				return nil
			}
		}
	}

	s.page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String("debug_card.png")})
	return fmt.Errorf("video card not found — check debug_card.png")
}

// clickDownload clicks the Download button and saves the file
func (s *Scraper) clickDownload(destPath string) error {
	downloadCh := make(chan playwright.Download, 1)
	s.page.On("download", func(d playwright.Download) {
		downloadCh <- d
	})

	dlBtn, err := s.page.QuerySelector("button:has-text('Download')")
	if err != nil || dlBtn == nil {
		s.page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String("debug_download.png")})
		return fmt.Errorf("download button not found — check debug_download.png")
	}

	if err := dlBtn.Click(); err != nil {
		return fmt.Errorf("click download: %w", err)
	}

	select {
	case dl := <-downloadCh:
		if err := dl.SaveAs(destPath); err != nil {
			return fmt.Errorf("save file: %w", err)
		}
		fmt.Printf("  -> Saved to %s\n", destPath)
		return nil
	case <-time.After(30 * time.Second):
		return fmt.Errorf("download timeout")
	}
}

func (s *Scraper) switchToAIVideo() error {
    // Click the mode switcher button (shows "AI Agent" by default)
    modeBtn, err := s.page.QuerySelector(".lv-select-view-value")
    if err != nil || modeBtn == nil {
        return fmt.Errorf("mode button not found")
    }
    if err := modeBtn.Click(); err != nil {
        return fmt.Errorf("click mode button: %w", err)
    }
    time.Sleep(500 * time.Millisecond)

    // Click "AI Video" from dropdown
    videoOption, err := s.page.QuerySelector("li[role='option']:has-text('AI Video')")
    if err != nil || videoOption == nil {
        return fmt.Errorf("AI Video option not found")
    }
    if err := videoOption.Click(); err != nil {
        return fmt.Errorf("click AI Video: %w", err)
    }
    time.Sleep(1 * time.Second)
    return nil
}
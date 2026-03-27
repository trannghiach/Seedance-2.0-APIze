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
	Model       string   // "seedance-2.0" | "seedance-2.0-fast", default "seedance-2.0-fast"
	Duration    int      // 4-15, default 5
	AspectRatio string   // "16:9" | "9:16" | "1:1" | "4:3" | "3:4" | "21:9", default "16:9"
	Mode        string   // "omni" | "start-end", default "omni"
	References  []string // file paths, omni only, max 9
	StartFrame  string   // file path, start-end only
	EndFrame    string   // file path, start-end only
}

type GenerateResult struct {
	VideoPath string
}

type Scraper struct {
	page playwright.Page
}

func New(page playwright.Page) *Scraper {
	return &Scraper{page: page}
}

func (s *Scraper) Generate(opts GenerateOptions) (*GenerateResult, error) {
	// Apply defaults
	if opts.Model == "" {
		opts.Model = "seedance-2.0-fast"
	}
	if opts.Duration == 0 {
		opts.Duration = 5
	}
	if opts.AspectRatio == "" {
		opts.AspectRatio = "16:9"
	}
	if opts.Mode == "" {
		opts.Mode = "omni"
	}

	// 1. Navigate
	if _, err := s.page.Goto(GenerateURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(30000),
	}); err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}
	time.Sleep(2 * time.Second)

	if strings.Contains(s.page.URL(), "login") {
		return nil, fmt.Errorf("not logged in — run: go run . login")
	}

	// 2. Switch to AI Video mode
	if err := s.switchToAIVideo(); err != nil {
		return nil, fmt.Errorf("switch to AI Video: %w", err)
	}

	// 3. Select model
	if err := s.selectModel(opts.Model); err != nil {
		return nil, fmt.Errorf("select model: %w", err)
	}

	// 4. Select reference mode
	if err := s.selectMode(opts.Mode); err != nil {
		return nil, fmt.Errorf("select mode: %w", err)
	}

	// 5. Select aspect ratio
	if err := s.selectAspectRatio(opts.AspectRatio); err != nil {
		return nil, fmt.Errorf("select aspect ratio: %w", err)
	}

	// 6. Select duration
	if err := s.selectDuration(opts.Duration); err != nil {
		return nil, fmt.Errorf("select duration: %w", err)
	}

	// 7. Upload files
	switch opts.Mode {
	case "omni":
		if len(opts.References) > 0 {
			if err := s.uploadReferences(opts.References); err != nil {
				return nil, fmt.Errorf("upload references: %w", err)
			}
		}
	case "start-end":
		if err := s.uploadStartEnd(opts.StartFrame, opts.EndFrame); err != nil {
			return nil, fmt.Errorf("upload start/end frames: %w", err)
		}
	}

	// 8. Find and fill prompt
	promptSelectors := []string{
		"div[contenteditable='true']",
		"textarea[placeholder*='Describe']",
		"textarea[placeholder*='prompt']",
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

	if err := promptEl.Click(); err != nil {
		return nil, fmt.Errorf("click prompt: %w", err)
	}
	if err := promptEl.Fill(opts.Prompt); err != nil {
		promptEl.Click(playwright.ElementHandleClickOptions{ClickCount: playwright.Int(3)})
		s.page.Keyboard().Press("Backspace")
		s.page.Keyboard().Type(opts.Prompt)
	}
	time.Sleep(500 * time.Millisecond)

	// 9. Submit
	if err := s.page.Keyboard().Press("Enter"); err != nil {
		return nil, fmt.Errorf("press enter: %w", err)
	}
	fmt.Println("  -> Job submitted, tracking progress...")

	// Handle first-time confirmation dialog if present
	time.Sleep(1 * time.Second)
	confirmBtn, _ := s.page.QuerySelector("button.lv-btn-primary:has-text('Confirm')")
	if confirmBtn != nil {
		confirmBtn.Click()
		fmt.Println("  -> Confirmed terms dialog")
	}

	// Reload for clean state
	time.Sleep(3 * time.Second)
	if _, err := s.page.Reload(playwright.PageReloadOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		return nil, fmt.Errorf("reload: %w", err)
	}
	time.Sleep(2 * time.Second)

	// 10. Wait for generation
	if err := s.waitForProgress(); err != nil {
		return nil, fmt.Errorf("wait for progress: %w", err)
	}

	// 11. Click first card
	if err := s.clickFirstCard(); err != nil {
		return nil, fmt.Errorf("click card: %w", err)
	}
	time.Sleep(3 * time.Second)

	// 12. Download
	outPath := fmt.Sprintf("%s/dreamina_%d.mp4", os.TempDir(), time.Now().Unix())
	if err := s.clickDownload(outPath); err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}

	return &GenerateResult{VideoPath: outPath}, nil
}

// ── UI Interaction Helpers ────────────────────────────────────────────────────

func (s *Scraper) switchToAIVideo() error {
	modeBtn, err := s.page.QuerySelector(".lv-select-view-value")
	if err != nil || modeBtn == nil {
		return fmt.Errorf("mode button not found")
	}
	if err := modeBtn.Click(); err != nil {
		return fmt.Errorf("click mode button: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

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

func (s *Scraper) selectModel(model string) error {
	modelBtn, err := s.page.QuerySelector(".lv-select-view-value:has-text('Seedance')")
	if err != nil || modelBtn == nil {
		return fmt.Errorf("model selector not found")
	}
	if err := modelBtn.Click(); err != nil {
		return fmt.Errorf("click model selector: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	var optionText string
	switch model {
	case "seedance-2.0":
		optionText = "Dreamina Seedance 2.0"
	default: // seedance-2.0-fast
		optionText = "Dreamina Seedance 2.0 Fast"
	}

	// Use exact match to avoid "2.0 Fast" matching "2.0"
	options, err := s.page.QuerySelectorAll("li[role='option']")
	if err != nil {
		return fmt.Errorf("query options: %w", err)
	}
	for _, opt := range options {
		text, _ := opt.TextContent()
		trimmed := strings.TrimSpace(text)
		if model == "seedance-2.0" {
			if strings.Contains(trimmed, "Seedance 2.0") && !strings.Contains(trimmed, "Fast") {
				return opt.Click()
			}
		} else {
			if strings.Contains(trimmed, "Seedance 2.0 Fast") {
				return opt.Click()
			}
		}
	}
	return fmt.Errorf("model option '%s' not found", optionText)
}

func (s *Scraper) selectMode(mode string) error {
	// Find current mode button (could be any mode)
	modeBtn, err := s.page.QuerySelector(
		".lv-select-view-value:has-text('First and last frames'), " +
			".lv-select-view-value:has-text('Omni reference'), " +
			".lv-select-view-value:has-text('Multiframes')",
	)
	if err != nil || modeBtn == nil {
		return fmt.Errorf("mode selector not found")
	}
	if err := modeBtn.Click(); err != nil {
		return fmt.Errorf("click mode selector: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	var optionText string
	switch mode {
	case "start-end":
		optionText = "First and last frames"
	default: // omni
		optionText = "Omni reference"
	}

	option, err := s.page.QuerySelector(fmt.Sprintf("li[role='option']:has-text('%s')", optionText))
	if err != nil || option == nil {
		return fmt.Errorf("mode option '%s' not found", optionText)
	}
	if err := option.Click(); err != nil {
		return fmt.Errorf("click mode option: %w", err)
	}
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (s *Scraper) selectAspectRatio(ratio string) error {
	// Open aspect ratio popover
	ratioBtn, err := s.page.QuerySelector("button[class*='toolbar-button']:has-text(':')")
	if err != nil || ratioBtn == nil {
		return fmt.Errorf("aspect ratio button not found")
	}
	if err := ratioBtn.Click(); err != nil {
		return fmt.Errorf("click aspect ratio button: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Click radio input
	radioInput, err := s.page.QuerySelector(fmt.Sprintf("input[type='radio'][value='%s']", ratio))
	if err != nil || radioInput == nil {
		return fmt.Errorf("aspect ratio '%s' not found", ratio)
	}
	if err := radioInput.DispatchEvent("click", nil); err != nil {
		return fmt.Errorf("click aspect ratio: %w", err)
	}

	// Wait for popover to close
	s.page.Click("div[contenteditable='true'], textarea")

	return nil
}

func (s *Scraper) selectDuration(duration int) error {
	// Open duration dropdown
	durationBtn, err := s.page.QuerySelector(".lv-select-view-value:has-text('4s'), .lv-select-view-value:has-text('5s'), .lv-select-view-value:has-text('6s'), .lv-select-view-value:has-text('7s'), .lv-select-view-value:has-text('8s'), .lv-select-view-value:has-text('9s'), .lv-select-view-value:has-text('10s'), .lv-select-view-value:has-text('11s'), .lv-select-view-value:has-text('12s'), .lv-select-view-value:has-text('13s'), .lv-select-view-value:has-text('14s'), .lv-select-view-value:has-text('15s')")
	if err != nil || durationBtn == nil {
		return fmt.Errorf("duration selector not found")
	}
	if err := durationBtn.Click(); err != nil {
		return fmt.Errorf("click duration selector: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Click option e.g. "8s"
	optionText := fmt.Sprintf("%ds", duration)
	option, err := s.page.QuerySelector(fmt.Sprintf("li[role='option']:has-text('%s')", optionText))
	if err != nil || option == nil {
		return fmt.Errorf("duration '%s' not found", optionText)
	}
	if err := option.Click(); err != nil {
		return fmt.Errorf("click duration option: %w", err)
	}
	time.Sleep(300 * time.Millisecond)
	return nil
}

func (s *Scraper) uploadReferences(filePaths []string) error {
	input, err := s.page.QuerySelector("div[class*='reference-upload']:has-text('Reference') input[type='file']")
	if err != nil || input == nil {
		return fmt.Errorf("reference upload input not found")
	}
	if err := input.SetInputFiles(filePaths); err != nil {
		return fmt.Errorf("set input files: %w", err)
	}
	time.Sleep(1 * time.Second)
	return nil
}

func (s *Scraper) uploadStartEnd(startFrame, endFrame string) error {
	if startFrame != "" {
		input, err := s.page.QuerySelector("div[class*='reference-upload']:has-text('First frame') input[type='file']")
		if err != nil || input == nil {
			return fmt.Errorf("start frame upload input not found")
		}
		if err := input.SetInputFiles([]string{startFrame}); err != nil {
			return fmt.Errorf("set start frame: %w", err)
		}
		time.Sleep(500 * time.Millisecond)
	}

	if endFrame != "" {
		input, err := s.page.QuerySelector("div[class*='reference-upload']:has-text('Last frame') input[type='file']")
		if err != nil || input == nil {
			return fmt.Errorf("end frame upload input not found")
		}
		if err := input.SetInputFiles([]string{endFrame}); err != nil {
			return fmt.Errorf("set end frame: %w", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

// ── Progress & Download ───────────────────────────────────────────────────────

func (s *Scraper) waitForProgress() error {
	appeared := false

	for {
		var activeText string
		for _, sel := range []string{
			"text=/\\d+%/",
			"text=/Accelerating/i",
			"text=/in queue/i",
			"text=/minutes wait/i",
			"div[class*='progress-badge']",
			"div[class*='progress-tips']",
		} {
			el, _ := s.page.QuerySelector(sel)
			if el != nil {
				text, _ := el.TextContent()
				fmt.Printf("  [debug] matched '%s': %q\n", sel, text)
				activeText = text
				break
			}
		}

		if activeText != "" {
			fmt.Printf("  %s\r", activeText)
			appeared = true
		} else if appeared {
			// Progress gone — check if video exists
			lastCard, _ := s.page.QuerySelector("div[class*='video-record']:last-of-type")
			if lastCard != nil {
				if video, _ := lastCard.QuerySelector("video"); video == nil {
					return fmt.Errorf("generation failed — something went wrong on Dreamina's side")
				}
			}
			fmt.Println("  -> Generation complete")
			return nil
		} else {
			if el, _ := s.page.QuerySelector("text=/Face detected/i"); el != nil {
				return fmt.Errorf("face detected in reference images — Dreamina rejected the request")
			}
			fmt.Print("  waiting for generation to start...\r")
		}

		time.Sleep(3 * time.Second)
	}
}

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
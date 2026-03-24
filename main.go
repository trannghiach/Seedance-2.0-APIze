package main

import (
	"fmt"
	"os"
	"time"

	"github.com/yourname/dreamina-pw/browser"
	"github.com/yourname/dreamina-pw/queue"
	"github.com/yourname/dreamina-pw/scraper"
	"github.com/yourname/dreamina-pw/server"
)

const usage = `
Dreamina Playwright API Wrapper

Commands:
	login    Open browser to log in to Dreamina (save session)
  serve    Start API server
  test     Generate 1 video test

Usage:
  go run . login
  go run . serve [--port 8080] [--key myapikey] [--headless]
  go run . test "a cat walking in rain"
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(1)
	}

	switch os.Args[1] {

	// ── LOGIN ─────────────────────────────────────────────────────────
	// Open browser for manual login, then save session
	case "login":
		fmt.Println("Opening browser for login...")
		fmt.Println("1. Log in to Dreamina")
		fmt.Println("2. After logging in, return to the terminal and press Enter")

		mgr, err := browser.New(false) // headful so the user can see it
		if err != nil {
			fatal("browser:", err)
		}
		defer mgr.Close()

		page, err := mgr.NewPage()
		if err != nil {
			fatal("page:", err)
		}

		page.Goto("https://dreamina.capcut.com/ai-tool/home")

		fmt.Print("\nPress Enter when logged in... ")
		fmt.Scanln()

		// Session is automatically saved to .dreamina-session/ by PersistentContext
		fmt.Println("✓ Session saved to .dreamina-session/")

	// ── SERVE ─────────────────────────────────────────────────────────
	case "serve":
		port := getFlag("--port", "8080")
		apiKey := getFlag("--key", "")
		headless := !hasFlag("--show")
		concurrency := 1 // 1 video at a time to avoid rate limits

		fmt.Println("Starting Dreamina API wrapper...")

		mgr, err := browser.New(headless)
		if err != nil {
			fatal("browser:", err)
		}
		defer mgr.Close()

		// Warmup: open Dreamina page and verify login
		fmt.Println("Checking session...")
		page, err := mgr.NewPage()
		if err != nil {
			fatal("page:", err)
		}
		page.Goto("https://dreamina.capcut.com/ai-tool/home")
		time.Sleep(4 * time.Second)

		if !browser.IsLoggedIn(page) {
			fmt.Println("✗ Not logged in. Run: go run . login")
			os.Exit(1)
		}
		page.Close()
		fmt.Println("✓ Session valid")

		q := queue.New(mgr, concurrency)
		srv := server.New(q, apiKey, port)

		if err := srv.Run(); err != nil {
			fatal("server:", err)
		}

	// ── TEST ──────────────────────────────────────────────────────────
	case "test":
		prompt := "a cat walking in rain, cinematic"
		if len(os.Args) > 2 {
			prompt = os.Args[2]
		}

		fmt.Printf("Testing with prompt: %q\n", prompt)

		mgr, err := browser.New(false) // headful for debugging
		if err != nil {
			fatal("browser:", err)
		}
		defer mgr.Close()

		page, err := mgr.NewPage()
		if err != nil {
			fatal("page:", err)
		}

		s := scraper.New(page)
		result, err := s.Generate(scraper.GenerateOptions{
			Prompt:      prompt,
			Duration:    4,
			Resolution:  "720p",
			AspectRatio: "16:9",
		})
		if err != nil {
			fatal("generate:", err)
		}

		fmt.Printf("\n✓ Video saved: %s\n", result.VideoPath)
		fmt.Printf("  URL: %s\n", result.VideoURL)

	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		fmt.Print(usage)
		os.Exit(1)
	}
}

func fatal(msg string, err error) {
	fmt.Fprintf(os.Stderr, "✗ %s %v\n", msg, err)
	os.Exit(1)
}

func getFlag(name, def string) string {
	for i, arg := range os.Args {
		if arg == name && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return def
}

func hasFlag(name string) bool {
	for _, arg := range os.Args {
		if arg == name {
			return true
		}
	}
	return false
}

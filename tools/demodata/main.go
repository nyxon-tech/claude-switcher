// Command demodata writes a made-up Claude setup (a Desktop data folder, a vault with three saved
// logins and a projects folder full of transcripts) for screenshots, demos and manual testing:
//
//	go run ./tools/demodata -out demo
//	claude-switcher --data-dir demo/claude --vault-dir demo/vault --projects-dir demo/projects
//
// Nothing here reads or touches the real Claude folders.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const org = "7c1e5a90-3b2d-4f6e-9a18-2d4b6c8e0f13"

type account struct {
	profile, uuid string
	chats         []chat
}

type chat struct {
	title, cwd, model string
	ageHours          float64
	turns             int
	lost              bool // a transcript no card lists, for Recover
}

var accounts = []account{
	{"work", "0ce4cc0c-5a3e-4d8f-b6a1-9e2f7c4d1b35", []chat{
		{"Refactor the payment service", `C:\code\payments-api`, "claude-opus-5", 1.5, 14, false},
		{"بررسی کد ماژول پرداخت", `C:\code\payments-api`, "claude-opus-5", 3, 9, false},
		{"Fix the flaky login test", `C:\code\web-app`, "claude-sonnet-5", 20, 6, false},
		{"طراحی صفحه‌ی داشبورد (نسخه ۲)", `C:\code\dashboard`, "claude-opus-5", 26, 11, false},
		{"Migrate CI to GitHub Actions", `C:\code\infra`, "claude-sonnet-5", 50, 8, false},
		{"ترجمه‌ی رابط کاربری به فارسی", `C:\code\web-app`, "claude-opus-5", 75, 12, false},
		{"Database index tuning", `C:\code\payments-api`, "claude-opus-5", 120, 7, false},
		{"Release notes for v2.4", `C:\code\web-app`, "claude-haiku-4-5", 170, 3, false},
		{"پیاده‌سازی جستجوی فازی", `C:\code\search`, "claude-opus-5", 240, 10, false},
		{"Write the onboarding guide", `C:\code\docs`, "claude-sonnet-5", 400, 5, false},
		{"Load test the checkout flow", `C:\code\payments-api`, "claude-opus-5", 30, 9, true},
		{"مستندسازی API پرداخت", `C:\code\docs`, "claude-sonnet-5", 90, 4, true},
	}},
	{"personal", "5cb27541-7066-4bed-894e-d8ab69c60380", []chat{
		{"Plan a week in Lisbon", `C:\Users\demo`, "claude-opus-5", 5, 8, false},
		{"دستور پخت قورمه‌سبزی", `C:\Users\demo`, "claude-sonnet-5", 40, 5, false},
		{"Learn Rust ownership", `C:\code\rust-playground`, "claude-opus-5", 60, 13, false},
		{"کتاب‌هایی برای یادگیری طراحی سیستم", `C:\Users\demo`, "claude-opus-5", 200, 6, false},
		{"Budget spreadsheet formulas", `C:\Users\demo\finance`, "claude-haiku-4-5", 300, 4, false},
	}},
	{"client", "9b3d27e1-44c6-4a0b-8e5f-61a7c2d9f048", []chat{
		{"Landing page copy review", `C:\code\client-site`, "claude-sonnet-5", 10, 5, false},
		{"بهینه‌سازی سرعت سایت", `C:\code\client-site`, "claude-opus-5", 130, 7, false},
	}},
}

func main() {
	out := flag.String("out", "demo", "folder to write the demo setup into (replaced)")
	flag.Parse()
	now := time.Now()

	if err := os.RemoveAll(*out); err != nil {
		log.Fatal(err)
	}
	data := filepath.Join(*out, "claude")
	vault := filepath.Join(*out, "vault")
	projects := filepath.Join(*out, "projects")

	// Desktop is signed into "work", and every account has a saved login.
	must(writeJSON(filepath.Join(data, "config.json"), map[string]any{"lastKnownAccountUuid": accounts[0].uuid}))
	for i, a := range accounts {
		dir := filepath.Join(vault, a.profile)
		must(writeJSON(filepath.Join(dir, "config.json"), map[string]any{"lastKnownAccountUuid": a.uuid}))
		must(os.WriteFile(filepath.Join(dir, "_account"), []byte(a.uuid), 0o600))
		saved := now.Add(-time.Duration(i*27+2) * time.Hour)
		must(os.Chtimes(filepath.Join(dir, "config.json"), saved, saved))
		for n, c := range a.chats {
			session := uuid(i, n, 1)
			start := now.Add(-time.Duration(c.ageHours*float64(time.Hour)) - time.Duration(c.turns)*6*time.Minute)
			end := start.Add(time.Duration(c.turns) * 6 * time.Minute)
			must(writeTranscript(projects, session, c, start))
			if c.lost {
				continue
			}
			card := map[string]any{
				"sessionId":      "local_" + uuid(i, n, 2),
				"cliSessionId":   session,
				"cwd":            c.cwd,
				"originCwd":      c.cwd,
				"createdAt":      start.UnixMilli(),
				"lastActivityAt": end.UnixMilli(),
				"lastFocusedAt":  end.UnixMilli(),
				"model":          c.model,
				"isArchived":     false,
				"title":          c.title,
				"titleSource":    "auto",
				"permissionMode": "default",
			}
			must(writeJSON(filepath.Join(data, "claude-code-sessions", a.uuid, org, card["sessionId"].(string)+".json"), card))
		}
	}
	must(os.WriteFile(filepath.Join(vault, "_current_profile"), []byte("work"), 0o600))
	fmt.Printf("claude-switcher --data-dir %s --vault-dir %s --projects-dir %s\n", data, vault, projects)
}

var nonWord = regexp.MustCompile(`[^A-Za-z0-9]`)

// writeTranscript writes a short conversation with token usage, spread over the chat's time span.
func writeTranscript(projects, session string, c chat, start time.Time) error {
	path := filepath.Join(projects, nonWord.ReplaceAllString(c.cwd, "-"), session+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	lines := []any{map[string]any{"type": "custom-title", "customTitle": c.title, "sessionId": session}}
	parent := ""
	for t := 0; t < c.turns; t++ {
		at := start.Add(time.Duration(t) * 6 * time.Minute)
		user := uuid(len(c.title), t, 3)
		lines = append(lines, map[string]any{
			"type": "user", "uuid": user, "parentUuid": nullable(parent), "sessionId": session, "cwd": c.cwd,
			"timestamp": at.UTC().Format(time.RFC3339Nano), "isSidechain": false, "userType": "external",
			"message": map[string]any{"role": "user", "content": prompt(c, t)},
		})
		reply := uuid(len(c.title), t, 4)
		lines = append(lines, map[string]any{
			"type": "assistant", "uuid": reply, "parentUuid": user, "sessionId": session, "cwd": c.cwd,
			"timestamp": at.Add(40 * time.Second).UTC().Format(time.RFC3339Nano), "isSidechain": false,
			"message": map[string]any{
				"id": fmt.Sprintf("msg_%s_%02d", session[:8], t), "role": "assistant", "model": c.model,
				"content": []any{map[string]any{"type": "text", "text": answer(c, t)}},
				"usage": map[string]any{
					"input_tokens": 1200 + 90*t, "output_tokens": 850 + 170*((t*7)%11),
					"cache_creation_input_tokens": 4000, "cache_read_input_tokens": 26000 + 3000*t,
				},
			},
		})
		parent = reply
	}
	lines = append(lines, map[string]any{"type": "last-prompt", "lastPrompt": prompt(c, c.turns-1), "sessionId": session})
	for _, l := range lines {
		if err := enc.Encode(l); err != nil {
			return err
		}
	}
	return nil
}

func prompt(c chat, t int) string {
	if t == 0 {
		return c.title + ": let's start with the current state of the code."
	}
	return fmt.Sprintf("Next step %d for %q, and keep the tests green.", t+1, c.title)
}

func answer(c chat, t int) string {
	return fmt.Sprintf("Done: step %d of %q. I changed the smallest set of files and ran the tests; all pass.", t+1, c.title)
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// uuid is a stable, made-up v4-shaped id, so repeated runs give the same files.
func uuid(a, b, c int) string {
	h := sha256.Sum256(fmt.Appendf(nil, "%d/%d/%d", a, b, c))
	x := hex.EncodeToString(h[:16])
	return x[:8] + "-" + x[8:12] + "-4" + x[13:16] + "-8" + x[17:20] + "-" + x[20:32]
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

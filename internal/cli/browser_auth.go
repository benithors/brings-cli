package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

const bringLoginURL = "https://web.getbring.com/login"
const bringAppURL = "https://web.getbring.com/app"

// BrowserAuthResult holds auth data extracted from browser login.
type BrowserAuthResult struct {
	AccessToken    string
	UserUUID       string
	PublicUserUUID string
	UserName       string
	Email          string
}

func ensurePlaywright() (*playwright.Playwright, error) {
	pw, err := playwright.Run()
	if err == nil {
		return pw, nil
	}

	if installErr := playwright.Install(); installErr != nil {
		return nil, fmt.Errorf("playwright install failed: %w", installErr)
	}

	pw, err = playwright.Run()
	if err != nil {
		return nil, err
	}
	return pw, nil
}

// BrowserLogin opens a browser to log in and extracts tokens from localStorage.
func BrowserLogin(ctx context.Context) (BrowserAuthResult, error) {
	_ = ctx
	pw, err := ensurePlaywright()
	if err != nil {
		return BrowserAuthResult{}, err
	}
	defer pw.Stop()

	userDataDir := filepath.Join(os.TempDir(), "brings-browser-auth")

	browser := pw.Chromium
	contextOptions := playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless: playwright.Bool(false),
		Viewport: &playwright.Size{Width: 1280, Height: 800},
		Args:     []string{"--disable-blink-features=AutomationControlled"},
	}
	browserContext, err := browser.LaunchPersistentContext(userDataDir, contextOptions)
	if err != nil {
		return BrowserAuthResult{}, err
	}
	defer browserContext.Close()

	page, err := browserContext.NewPage()
	if err != nil {
		return BrowserAuthResult{}, err
	}

	_, err = page.Goto(bringAppURL, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateNetworkidle})
	if err != nil {
		return BrowserAuthResult{}, err
	}

	if strings.Contains(page.URL(), "/login") {
		fmt.Println()
		fmt.Println("Please log in to Bring! in the browser window...")
		fmt.Println("(The browser will close automatically after successful login)")
		fmt.Println()
		if err := waitForLogin(page, 5*time.Minute); err != nil {
			return BrowserAuthResult{}, err
		}
	}

	fmt.Println("Login detected, extracting token...")
	authPage := findBringAppPage(page)
	_ = waitForAuthStorage(authPage, 90*time.Second)
	page.WaitForTimeout(1000)

	return extractAuthFromStorage(authPage)
}

// BrowserLoginWithIntercept intercepts the auth response to capture tokens.
func BrowserLoginWithIntercept(ctx context.Context) (BrowserAuthResult, error) {
	_ = ctx
	pw, err := ensurePlaywright()
	if err != nil {
		return BrowserAuthResult{}, err
	}
	defer pw.Stop()

	home, _ := os.UserHomeDir()
	userDataDir := filepath.Join(home, ".brings-browser-profile")

	browser := pw.Chromium
	contextOptions := playwright.BrowserTypeLaunchPersistentContextOptions{
		Channel:  playwright.String("chrome"),
		Headless: playwright.Bool(false),
		Viewport: &playwright.Size{Width: 1280, Height: 800},
		Args:     []string{"--disable-blink-features=AutomationControlled"},
	}
	browserContext, err := browser.LaunchPersistentContext(userDataDir, contextOptions)
	if err != nil {
		return BrowserAuthResult{}, err
	}
	defer browserContext.Close()

	page, err := browserContext.NewPage()
	if err != nil {
		return BrowserAuthResult{}, err
	}

	authResponseCh := make(chan authResponsePayload, 1)
	page.OnResponse(func(response playwright.Response) {
		if response.Status() != 200 {
			return
		}
		if !strings.Contains(response.URL(), "/bringauth") {
			return
		}
		go func(resp playwright.Response) {
			var payload authResponsePayload
			if err := resp.JSON(&payload); err != nil {
				return
			}
			if payload.AccessToken == "" {
				return
			}
			select {
			case authResponseCh <- payload:
			default:
			}
		}(response)
	})

	_, err = page.Goto(bringLoginURL, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateNetworkidle})
	if err != nil {
		return BrowserAuthResult{}, err
	}

	fmt.Println()
	fmt.Println("Please log in to Bring! in the browser window...")
	fmt.Println("(The browser will close automatically after successful login)")
	fmt.Println()

	if err := waitForLogin(page, 5*time.Minute); err != nil {
		return BrowserAuthResult{}, err
	}

	if payload, ok := waitForAuthResponse(authResponseCh, 10*time.Second); ok {
		return finalizeAuthResult(BrowserAuthResult{
			AccessToken:    payload.AccessToken,
			UserUUID:       payload.UUID,
			PublicUserUUID: payload.PublicUUID,
			UserName:       payload.Name,
		})
	}

	fmt.Println("Extracting token from storage...")
	authPage := findBringAppPage(page)
	_ = waitForAuthStorage(authPage, 90*time.Second)
	page.WaitForTimeout(1000)
	return extractAuthFromStorage(authPage)
}

type authResponsePayload struct {
	Name         string `json:"name"`
	UUID         string `json:"uuid"`
	PublicUUID   string `json:"publicUuid"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func waitForLogin(page playwright.Page, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if loginDetected(page) {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("login timed out")
		}
		page.WaitForTimeout(1000)
	}
}

func waitForAuthResponse(ch <-chan authResponsePayload, timeout time.Duration) (authResponsePayload, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case payload := <-ch:
		return payload, true
	case <-timer.C:
		return authResponsePayload{}, false
	}
}

func loginDetected(page playwright.Page) bool {
	pages := []playwright.Page{page}
	if ctx := page.Context(); ctx != nil {
		pages = ctx.Pages()
	}
	for _, p := range pages {
		if !isBringURL(p.URL()) {
			continue
		}
		if !strings.Contains(p.URL(), "/login") {
			return true
		}
	}
	return false
}

func isBringURL(url string) bool {
	return strings.Contains(url, "web.getbring.com")
}

func findBringAppPage(page playwright.Page) playwright.Page {
	pages := []playwright.Page{page}
	if ctx := page.Context(); ctx != nil {
		pages = ctx.Pages()
	}
	for _, p := range pages {
		if isBringURL(p.URL()) && !strings.Contains(p.URL(), "/login") {
			return p
		}
	}
	return page
}

func waitForAuthStorage(page playwright.Page, timeout time.Duration) error {
	timeoutMs := float64(timeout.Milliseconds())
	_, err := page.WaitForFunction(
		`(function () {
			const accessToken = localStorage.getItem('accessToken') || sessionStorage.getItem('accessToken');
			return !!accessToken;
		})()`,
		nil,
		playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(timeoutMs)},
	)
	return err
}

func extractAuthFromStorage(page playwright.Page) (BrowserAuthResult, error) {
	eval, err := page.Evaluate(`() => JSON.stringify({
		accessToken: localStorage.getItem('accessToken') || sessionStorage.getItem('accessToken'),
		userUuid: localStorage.getItem('userUuid') || sessionStorage.getItem('userUuid'),
		publicUserUuid: localStorage.getItem('publicUserUuid') || sessionStorage.getItem('publicUserUuid'),
		userName: localStorage.getItem('userName') || sessionStorage.getItem('userName'),
		email: localStorage.getItem('email') || sessionStorage.getItem('email')
	})`)
	if err != nil {
		return BrowserAuthResult{}, err
	}

	jsonStr, ok := eval.(string)
	if !ok {
		return BrowserAuthResult{}, errors.New("unexpected localStorage result")
	}

	var data map[string]string
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return BrowserAuthResult{}, err
	}

	result := BrowserAuthResult{
		AccessToken:    data["accessToken"],
		UserUUID:       data["userUuid"],
		PublicUserUUID: data["publicUserUuid"],
		UserName:       data["userName"],
		Email:          data["email"],
	}

	if result.AccessToken == "" || result.UserUUID == "" {
		fallback, err := extractAuthFromStorageFallback(page)
		if err == nil && (fallback.AccessToken != "" || fallback.UserUUID != "") {
			if result.AccessToken == "" {
				result.AccessToken = fallback.AccessToken
			}
			if result.UserUUID == "" {
				result.UserUUID = fallback.UserUUID
			}
			if result.PublicUserUUID == "" {
				result.PublicUserUUID = fallback.PublicUserUUID
			}
			if result.UserName == "" {
				result.UserName = fallback.UserName
			}
			if result.Email == "" {
				result.Email = fallback.Email
			}
		}
	}

	return finalizeAuthResult(result)
}

func extractAuthFromStorageFallback(page playwright.Page) (BrowserAuthResult, error) {
	eval, err := page.Evaluate(`() => {
		const storages = [localStorage, sessionStorage];
		const result = {};
		const jwtRegex = /eyJ[A-Za-z0-9_-]+\\.[A-Za-z0-9_-]+\\.[A-Za-z0-9_-]+/;
		const trySet = (obj) => {
			if (!obj || typeof obj !== 'object') {
				return false;
			}
			const accessToken = obj.access_token || obj.accessToken || obj.token || (obj.auth && (obj.auth.access_token || obj.auth.accessToken));
			const userUuid = obj.userUuid || obj.user_uuid || obj.uuid || (obj.user && (obj.user.uuid || obj.user.userUuid));
			const publicUserUuid = obj.publicUuid || obj.public_user_uuid || obj.publicUserUuid;
			const userName = obj.name || obj.userName || (obj.user && (obj.user.name || obj.user.userName));
			const email = obj.email || (obj.user && obj.user.email);
			if (accessToken && !result.accessToken) {
				result.accessToken = accessToken;
			}
			if (userUuid && !result.userUuid) {
				result.userUuid = userUuid;
			}
			if (publicUserUuid && !result.publicUserUuid) {
				result.publicUserUuid = publicUserUuid;
			}
			if (userName && !result.userName) {
				result.userName = userName;
			}
			if (email && !result.email) {
				result.email = email;
			}
			return result.accessToken || result.userUuid;
		};

		for (const storage of storages) {
			for (let i = 0; i < storage.length; i += 1) {
				const key = storage.key(i);
				const value = storage.getItem(key);
				if (!value) {
					continue;
				}
				if (!result.accessToken) {
					const jwt = value.match(jwtRegex);
					if (jwt && jwt[0]) {
						result.accessToken = jwt[0];
					}
				}
				try {
					const parsed = JSON.parse(value);
					if (trySet(parsed)) {
						if (result.accessToken && result.userUuid) {
							return JSON.stringify(result);
						}
					}
				} catch (err) {
					// ignore non-JSON values
				}
			}
		}
		return JSON.stringify(result);
	}`)
	if err != nil {
		return BrowserAuthResult{}, err
	}

	jsonStr, ok := eval.(string)
	if !ok {
		return BrowserAuthResult{}, errors.New("unexpected storage scan result")
	}

	var data map[string]string
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return BrowserAuthResult{}, err
	}

	return BrowserAuthResult{
		AccessToken:    data["accessToken"],
		UserUUID:       data["userUuid"],
		PublicUserUUID: data["publicUserUuid"],
		UserName:       data["userName"],
		Email:          data["email"],
	}, nil
}

func finalizeAuthResult(result BrowserAuthResult) (BrowserAuthResult, error) {
	if result.AccessToken == "" {
		return BrowserAuthResult{}, errors.New("failed to extract authentication data")
	}
	if result.UserUUID != "" {
		return result, nil
	}
	claims, err := decodeJWT(result.AccessToken)
	if err != nil || claims.Sub == "" {
		return BrowserAuthResult{}, errors.New("failed to extract authentication data")
	}
	parts := strings.Split(claims.Sub, ":")
	result.UserUUID = parts[len(parts)-1]
	return result, nil
}

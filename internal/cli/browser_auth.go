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
	page.WaitForTimeout(1000)

	return extractAuthFromLocalStorage(page)
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

	var authResponse *authResponsePayload
	page.OnResponse(func(response playwright.Response) {
		if response.Status() != 200 {
			return
		}
		if !strings.Contains(response.URL(), "/bringauth") {
			return
		}
		var payload authResponsePayload
		if err := response.JSON(&payload); err != nil {
			return
		}
		if payload.AccessToken != "" {
			authResponse = &payload
		}
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

	if authResponse != nil && authResponse.AccessToken != "" {
		return BrowserAuthResult{
			AccessToken:    authResponse.AccessToken,
			UserUUID:       authResponse.UUID,
			PublicUserUUID: authResponse.PublicUUID,
			UserName:       authResponse.Name,
		}, nil
	}

	fmt.Println("Extracting token from localStorage...")
	page.WaitForTimeout(1000)
	return extractAuthFromLocalStorage(page)
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
		if !strings.Contains(page.URL(), "/login") {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("login timed out")
		}
		page.WaitForTimeout(1000)
	}
}

func extractAuthFromLocalStorage(page playwright.Page) (BrowserAuthResult, error) {
	eval, err := page.Evaluate(`() => JSON.stringify({
        accessToken: localStorage.getItem('accessToken'),
        userUuid: localStorage.getItem('userUuid'),
        publicUserUuid: localStorage.getItem('publicUserUuid'),
        userName: localStorage.getItem('userName'),
        email: localStorage.getItem('email')
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

	if data["accessToken"] == "" || data["userUuid"] == "" {
		return BrowserAuthResult{}, errors.New("failed to extract authentication data")
	}

	return BrowserAuthResult{
		AccessToken:    data["accessToken"],
		UserUUID:       data["userUuid"],
		PublicUserUUID: data["publicUserUuid"],
		UserName:       data["userName"],
		Email:          data["email"],
	}, nil
}

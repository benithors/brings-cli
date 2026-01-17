package cli

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func runCLI(args []string) (string, string, int) {
	oldStdout := os.Stdout
	oldStderr := os.Stderr

	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	code := Run(args)

	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	outBytes, _ := io.ReadAll(rOut)
	errBytes, _ := io.ReadAll(rErr)

	return string(outBytes), string(errBytes), code
}

func TestListsCommandOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bringusers/user-uuid/lists":
			if r.Header.Get("Authorization") != "Bearer token" {
				t.Fatalf("missing authorization header")
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"lists": []map[string]string{{"listUuid": "list-1", "name": "Groceries"}},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_BASE_URL", server.URL)

	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"lists"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, "Shopping Lists:") || !strings.Contains(stdout, "Groceries (list-1)") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestItemsCommandOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bringusers/user-uuid/lists":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"lists": []map[string]string{{"listUuid": "list-1", "name": "Groceries"}},
			})
		case "/bringlists/list-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"purchase": []map[string]string{{"name": "Milk", "specification": "2%"}},
				"recently": []map[string]string{{"name": "Bread"}},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_BASE_URL", server.URL)

	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"items", "--all"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, "To Purchase:") || !strings.Contains(stdout, "Milk (2%)") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
	if !strings.Contains(stdout, "Recent Items:") || !strings.Contains(stdout, "Bread") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestItemsCommandUsesExplicitList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bringlists/list-2":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"purchase": []map[string]string{{"name": "Eggs", "specification": ""}},
				"recently": []map[string]string{},
			})
		case "/bringusers/user-uuid/lists":
			t.Fatalf("should not fetch lists when --list is provided")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_BASE_URL", server.URL)
	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"items", "--list", "list-2"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if strings.Contains(stdout, "List:") {
		t.Fatalf("unexpected list header: %s", stdout)
	}
	if !strings.Contains(stdout, "Eggs") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestLoginTokenFlowSavesConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bringusers/user-uuid":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"userUuid":       "user-uuid",
				"publicUserUuid": "public-uuid",
				"email":          "test@example.com",
				"name":           "Tester",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_BASE_URL", server.URL)

	token := buildJWT(map[string]interface{}{
		"sub": "BRN:TEST:USER:user-uuid",
		"exp": float64(4102444800),
	})

	stdout, stderr, code := runCLI([]string{"login", "--token", token})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, "Logged in as") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}

	config := loadConfig()
	if config.AccessToken != token {
		t.Fatalf("token not saved")
	}
	if config.UserUUID != "user-uuid" || config.PublicUserUUID != "public-uuid" {
		t.Fatalf("user info not saved")
	}
}

func TestConfigCommandSetsServings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stdout, stderr, code := runCLI([]string{"config", "servings", "4"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, "Set servings = 4") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
	config := loadConfig()
	if config.Servings != 4 {
		t.Fatalf("servings not saved")
	}
}

func TestAddCommandUsesDefaultList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bringusers/user-uuid/lists":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"lists": []map[string]string{{"listUuid": "list-1", "name": "Groceries"}},
			})
		case "/bringlists/list-1":
			body, _ := io.ReadAll(r.Body)
			values, _ := url.ParseQuery(string(body))
			if values.Get("purchase") != "Milk" {
				t.Fatalf("unexpected purchase: %s", values.Get("purchase"))
			}
			if values.Get("specification") != "2%" {
				t.Fatalf("unexpected spec: %s", values.Get("specification"))
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_BASE_URL", server.URL)
	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"add", "Milk", "--spec", "2%"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, "Added \"Milk\" (2%) to Groceries") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestRemoveCommandUsesExplicitList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bringlists/list-1" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		values, _ := url.ParseQuery(string(body))
		if values.Get("remove") != "Milk" {
			t.Fatalf("unexpected remove: %s", values.Get("remove"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_BASE_URL", server.URL)
	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"remove", "Milk", "--list", "list-1"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, "Removed \"Milk\" from list-1") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestCompleteCommandUsesExplicitList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bringlists/list-1" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		values, _ := url.ParseQuery(string(body))
		if values.Get("recently") != "Milk" {
			t.Fatalf("unexpected recently: %s", values.Get("recently"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_BASE_URL", server.URL)
	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"complete", "Milk", "--list", "list-1"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, "Completed \"Milk\" in list-1") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestAddRecipeFiltersPantryAndScales(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/bringtemplates/content/"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"title": "Pancakes",
				"yield": "2",
				"items": []map[string]interface{}{
					{"itemId": "Milk", "spec": "2 l", "stock": false},
					{"itemId": "Flour", "spec": "500 g"},
					{"itemId": "Salt", "spec": "1 tsp", "stock": true},
				},
			})
		case r.URL.Path == "/bringusers/user-uuid/lists":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"lists": []map[string]string{{"listUuid": "list-1", "name": "Groceries"}},
			})
		case r.URL.Path == "/bringlists/list-1/items":
			body, _ := io.ReadAll(r.Body)
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			changes, ok := payload["changes"].([]interface{})
			if !ok || len(changes) != 2 {
				t.Fatalf("unexpected changes payload: %v", payload["changes"])
			}
			first := changes[0].(map[string]interface{})
			second := changes[1].(map[string]interface{})
			specs := []string{first["spec"].(string), second["spec"].(string)}
			if !containsString(specs, "4 l") || !containsString(specs, "1000 g") {
				t.Fatalf("unexpected scaled specs: %v", specs)
			}
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_BASE_URL", server.URL)
	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"add-recipe", "recipe-1", "--servings", "4"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, "Added 2 ingredients from \"Pancakes\" to Groceries") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestNotifyCommandSendsUrgentMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bringnotifications/lists/list-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected content-type: %s", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if payload["listNotificationType"] != "URGENT_MESSAGE" {
			t.Fatalf("unexpected notification type: %v", payload["listNotificationType"])
		}
		args, ok := payload["arguments"].([]interface{})
		if !ok || len(args) != 1 || args[0] != "Milk" {
			t.Fatalf("unexpected arguments: %v", payload["arguments"])
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_BASE_URL", server.URL)
	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"notify", "URGENT_MESSAGE", "--message", "Milk", "--list", "list-1"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, "Notification \"URGENT_MESSAGE\" sent to list-1") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestActivityCommandOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bringusers/user-uuid/lists":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"lists": []map[string]string{{"listUuid": "list-1", "name": "Groceries"}},
			})
		case "/bringlists/list-1/activity":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"timeline": []map[string]interface{}{
					{"type": "LIST_ITEMS_ADDED", "timestamp": "2024-01-01T12:00:00Z", "content": map[string]interface{}{"itemId": "Milk"}},
				},
				"totalEvents": 1,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_BASE_URL", server.URL)
	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"activity"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, "Activity for: Groceries") || !strings.Contains(stdout, "LIST_ITEMS_ADDED") || !strings.Contains(stdout, "Milk") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestRecipeCommandOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/bringtemplates/content/") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"title": "Pancakes",
				"yield": "2",
				"nutrition": map[string]interface{}{
					"calories": "200",
				},
				"items": []map[string]interface{}{
					{"itemId": "Milk", "spec": "2 l"},
					{"itemId": "Salt", "spec": "1 tsp", "stock": true},
				},
				"instructions": []string{"Mix", "Bake"},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_BASE_URL", server.URL)
	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid", Servings: 4}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"recipe", "recipe-1", "--format", "human"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, "Pancakes") || !strings.Contains(stdout, "Servings: 2 -> scaled to 4") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
	if !strings.Contains(stdout, "4 l Milk") || !strings.Contains(stdout, "2 tsp Salt (pantry)") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestRecipeCommandJSONOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/bringtemplates/content/") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"title": "Pasta",
				"nutrition": map[string]interface{}{
					"calories": "200",
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_BASE_URL", server.URL)
	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"recipe", "recipe-1"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}
	if payload["id"] != "recipe-1" {
		t.Fatalf("unexpected id: %#v", payload["id"])
	}
	if payload["title"] != "Pasta" {
		t.Fatalf("unexpected title: %#v", payload["title"])
	}
	nutrition, ok := payload["nutrition"].(map[string]interface{})
	if !ok || nutrition["calories"] != "200" {
		t.Fatalf("unexpected nutrition: %#v", payload["nutrition"])
	}
}

func TestRecipeCommandImagesOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/bringtemplates/content/") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"title":    "Pasta",
				"imageUrl": "https://example.com/pasta.jpg",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_BASE_URL", server.URL)
	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"recipe", "recipe-1", "--format", "human", "--images"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, "Image: https://example.com/pasta.jpg") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestInspirationsFiltersOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bringusers/user-uuid/inspirationstreamfilters" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"filters": []map[string]interface{}{
				{"tag": "mine", "name": "My Recipes"},
			},
		})
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_BASE_URL", server.URL)
	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"inspirations", "--filters", "--format", "human"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, "mine: My Recipes") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestInspirationsListOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/inspirations") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"entries": []map[string]interface{}{
				{"content": map[string]interface{}{
					"title":       "Soup",
					"author":      "Chef",
					"likeCount":   5,
					"contentUuid": "abc-123",
					"type":        "recipe",
					"tags":        []string{"mine", "seasonal"},
				}},
			},
			"total": 1,
		})
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_BASE_URL", server.URL)
	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"inspirations", "--format", "human"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, "Soup") || !strings.Contains(stdout, "ID: abc-123") || !strings.Contains(stdout, "Tags: seasonal") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestInspirationsJSONOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/inspirations") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"entries": []map[string]interface{}{
				{"content": map[string]interface{}{
					"title":       "Soup",
					"contentUuid": "abc-123",
					"imageUrl":    "https://example.com/soup.jpg",
				}},
			},
			"total": 1,
		})
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_BASE_URL", server.URL)
	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"inspirations"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}
	entries, ok := payload["entries"].([]interface{})
	if !ok || len(entries) != 1 {
		t.Fatalf("unexpected entries: %#v", payload["entries"])
	}
	entry := entries[0].(map[string]interface{})
	if entry["title"] != "Soup" {
		t.Fatalf("unexpected title: %#v", entry["title"])
	}
	if entry["imageUrl"] != "https://example.com/soup.jpg" {
		t.Fatalf("unexpected imageUrl: %#v", entry["imageUrl"])
	}
	if entry["author"] != nil || entry["likes"] != nil || entry["linkOutUrl"] != nil {
		t.Fatalf("unexpected extra fields: %#v", entry)
	}
}

func TestInspirationsImagesOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/inspirations") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"entries": []map[string]interface{}{
				{"content": map[string]interface{}{
					"title":       "Toast",
					"contentUuid": "img-123",
					"imageUrl":    "https://example.com/toast.jpg",
				}},
			},
			"total": 1,
		})
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_BASE_URL", server.URL)
	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"inspirations", "--format", "human", "--images"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, "Image: https://example.com/toast.jpg") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestStatusShowsExpiredWarning(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	expiredToken := buildJWT(map[string]interface{}{
		"exp": float64(time.Now().Add(-2 * time.Hour).Unix()),
	})
	if err := saveConfig(Config{AccessToken: expiredToken, UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"status"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, "Warning: Token has expired") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestNotLoggedInReturnsError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stdout, stderr, code := runCLI([]string{"lists"})
	if code == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if stdout != "" {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
	if !strings.Contains(stderr, "Not logged in") {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
}

func TestCatalogCommandOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/locale/catalog.en-US.json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"language": "en-US",
			"catalog": map[string]interface{}{
				"sections": []map[string]interface{}{
					{"name": "Dairy", "items": []map[string]string{{"name": "Milk"}, {"name": "Cheese"}}},
				},
			},
		})
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_WEB_BASE_URL", server.URL)
	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"catalog", "en-US"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, "Catalog (en-US):") || !strings.Contains(stdout, "Dairy:") || !strings.Contains(stdout, "Milk") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestAddCommandHandlesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bringusers/user-uuid/lists":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"lists": []map[string]string{{"listUuid": "list-1", "name": "Groceries"}},
			})
		case "/bringlists/list-1":
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   "invalid_item",
				"message": "Item name not allowed",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_BASE_URL", server.URL)
	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"add", "Milk"})
	if code == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if stdout != "" {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
	if !strings.Contains(stderr, "Item name not allowed") {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
}

func TestNotifyHandlesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bringnotifications/lists/list-1" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "invalid_token",
			"message": "Token expired",
		})
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_BASE_URL", server.URL)
	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"notify", "GOING_SHOPPING", "--list", "list-1"})
	if code == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if stdout != "" {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
	if !strings.Contains(stderr, "Token expired") {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
}

func TestRecipeCommandHandlesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "not_found",
			"message": "Recipe not found",
		})
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_BASE_URL", server.URL)
	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"recipe", "missing"})
	if code == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if stdout != "" {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
	if !strings.Contains(stderr, "Recipe not found") {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
}

func TestStatusNotLoggedIn(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stdout, stderr, code := runCLI([]string{"status"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, "Not logged in") || !strings.Contains(stdout, "Run `brings login`") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestNotifyUrgentMissingMessage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"notify", "URGENT_MESSAGE", "--list", "list-1"})
	if code == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if stdout != "" {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
	if !strings.Contains(stderr, "URGENT_MESSAGE") {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
}

func TestConfigUnknownKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stdout, stderr, code := runCLI([]string{"config", "unknown"})
	if code == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if stdout != "" {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
	if !strings.Contains(stderr, "Unknown config key") {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
}

func TestItemsCommandNoLists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bringusers/user-uuid/lists" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"lists": []map[string]string{},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRINGS_BASE_URL", server.URL)
	if err := saveConfig(Config{AccessToken: "token", UserUUID: "user-uuid"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, stderr, code := runCLI([]string{"items"})
	if code == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if stdout != "" {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
	if !strings.Contains(stderr, "no shopping lists found") {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
}

func buildJWT(payload map[string]interface{}) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	bytes, _ := json.Marshal(payload)
	return header + "." + base64.RawURLEncoding.EncodeToString(bytes) + "."
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

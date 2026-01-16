package bring

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestLoginSetsHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bringauth" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); !strings.Contains(ct, "application/x-www-form-urlencoded") {
			t.Fatalf("unexpected content-type: %s", ct)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse body: %v", err)
		}
		if values.Get("email") != "user@example.com" {
			t.Fatalf("unexpected email: %s", values.Get("email"))
		}
		if values.Get("password") != "secret" {
			t.Fatalf("unexpected password: %s", values.Get("password"))
		}

		resp := AuthSuccessResponse{
			Name:         "Tester",
			UUID:         "user-uuid",
			PublicUUID:   "public-uuid",
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := New(BringOptions{Mail: "user@example.com", Password: "secret", URL: server.URL})
	if err := client.Login(context.Background()); err != nil {
		t.Fatalf("login failed: %v", err)
	}

	if client.Name != "Tester" {
		t.Fatalf("unexpected name: %s", client.Name)
	}
	if client.uuid != "user-uuid" {
		t.Fatalf("unexpected uuid: %s", client.uuid)
	}
	if client.bearerToken != "access-token" {
		t.Fatalf("unexpected access token: %s", client.bearerToken)
	}
	if client.headers["Authorization"] != "Bearer access-token" {
		t.Fatalf("unexpected auth header: %s", client.headers["Authorization"])
	}
	if client.headers["X-BRING-USER-UUID"] != "user-uuid" {
		t.Fatalf("unexpected user header: %s", client.headers["X-BRING-USER-UUID"])
	}
	if client.headers["X-BRING-PUBLIC-USER-UUID"] != "public-uuid" {
		t.Fatalf("unexpected public header: %s", client.headers["X-BRING-PUBLIC-USER-UUID"])
	}
	if client.putHeaders["Content-Type"] == "" {
		t.Fatalf("expected put headers content-type to be set")
	}
}

func TestLoadListsUsesAuthHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bringusers/user-uuid/lists" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer access-token" {
			t.Fatalf("missing authorization header")
		}
		if r.Header.Get("X-BRING-USER-UUID") != "user-uuid" {
			t.Fatalf("missing user header")
		}
		resp := LoadListsResponse{Lists: []LoadListsEntry{{ListUUID: "list-1", Name: "Groceries"}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := FromToken(TokenAuthOptions{AccessToken: "access-token", UserUUID: "user-uuid", URL: server.URL})
	lists, err := client.LoadLists(context.Background())
	if err != nil {
		t.Fatalf("load lists failed: %v", err)
	}
	if len(lists.Lists) != 1 || lists.Lists[0].ListUUID != "list-1" {
		t.Fatalf("unexpected lists response")
	}
}

func TestSaveItemFormBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bringlists/list-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); !strings.Contains(ct, "application/x-www-form-urlencoded") {
			t.Fatalf("unexpected content-type: %s", ct)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse body: %v", err)
		}
		if values.Get("purchase") != "Milk" {
			t.Fatalf("unexpected purchase: %s", values.Get("purchase"))
		}
		if values.Get("specification") != "2%" {
			t.Fatalf("unexpected specification: %s", values.Get("specification"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := FromToken(TokenAuthOptions{AccessToken: "access-token", UserUUID: "user-uuid", URL: server.URL})
	if _, err := client.SaveItem(context.Background(), "list-1", "Milk", "2%"); err != nil {
		t.Fatalf("save item failed: %v", err)
	}
}

func TestRemoveItemFormBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bringlists/list-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse body: %v", err)
		}
		if values.Get("remove") != "Milk" {
			t.Fatalf("unexpected remove: %s", values.Get("remove"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := FromToken(TokenAuthOptions{AccessToken: "access-token", UserUUID: "user-uuid", URL: server.URL})
	if _, err := client.RemoveItem(context.Background(), "list-1", "Milk"); err != nil {
		t.Fatalf("remove item failed: %v", err)
	}
}

func TestMoveToRecentFormBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bringlists/list-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse body: %v", err)
		}
		if values.Get("recently") != "Milk" {
			t.Fatalf("unexpected recently: %s", values.Get("recently"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := FromToken(TokenAuthOptions{AccessToken: "access-token", UserUUID: "user-uuid", URL: server.URL})
	if _, err := client.MoveToRecentList(context.Background(), "list-1", "Milk"); err != nil {
		t.Fatalf("move to recent failed: %v", err)
	}
}

func TestBatchUpdateItemsPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bringlists/list-1/items" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected content-type: %s", r.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		changes, ok := payload["changes"].([]interface{})
		if !ok || len(changes) != 2 {
			t.Fatalf("unexpected changes payload")
		}
		first := changes[0].(map[string]interface{})
		if first["operation"] != string(BringItemToPurchase) {
			t.Fatalf("unexpected operation: %v", first["operation"])
		}
		second := changes[1].(map[string]interface{})
		if second["operation"] != string(BringItemRemove) {
			t.Fatalf("unexpected operation: %v", second["operation"])
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := FromToken(TokenAuthOptions{AccessToken: "access-token", UserUUID: "user-uuid", URL: server.URL})
	items := []BatchUpdateItem{
		{ItemID: "Bread"},
		{ItemID: "Milk", Operation: BringItemRemove},
	}
	if _, err := client.BatchUpdateItems(context.Background(), "list-1", items, BringItemToPurchase); err != nil {
		t.Fatalf("batch update failed: %v", err)
	}
}

func TestGetItemsErrorPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid_grant", Message: "JWT access token is not valid"})
	}))
	defer server.Close()

	client := FromToken(TokenAuthOptions{AccessToken: "access-token", UserUUID: "user-uuid", URL: server.URL})
	_, err := client.GetItems(context.Background(), "list-1")
	if err == nil || !strings.Contains(err.Error(), "JWT access token is not valid") {
		t.Fatalf("expected JWT error, got %v", err)
	}
}

func TestHTTPErrorPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid_grant", Message: "token expired"})
	}))
	defer server.Close()

	client := FromToken(TokenAuthOptions{AccessToken: "access-token", UserUUID: "user-uuid", URL: server.URL})
	_, err := client.LoadLists(context.Background())
	if err == nil || !strings.Contains(err.Error(), "token expired") {
		t.Fatalf("expected error message, got %v", err)
	}
}

func TestNotifyUrgentMessagePayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bringnotifications/lists/list-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected content-type: %s", r.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if payload["listNotificationType"] != string(NotifyUrgentMessage) {
			t.Fatalf("unexpected notification type: %v", payload["listNotificationType"])
		}
		if payload["senderPublicUserUuid"] != "public-uuid" {
			t.Fatalf("unexpected sender: %v", payload["senderPublicUserUuid"])
		}
		args, ok := payload["arguments"].([]interface{})
		if !ok || len(args) != 1 || args[0] != "Milk" {
			t.Fatalf("unexpected arguments: %v", payload["arguments"])
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := FromToken(TokenAuthOptions{
		AccessToken:    "access-token",
		UserUUID:       "user-uuid",
		PublicUserUUID: "public-uuid",
		URL:            server.URL,
	})
	if _, err := client.Notify(context.Background(), "list-1", NotifyUrgentMessage, "Milk", nil, "", "", ""); err != nil {
		t.Fatalf("notify failed: %v", err)
	}
}

func TestUserLocaleObjectUnmarshal(t *testing.T) {
	payload := []byte(`{"email":"test@example.com","emailVerified":true,"premiumConfiguration":{},"publicUserUuid":"pub","userLocale":{"language":"en","country":"US"},"userUuid":"user"}`)
	var resp GetUserAccountResponse
	if err := json.Unmarshal(payload, &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.UserLocale.String() != "en-US" {
		t.Fatalf("unexpected locale: %s", resp.UserLocale.String())
	}
}

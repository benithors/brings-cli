package bring

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoadCatalogUsesWebBase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/locale/catalog.en-US.json" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"language": "en-US",
			"catalog": map[string]interface{}{
				"sections": []map[string]interface{}{
					{"sectionId": "1", "name": "Dairy", "items": []map[string]string{{"itemId": "milk", "name": "Milk"}}},
				},
			},
		})
	}))
	defer server.Close()

	t.Setenv("BRINGS_WEB_BASE_URL", server.URL)
	client := New(BringOptions{})
	catalog, err := client.LoadCatalog(context.Background(), "en-US")
	if err != nil {
		t.Fatalf("load catalog failed: %v", err)
	}
	if catalog.Language != "en-US" || len(catalog.Catalog.Sections) != 1 {
		t.Fatalf("unexpected catalog response")
	}
}

func TestLoadTranslationsUsesWebBase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/locale/articles.en-US.json" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"milk": "Milk"})
	}))
	defer server.Close()

	t.Setenv("BRINGS_WEB_BASE_URL", server.URL)
	client := New(BringOptions{})
	translations, err := client.LoadTranslations(context.Background(), "en-US")
	if err != nil {
		t.Fatalf("load translations failed: %v", err)
	}
	if translations["milk"] != "Milk" {
		t.Fatalf("unexpected translations response")
	}
}

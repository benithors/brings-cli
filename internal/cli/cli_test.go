package cli

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDecodeJWT(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"BRN:TEST:USER:uuid-123","email":"test@example.com","exp":1700000000}`))
	token := header + "." + payload + "."

	claims, err := decodeJWT(token)
	if err != nil {
		t.Fatalf("decodeJWT failed: %v", err)
	}
	if claims.Sub != "BRN:TEST:USER:uuid-123" {
		t.Fatalf("unexpected sub: %s", claims.Sub)
	}
	if claims.Email != "test@example.com" {
		t.Fatalf("unexpected email: %s", claims.Email)
	}
	if claims.Exp != 1700000000 {
		t.Fatalf("unexpected exp: %d", claims.Exp)
	}
}

func TestDecodeJWTInvalid(t *testing.T) {
	_, err := decodeJWT("not-a-jwt")
	if err == nil {
		t.Fatalf("expected decode error")
	}
}

func TestScaleSpec(t *testing.T) {
	if got := scaleSpec("2.5 kg", 2); got != "5 kg" {
		t.Fatalf("unexpected scale: %s", got)
	}
	if got := scaleSpec("2,5 kg", 2); got != "5 kg" {
		t.Fatalf("unexpected scale: %s", got)
	}
	if got := scaleSpec("salt", 2); got != "salt" {
		t.Fatalf("unexpected scale: %s", got)
	}
	if got := scaleSpec("1-2 tbsp", 2); got != "1-2 tbsp" {
		t.Fatalf("unexpected scale: %s", got)
	}
	if got := scaleSpec("1/2 cup", 2); got != "1/2 cup" {
		t.Fatalf("unexpected scale: %s", got)
	}
}

func TestParseServings(t *testing.T) {
	if got := parseServings(nil, "", 0); got != 0 {
		t.Fatalf("unexpected servings: %d", got)
	}
	if got := parseServings("4"); got != 4 {
		t.Fatalf("unexpected servings: %d", got)
	}
	if got := parseServings(2.0); got != 2 {
		t.Fatalf("unexpected servings: %d", got)
	}
}

func TestConfigPersistence(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := Config{
		AccessToken:    "token",
		UserUUID:       "user",
		PublicUserUUID: "public",
		UserName:       "Tester",
		Email:          "test@example.com",
		Servings:       3,
		DefaultList:    "list-1",
		Locale:         "en-US",
	}

	if err := saveConfig(cfg); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	loaded := loadConfig()
	if loaded.AccessToken != cfg.AccessToken || loaded.UserUUID != cfg.UserUUID {
		t.Fatalf("loaded config mismatch")
	}

	path := filepath.Join(tmp, ".config", "brings", "config.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file: %v", err)
	}

	if err := clearConfig(); err != nil {
		t.Fatalf("clear config failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected config file removed")
	}
}

func TestJWTExpiryLogic(t *testing.T) {
	payload := map[string]interface{}{
		"sub": "BRN:TEST:USER:uuid-123",
		"exp": time.Now().Add(2 * time.Hour).Unix(),
	}
	payloadBytes, _ := json.Marshal(payload)
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	token := header + "." + base64.RawURLEncoding.EncodeToString(payloadBytes) + "."

	claims, err := decodeJWT(token)
	if err != nil {
		t.Fatalf("decodeJWT failed: %v", err)
	}
	if claims.Exp == 0 {
		t.Fatalf("expected exp to be set")
	}
}

package bring

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.getbring.com/rest/v2/"
const bringAPIKey = "cof4Nc6D8saplXjE3h3HXqHH8m7VU2i1Gs0g85Sp"

// Bring is a client for the Bring! API.
type Bring struct {
	mail         string
	password     string
	url          string
	uuid         string
	headers      map[string]string
	Name         string
	PublicUUID   string
	bearerToken  string
	refreshToken string
	putHeaders   map[string]string
	client       *http.Client
}

// New creates a Bring client using email/password credentials.
func New(options BringOptions) *Bring {
	baseURL := normalizeBaseURL(options.URL)
	return &Bring{
		mail:     options.Mail,
		password: options.Password,
		url:      baseURL,
		uuid:     options.UUID,
		headers: map[string]string{
			"X-BRING-API-KEY":       bringAPIKey,
			"X-BRING-CLIENT":        "webApp",
			"X-BRING-CLIENT-SOURCE": "webApp",
			"X-BRING-COUNTRY":       "DE",
		},
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// FromToken creates a Bring client using an existing access token.
func FromToken(options TokenAuthOptions) *Bring {
	bring := New(BringOptions{URL: options.URL})
	bring.setAuthHeaders(options.UserUUID, options.AccessToken, options.PublicUserUUID)
	return bring
}

// Login authenticates using email/password and sets auth headers.
func (b *Bring) Login(ctx context.Context) error {
	form := url.Values{}
	form.Set("email", b.mail)
	form.Set("password", b.password)

	headers := map[string]string{
		"Content-Type": "application/x-www-form-urlencoded; charset=UTF-8",
	}
	body, _, err := b.doRequest(ctx, http.MethodPost, b.url+"bringauth", headers, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("cannot login: %w", err)
	}

	if err := decodeError(body); err != nil {
		return fmt.Errorf("cannot login: %w", err)
	}

	var data AuthSuccessResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return fmt.Errorf("cannot login: %w", err)
	}

	b.Name = data.Name
	b.setAuthHeaders(data.UUID, data.AccessToken, data.PublicUUID)
	b.refreshToken = data.RefreshToken
	return nil
}

// LoadLists loads all shopping lists.
func (b *Bring) LoadLists(ctx context.Context) (LoadListsResponse, error) {
	var lists LoadListsResponse
	body, _, err := b.doRequest(ctx, http.MethodGet, b.url+"bringusers/"+b.uuid+"/lists", b.headers, nil)
	if err != nil {
		return lists, fmt.Errorf("cannot get lists: %w", err)
	}
	if err := decodeJSON(body, &lists); err != nil {
		return lists, fmt.Errorf("cannot get lists: %w", err)
	}
	return lists, nil
}

// GetItems gets all items from a list.
func (b *Bring) GetItems(ctx context.Context, listUUID string) (GetItemsResponse, error) {
	var items GetItemsResponse
	body, _, err := b.doRequest(ctx, http.MethodGet, b.url+"bringlists/"+listUUID, b.headers, nil)
	if err != nil {
		return items, fmt.Errorf("cannot get items for list %s: %w", listUUID, err)
	}
	if err := decodeJSON(body, &items); err != nil {
		return items, fmt.Errorf("cannot get items for list %s: %w", listUUID, err)
	}
	return items, nil
}

// GetItemsDetails gets detailed information about items from a list.
func (b *Bring) GetItemsDetails(ctx context.Context, listUUID string) ([]GetItemsDetailsEntry, error) {
	var items []GetItemsDetailsEntry
	body, _, err := b.doRequest(ctx, http.MethodGet, b.url+"bringlists/"+listUUID+"/details", b.headers, nil)
	if err != nil {
		return items, fmt.Errorf("cannot get detailed items for list %s: %w", listUUID, err)
	}
	if err := decodeJSON(body, &items); err != nil {
		return items, fmt.Errorf("cannot get detailed items for list %s: %w", listUUID, err)
	}
	return items, nil
}

// GetUserAccount returns account information for the current user.
func (b *Bring) GetUserAccount(ctx context.Context) (GetUserAccountResponse, error) {
	var account GetUserAccountResponse
	body, _, err := b.doRequest(ctx, http.MethodGet, b.url+"bringusers/"+b.uuid, b.headers, nil)
	if err != nil {
		return account, fmt.Errorf("cannot get user account: %w", err)
	}
	if err := decodeJSON(body, &account); err != nil {
		return account, fmt.Errorf("cannot get user account: %w", err)
	}
	b.PublicUUID = account.PublicUserUUID
	b.headers["X-BRING-PUBLIC-USER-UUID"] = account.PublicUserUUID
	return account, nil
}

// SaveItem adds an item to a list.
func (b *Bring) SaveItem(ctx context.Context, listUUID, itemName, specification string) (string, error) {
	form := url.Values{}
	form.Set("purchase", itemName)
	form.Set("recently", "")
	form.Set("specification", specification)
	form.Set("remove", "")
	form.Set("sender", "null")

	body, _, err := b.doRequest(ctx, http.MethodPut, b.url+"bringlists/"+listUUID, b.putHeaders, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("cannot save item %s (%s) to %s: %w", itemName, specification, listUUID, err)
	}
	if err := decodeError(body); err != nil {
		return "", fmt.Errorf("cannot save item %s (%s) to %s: %w", itemName, specification, listUUID, err)
	}
	return string(body), nil
}

// UpdateItem updates an item on a list.
func (b *Bring) UpdateItem(ctx context.Context, listUUID, itemName, specification, itemUUID string) (string, error) {
	item := BatchUpdateItem{ItemID: itemName, Spec: specification, UUID: itemUUID}
	resp, err := b.BatchUpdateItems(ctx, listUUID, []BatchUpdateItem{item}, BringItemToPurchase)
	if err != nil {
		return "", fmt.Errorf("cannot update item %s (%s) in %s: %w", itemName, specification, listUUID, err)
	}
	return resp, nil
}

// SaveItemImage saves an image for an item.
func (b *Bring) SaveItemImage(ctx context.Context, itemUUID string, image Image) (map[string]string, error) {
	form := url.Values{}
	form.Set("imageData", image.ImageData)

	body, _, err := b.doRequest(ctx, http.MethodPut, b.url+"bringlistitemdetails/"+itemUUID+"/image", b.putHeaders, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("cannot save item image %s: %w", itemUUID, err)
	}
	if err := decodeError(body); err != nil {
		return nil, fmt.Errorf("cannot save item image %s: %w", itemUUID, err)
	}

	var result map[string]string
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("cannot save item image %s: %w", itemUUID, err)
	}
	return result, nil
}

// CompleteItem marks an item as completed.
func (b *Bring) CompleteItem(ctx context.Context, listUUID, itemName, specification, itemUUID string) (string, error) {
	item := BatchUpdateItem{ItemID: itemName, Spec: specification, UUID: itemUUID}
	resp, err := b.BatchUpdateItems(ctx, listUUID, []BatchUpdateItem{item}, BringItemToRecently)
	if err != nil {
		return "", fmt.Errorf("cannot complete item %s from %s: %w", itemName, listUUID, err)
	}
	return resp, nil
}

// RemoveItem removes an item from a list.
func (b *Bring) RemoveItem(ctx context.Context, listUUID, itemName string) (string, error) {
	form := url.Values{}
	form.Set("purchase", "")
	form.Set("recently", "")
	form.Set("specification", "")
	form.Set("remove", itemName)
	form.Set("sender", "null")

	body, _, err := b.doRequest(ctx, http.MethodPut, b.url+"bringlists/"+listUUID, b.putHeaders, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("cannot remove item %s from %s: %w", itemName, listUUID, err)
	}
	if err := decodeError(body); err != nil {
		return "", fmt.Errorf("cannot remove item %s from %s: %w", itemName, listUUID, err)
	}
	return string(body), nil
}

// BatchUpdateItems updates items on a list.
func (b *Bring) BatchUpdateItems(ctx context.Context, listUUID string, items []BatchUpdateItem, operation BringItemOperation) (string, error) {
	type change struct {
		Accuracy  string                 `json:"accuracy"`
		Altitude  string                 `json:"altitude"`
		Latitude  string                 `json:"latitude"`
		Longitude string                 `json:"longitude"`
		ItemID    string                 `json:"itemId"`
		Spec      string                 `json:"spec,omitempty"`
		UUID      string                 `json:"uuid,omitempty"`
		Operation BringItemOperation     `json:"operation,omitempty"`
		Attribute map[string]interface{} `json:"attribute,omitempty"`
	}

	changes := make([]change, 0, len(items))
	for _, item := range items {
		op := item.Operation
		if op == "" {
			op = operation
		}
		changes = append(changes, change{
			Accuracy:  "0.0",
			Altitude:  "0.0",
			Latitude:  "0.0",
			Longitude: "0.0",
			ItemID:    item.ItemID,
			Spec:      item.Spec,
			UUID:      item.UUID,
			Operation: op,
			Attribute: item.Attribute,
		})
	}

	payload := map[string]interface{}{
		"changes": changes,
		"sender":  "",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	headers := cloneHeaders(b.headers)
	headers["Content-Type"] = "application/json"

	body, _, err := b.doRequest(ctx, http.MethodPut, b.url+"bringlists/"+listUUID+"/items", headers, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("cannot batch update items for list %s: %w", listUUID, err)
	}
	if err := decodeError(body); err != nil {
		return "", fmt.Errorf("cannot batch update items for list %s: %w", listUUID, err)
	}
	return string(body), nil
}

// RemoveItemImage removes an image from an item.
func (b *Bring) RemoveItemImage(ctx context.Context, itemUUID string) (string, error) {
	body, _, err := b.doRequest(ctx, http.MethodDelete, b.url+"bringlistitemdetails/"+itemUUID+"/image", b.headers, nil)
	if err != nil {
		return "", fmt.Errorf("cannot remove item image %s: %w", itemUUID, err)
	}
	if err := decodeError(body); err != nil {
		return "", fmt.Errorf("cannot remove item image %s: %w", itemUUID, err)
	}
	return string(body), nil
}

// MoveToRecentList moves an item to the recent list.
func (b *Bring) MoveToRecentList(ctx context.Context, listUUID, itemName string) (string, error) {
	form := url.Values{}
	form.Set("purchase", "")
	form.Set("recently", itemName)
	form.Set("specification", "")
	form.Set("remove", "")
	form.Set("sender", "null")

	body, _, err := b.doRequest(ctx, http.MethodPut, b.url+"bringlists/"+listUUID, b.putHeaders, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("cannot remove item %s from %s: %w", itemName, listUUID, err)
	}
	if err := decodeError(body); err != nil {
		return "", fmt.Errorf("cannot remove item %s from %s: %w", itemName, listUUID, err)
	}
	return string(body), nil
}

// SetListArticleLanguage sets list article language.
func (b *Bring) SetListArticleLanguage(ctx context.Context, listUUID, language string) (string, error) {
	form := url.Values{}
	form.Set("value", language)

	body, _, err := b.doRequest(ctx, http.MethodPost, b.url+"bringusersettings/"+b.uuid+"/"+listUUID+"/listArticleLanguage", b.putHeaders, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("cannot set list article language for %s: %w", listUUID, err)
	}
	if err := decodeError(body); err != nil {
		return "", fmt.Errorf("cannot set list article language for %s: %w", listUUID, err)
	}
	return string(body), nil
}

// GetActivity gets activity for a shopping list.
func (b *Bring) GetActivity(ctx context.Context, listUUID string) (GetActivityResponse, error) {
	var activity GetActivityResponse
	body, _, err := b.doRequest(ctx, http.MethodGet, b.url+"bringlists/"+listUUID+"/activity", b.headers, nil)
	if err != nil {
		return activity, fmt.Errorf("cannot get activity for list %s: %w", listUUID, err)
	}
	if err := decodeJSON(body, &activity); err != nil {
		return activity, fmt.Errorf("cannot get activity for list %s: %w", listUUID, err)
	}
	return activity, nil
}

// GetAllUsersFromList gets all users from a list.
func (b *Bring) GetAllUsersFromList(ctx context.Context, listUUID string) (GetAllUsersFromListResponse, error) {
	var users GetAllUsersFromListResponse
	body, _, err := b.doRequest(ctx, http.MethodGet, b.url+"bringlists/"+listUUID+"/users", b.headers, nil)
	if err != nil {
		return users, fmt.Errorf("cannot get users from list: %w", err)
	}
	if err := decodeJSON(body, &users); err != nil {
		return users, fmt.Errorf("cannot get users from list: %w", err)
	}
	return users, nil
}

// GetInspirations gets inspirations/recipes.
func (b *Bring) GetInspirations(ctx context.Context, filter string) (GetInspirationsResponse, error) {
	if filter == "" {
		filter = "mine"
	}
	params := url.Values{}
	params.Set("filterTags", filter)
	params.Set("offset", "0")
	params.Set("limit", "2147483647")

	var inspirations GetInspirationsResponse
	body, _, err := b.doRequest(ctx, http.MethodGet, b.url+"bringusers/"+b.uuid+"/inspirations?"+params.Encode(), b.headers, nil)
	if err != nil {
		return inspirations, fmt.Errorf("cannot get inspirations: %w", err)
	}
	if err := decodeJSON(body, &inspirations); err != nil {
		return inspirations, fmt.Errorf("cannot get inspirations: %w", err)
	}
	return inspirations, nil
}

// GetInspirationDetails gets detailed recipe/inspiration content.
func (b *Bring) GetInspirationDetails(ctx context.Context, contentUUID string) (map[string]interface{}, error) {
	var content map[string]interface{}
	body, _, err := b.doRequest(ctx, http.MethodGet, b.url+"bringtemplates/content/"+contentUUID, b.headers, nil)
	if err != nil {
		return content, fmt.Errorf("cannot get inspiration details: %w", err)
	}
	if err := decodeJSON(body, &content); err != nil {
		return content, fmt.Errorf("cannot get inspiration details: %w", err)
	}
	return content, nil
}

// GetInspirationFilters gets available inspiration filters.
func (b *Bring) GetInspirationFilters(ctx context.Context) (GetInspirationFiltersResponse, error) {
	var filters GetInspirationFiltersResponse
	body, _, err := b.doRequest(ctx, http.MethodGet, b.url+"bringusers/"+b.uuid+"/inspirationstreamfilters", b.headers, nil)
	if err != nil {
		return filters, fmt.Errorf("cannot get inspiration filters: %w", err)
	}
	if err := decodeJSON(body, &filters); err != nil {
		return filters, fmt.Errorf("cannot get inspiration filters: %w", err)
	}
	return filters, nil
}

// GetUserSettings gets the user settings.
func (b *Bring) GetUserSettings(ctx context.Context) (GetUserSettingsResponse, error) {
	var settings GetUserSettingsResponse
	body, _, err := b.doRequest(ctx, http.MethodGet, b.url+"bringusersettings/"+b.uuid, b.headers, nil)
	if err != nil {
		return settings, fmt.Errorf("cannot get user settings: %w", err)
	}
	if err := decodeJSON(body, &settings); err != nil {
		return settings, fmt.Errorf("cannot get user settings: %w", err)
	}
	return settings, nil
}

// LoadTranslations loads translation file by locale.
func (b *Bring) LoadTranslations(ctx context.Context, locale string) (map[string]string, error) {
	webBase := webBaseURL()
	resp, err := b.client.Get(webBase + "/locale/articles." + locale + ".json")
	if err != nil {
		return nil, fmt.Errorf("cannot get translations: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cannot get translations: %w", err)
	}
	if err := decodeError(body); err != nil {
		return nil, fmt.Errorf("cannot get translations: %w", err)
	}
	var translations map[string]string
	if err := json.Unmarshal(body, &translations); err != nil {
		return nil, fmt.Errorf("cannot get translations: %w", err)
	}
	return translations, nil
}

// LoadCatalog loads catalog file by locale.
func (b *Bring) LoadCatalog(ctx context.Context, locale string) (LoadCatalogResponse, error) {
	var catalog LoadCatalogResponse
	webBase := webBaseURL()
	resp, err := b.client.Get(webBase + "/locale/catalog." + locale + ".json")
	if err != nil {
		return catalog, fmt.Errorf("cannot get catalog: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return catalog, fmt.Errorf("cannot get catalog: %w", err)
	}
	if err := decodeError(body); err != nil {
		return catalog, fmt.Errorf("cannot get catalog: %w", err)
	}
	if err := json.Unmarshal(body, &catalog); err != nil {
		return catalog, fmt.Errorf("cannot get catalog: %w", err)
	}
	return catalog, nil
}

// GetPendingInvitations gets pending invitations.
func (b *Bring) GetPendingInvitations(ctx context.Context) (GetPendingInvitationsResponse, error) {
	var invites GetPendingInvitationsResponse
	body, _, err := b.doRequest(ctx, http.MethodGet, b.url+"bringusers/"+b.uuid+"/invitations?status=pending", b.headers, nil)
	if err != nil {
		return invites, fmt.Errorf("cannot get pending invitations: %w", err)
	}
	if err := decodeJSON(body, &invites); err != nil {
		return invites, fmt.Errorf("cannot get pending invitations: %w", err)
	}
	return invites, nil
}

// Notify sends a notification to list members.
func (b *Bring) Notify(ctx context.Context, listUUID string, notificationType BringNotificationType, itemName string, activity interface{}, receiver string, activityType ActivityType, reaction ReactionType) (string, error) {
	allowed := map[BringNotificationType]bool{
		NotifyGoingShopping: true,
		NotifyChangedList:   true,
		NotifyShoppingDone:  true,
		NotifyUrgentMessage: true,
		NotifyListReaction:  true,
	}
	if !allowed[notificationType] {
		return "", fmt.Errorf("notificationType %s not supported", notificationType)
	}

	payload := map[string]interface{}{
		"arguments":            []string{},
		"listNotificationType": string(notificationType),
		"senderPublicUserUuid": b.PublicUUID,
	}

	if notificationType == NotifyUrgentMessage {
		if itemName == "" {
			return "", errors.New("notificationType is URGENT_MESSAGE but itemName is missing")
		}
		payload["arguments"] = []string{itemName}
	}

	if notificationType == NotifyListReaction {
		switch v := activity.(type) {
		case Activity:
			if v.Content.PublicUserUUID == "" || v.Content.UUID == "" || reaction == "" {
				return "", errors.New("notificationType is LIST_ACTIVITY_STREAM_REACTION but a parameter is missing")
			}
			payload["receiverPublicUserUuid"] = v.Content.PublicUserUUID
			payload["listActivityStreamReaction"] = map[string]interface{}{
				"moduleUuid":   v.Content.UUID,
				"moduleType":   v.Type,
				"reactionType": string(reaction),
			}
		case *Activity:
			if v == nil || v.Content.PublicUserUUID == "" || v.Content.UUID == "" || reaction == "" {
				return "", errors.New("notificationType is LIST_ACTIVITY_STREAM_REACTION but a parameter is missing")
			}
			payload["receiverPublicUserUuid"] = v.Content.PublicUserUUID
			payload["listActivityStreamReaction"] = map[string]interface{}{
				"moduleUuid":   v.Content.UUID,
				"moduleType":   v.Type,
				"reactionType": string(reaction),
			}
		case string:
			if v == "" || receiver == "" || activityType == "" || reaction == "" {
				return "", errors.New("notificationType is LIST_ACTIVITY_STREAM_REACTION but a parameter is missing")
			}
			payload["receiverPublicUserUuid"] = receiver
			payload["listActivityStreamReaction"] = map[string]interface{}{
				"moduleUuid":   v,
				"moduleType":   string(activityType),
				"reactionType": string(reaction),
			}
		default:
			return "", errors.New("notificationType is LIST_ACTIVITY_STREAM_REACTION but a parameter is missing")
		}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	headers := cloneHeaders(b.headers)
	headers["Content-Type"] = "application/json"

	body, _, err := b.doRequest(ctx, http.MethodPost, b.url+"bringnotifications/lists/"+listUUID, headers, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("cannot send notification for list %s: %w", listUUID, err)
	}
	if err := decodeError(body); err != nil {
		return "", fmt.Errorf("cannot send notification for list %s: %w", listUUID, err)
	}
	return string(body), nil
}

func (b *Bring) setAuthHeaders(userUUID, accessToken, publicUUID string) {
	b.uuid = userUUID
	b.bearerToken = accessToken
	b.PublicUUID = publicUUID
	b.headers["X-BRING-USER-UUID"] = userUUID
	b.headers["Authorization"] = "Bearer " + accessToken
	if publicUUID != "" {
		b.headers["X-BRING-PUBLIC-USER-UUID"] = publicUUID
	}

	b.putHeaders = cloneHeaders(b.headers)
	b.putHeaders["Content-Type"] = "application/x-www-form-urlencoded; charset=UTF-8"
}

func (b *Bring) doRequest(ctx context.Context, method, url string, headers map[string]string, body io.Reader) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, 0, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	if resp.StatusCode >= 400 {
		if err := decodeError(data); err != nil {
			return data, resp.StatusCode, err
		}
		return data, resp.StatusCode, fmt.Errorf("http %d", resp.StatusCode)
	}

	return data, resp.StatusCode, nil
}

func decodeJSON(body []byte, out interface{}) error {
	if err := decodeError(body); err != nil {
		return err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	return json.Unmarshal(body, out)
}

func decodeError(body []byte) error {
	var errResp ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return nil
	}
	if errResp.Error == "" {
		return nil
	}
	if errResp.Message != "" {
		return errors.New(errResp.Message)
	}
	if errResp.ErrorDescription != "" {
		return errors.New(errResp.ErrorDescription)
	}
	return errors.New(errResp.Error)
}

func normalizeBaseURL(base string) string {
	if base == "" {
		return defaultBaseURL
	}
	if strings.HasSuffix(base, "/") {
		return base
	}
	return base + "/"
}

func cloneHeaders(headers map[string]string) map[string]string {
	clone := make(map[string]string, len(headers))
	for key, value := range headers {
		clone[key] = value
	}
	return clone
}

func webBaseURL() string {
	if base := os.Getenv("BRINGS_WEB_BASE_URL"); base != "" {
		return strings.TrimRight(base, "/")
	}
	return "https://web.getbring.com"
}

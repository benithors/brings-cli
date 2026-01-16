package bring

import "encoding/json"

type BringOptions struct {
	Mail     string
	Password string
	URL      string
	UUID     string
}

type TokenAuthOptions struct {
	AccessToken    string
	UserUUID       string
	PublicUserUUID string
	URL            string
}

type GetItemsResponseEntry struct {
	Specification string `json:"specification"`
	Name          string `json:"name"`
}

type GetItemsResponse struct {
	UUID     string                  `json:"uuid"`
	Status   string                  `json:"status"`
	Purchase []GetItemsResponseEntry `json:"purchase"`
	Recently []GetItemsResponseEntry `json:"recently"`
}

type AuthSuccessResponse struct {
	Name         string `json:"name"`
	UUID         string `json:"uuid"`
	PublicUUID   string `json:"publicUuid"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type ErrorResponse struct {
	Message          string `json:"message"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	ErrorCode        int    `json:"errorcode"`
}

type GetAllUsersFromListEntry struct {
	PublicUUID  string `json:"publicUuid"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	PhotoPath   string `json:"photoPath"`
	PushEnabled bool   `json:"pushEnabled"`
	PlusTryOut  bool   `json:"plusTryOut"`
	Country     string `json:"country"`
	Language    string `json:"language"`
}

type GetAllUsersFromListResponse struct {
	Users []GetAllUsersFromListEntry `json:"users"`
}

type LoadListsEntry struct {
	ListUUID string `json:"listUuid"`
	Name     string `json:"name"`
	Theme    string `json:"theme"`
}

type LoadListsResponse struct {
	Lists []LoadListsEntry `json:"lists"`
}

type GetItemsDetailsEntry struct {
	UUID           string `json:"uuid"`
	ItemID         string `json:"itemId"`
	ListUUID       string `json:"listUuid"`
	UserIconItemID string `json:"userIconItemId"`
	UserSectionID  string `json:"userSectionId"`
	AssignedTo     string `json:"assignedTo"`
	ImageURL       string `json:"imageUrl"`
}

type UserSettingsEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type UserListSettingsEntry struct {
	ListUUID     string              `json:"listUuid"`
	UserSettings []UserSettingsEntry `json:"usersettings"`
}

type GetUserSettingsResponse struct {
	UserSettings     []UserSettingsEntry     `json:"userSettings"`
	UserListSettings []UserListSettingsEntry `json:"userlistsettings"`
}

type GetUserAccountResponse struct {
	Email                string          `json:"email"`
	EmailVerified        bool            `json:"emailVerified"`
	PremiumConfiguration map[string]bool `json:"premiumConfiguration"`
	PublicUserUUID       string          `json:"publicUserUuid"`
	UserLocale           LocaleValue     `json:"userLocale"`
	UserUUID             string          `json:"userUuid"`
	Name                 string          `json:"name"`
	PhotoPath            string          `json:"photoPath"`
}

// LocaleValue tolerates string or object values from the API.
type LocaleValue struct {
	Raw   json.RawMessage
	Value string
}

func (l *LocaleValue) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		l.Raw = nil
		l.Value = ""
		return nil
	}
	l.Raw = append(l.Raw[:0], b...)

	var str string
	if err := json.Unmarshal(b, &str); err == nil {
		l.Value = str
		return nil
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(b, &obj); err == nil {
		if v, ok := obj["locale"].(string); ok {
			l.Value = v
			return nil
		}
		if v, ok := obj["language"].(string); ok {
			if c, ok := obj["country"].(string); ok && c != "" {
				l.Value = v + "-" + c
			} else {
				l.Value = v
			}
			return nil
		}
	}

	l.Value = ""
	return nil
}

func (l LocaleValue) String() string {
	if l.Value != "" {
		return l.Value
	}
	if len(l.Raw) > 0 {
		return string(l.Raw)
	}
	return ""
}

type GetActivityResponse struct {
	Timeline    []map[string]interface{} `json:"timeline"`
	Timestamp   string                   `json:"timestamp"`
	TotalEvents int                      `json:"totalEvents"`
}

type GetInspirationsResponse struct {
	Entries []map[string]interface{} `json:"entries"`
	Count   int                      `json:"count"`
	Total   int                      `json:"total"`
}

type GetInspirationFiltersResponse struct {
	Filters []map[string]interface{} `json:"filters"`
}

type CatalogItemsEntry struct {
	ItemID string `json:"itemId"`
	Name   string `json:"name"`
}

type CatalogSectionsEntry struct {
	SectionID string              `json:"sectionId"`
	Name      string              `json:"name"`
	Items     []CatalogItemsEntry `json:"items"`
}

type LoadCatalogResponse struct {
	Language string `json:"language"`
	Catalog  struct {
		Sections []CatalogSectionsEntry `json:"sections"`
	} `json:"catalog"`
}

type GetPendingInvitationsResponse struct {
	Invitations []map[string]interface{} `json:"invitations"`
}

type BringItemOperation string

const (
	BringItemToPurchase BringItemOperation = "TO_PURCHASE"
	BringItemToRecently BringItemOperation = "TO_RECENTLY"
	BringItemRemove     BringItemOperation = "REMOVE"
	BringItemAttrUpdate BringItemOperation = "ATTRIBUTE_UPDATE"
)

type BatchUpdateItem struct {
	ItemID    string                 `json:"itemId"`
	Spec      string                 `json:"spec,omitempty"`
	UUID      string                 `json:"uuid,omitempty"`
	Operation BringItemOperation     `json:"operation,omitempty"`
	Attribute map[string]interface{} `json:"attribute,omitempty"`
}

type BringNotificationType string

const (
	NotifyGoingShopping BringNotificationType = "GOING_SHOPPING"
	NotifyChangedList   BringNotificationType = "CHANGED_LIST"
	NotifyShoppingDone  BringNotificationType = "SHOPPING_DONE"
	NotifyUrgentMessage BringNotificationType = "URGENT_MESSAGE"
	NotifyListReaction  BringNotificationType = "LIST_ACTIVITY_STREAM_REACTION"
)

type ActivityType string

const (
	ActivityItemsChanged ActivityType = "LIST_ITEMS_CHANGED"
	ActivityItemsAdded   ActivityType = "LIST_ITEMS_ADDED"
	ActivityItemsRemoved ActivityType = "LIST_ITEMS_REMOVED"
)

type ReactionType string

const (
	ReactionThumbsUp ReactionType = "THUMBS_UP"
	ReactionMonocle  ReactionType = "MONOCLE"
	ReactionDrooling ReactionType = "DROOLING"
	ReactionHeart    ReactionType = "HEART"
)

type Activity struct {
	Type    string `json:"type"`
	Content struct {
		UUID           string `json:"uuid"`
		PublicUserUUID string `json:"publicUserUuid"`
	} `json:"content"`
}

type Image struct {
	ImageData string `json:"imageData"`
}

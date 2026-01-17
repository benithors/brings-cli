package cli

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/benithors/brings-cli/bring"
)

const bringWebURL = "https://web.getbring.com/app"

// Run executes the CLI and returns an exit code.
func Run(args []string) int {
	command, flags, positional := parseArgs(args)

	if flags.Has("help") || flags.Has("h") || command == "help" {
		showHelp()
		return 0
	}

	switch command {
	case "login":
		return loginCommand(flags)
	case "logout":
		return logoutCommand()
	case "status":
		return statusCommand()
	case "lists":
		return listsCommand()
	case "items":
		return itemsCommand(positional, flags)
	case "add":
		return addCommand(positional, flags)
	case "remove", "rm":
		return removeCommand(positional, flags)
	case "complete", "done":
		return completeCommand(positional, flags)
	case "users":
		return usersCommand(flags)
	case "notify":
		return notifyCommand(positional, flags)
	case "activity":
		return activityCommand(flags)
	case "account":
		return accountCommand()
	case "settings":
		return settingsCommand()
	case "config":
		return configCommand(positional)
	case "inspirations":
		return inspirationsCommand(positional, flags)
	case "recipe":
		return recipeCommand(positional, flags)
	case "add-recipe":
		return addRecipeCommand(positional, flags)
	case "catalog":
		return catalogCommand(positional)
	case "":
		showHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		fmt.Fprintln(os.Stderr, "Run `brings --help` for usage")
		return 1
	}
}

type FlagSet struct {
	Values map[string]string
	Bools  map[string]bool
}

func (f FlagSet) Has(name string) bool {
	return f.Bools[name] || f.Values[name] != ""
}

func (f FlagSet) Get(name string) string {
	return f.Values[name]
}

type inspirationOutput struct {
	ID       string `json:"id,omitempty"`
	Title    string `json:"title,omitempty"`
	ImageURL string `json:"imageUrl,omitempty"`
}

type inspirationsOutput struct {
	Filter  string              `json:"filter,omitempty"`
	Count   int                 `json:"count"`
	Total   int                 `json:"total,omitempty"`
	Entries []inspirationOutput `json:"entries"`
}

type inspirationFilterOutput struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

type inspirationFiltersOutput struct {
	Filters []inspirationFilterOutput `json:"filters"`
}

type recipeIngredientOutput struct {
	Name   string `json:"name,omitempty"`
	Spec   string `json:"spec,omitempty"`
	Pantry bool   `json:"pantry,omitempty"`
}

type recipeOutput struct {
	ID        string            `json:"id"`
	Title     string            `json:"title,omitempty"`
	ImageURL  string            `json:"imageUrl,omitempty"`
	Nutrition map[string]string `json:"nutrition,omitempty"`
}

func parseArgs(args []string) (string, FlagSet, []string) {
	flags := FlagSet{Values: map[string]string{}, Bools: map[string]bool{}}
	positional := []string{}
	command := ""

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if i == 0 && !strings.HasPrefix(arg, "-") {
			command = arg
			continue
		}

		if strings.HasPrefix(arg, "--") {
			keyValue := strings.SplitN(arg[2:], "=", 2)
			key := keyValue[0]
			if len(keyValue) == 2 {
				flags.Values[key] = keyValue[1]
				continue
			}
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flags.Values[key] = args[i+1]
				i++
				continue
			}
			flags.Bools[key] = true
			continue
		}

		if strings.HasPrefix(arg, "-") {
			flags.Bools[arg[1:]] = true
			continue
		}

		positional = append(positional, arg)
	}

	return command, flags, positional
}

func prompt(question string) (string, error) {
	fmt.Print(question)
	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

type jwtClaims struct {
	Sub   string
	Email string
	Exp   int64
}

func decodeJWT(token string) (jwtClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return jwtClaims{}, fmt.Errorf("invalid token format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return jwtClaims{}, err
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return jwtClaims{}, err
	}

	claims := jwtClaims{}
	if sub, ok := raw["sub"].(string); ok {
		claims.Sub = sub
	}
	if email, ok := raw["email"].(string); ok {
		claims.Email = email
	}
	if expVal, ok := raw["exp"]; ok {
		claims.Exp = int64(toFloat(expVal))
	}
	return claims, nil
}

func loginCommand(flags FlagSet) int {
	baseURL := getBaseURL()
	if flags.Has("browser") || flags.Has("b") {
		result, err := BrowserLoginWithIntercept(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nError: Browser login failed - %s\n", err)
			return 1
		}

		fmt.Println("Validating token...")
		client := bring.FromToken(bring.TokenAuthOptions{
			AccessToken:    result.AccessToken,
			UserUUID:       result.UserUUID,
			PublicUserUUID: result.PublicUserUUID,
			URL:            baseURL,
		})
		account, err := client.GetUserAccount(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nError: Failed to validate token - %s\n", err)
			return 1
		}

		cfg := Config{
			AccessToken:    result.AccessToken,
			UserUUID:       account.UserUUID,
			PublicUserUUID: account.PublicUserUUID,
			UserName:       coalesce(account.Name, result.UserName),
			Email:          coalesce(account.Email, result.Email),
		}
		if err := saveConfig(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %s\n", err)
			return 1
		}

		fmt.Printf("\nLogged in as %s\n", coalesce(account.Name, account.Email))
		fmt.Printf("Config saved to %s\n", getConfigPath())
		return 0
	}

	token := flags.Get("token")
	if token == "" {
		fmt.Println()
		fmt.Println("To login, you need to extract your access token from the Bring! web app.")
		fmt.Println()
		fmt.Println("Steps:")
		fmt.Printf("  1. Open %s in your browser\n", bringWebURL)
		fmt.Println("  2. Log in with your credentials")
		fmt.Println("  3. Open DevTools (F12) -> Application tab -> Local Storage")
		fmt.Println("  4. Find the \"accessToken\" key and copy its value")
		fmt.Println()
		fmt.Println("Or use `brings login --browser` for automatic browser-based login.")
		fmt.Println()

		entered, err := prompt("Paste your access token: ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
			return 1
		}
		token = entered
	}

	if token == "" {
		fmt.Fprintln(os.Stderr, "Error: No token provided")
		return 1
	}

	decoded, err := decodeJWT(token)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: Invalid token format (not a valid JWT)")
		return 1
	}
	if decoded.Sub == "" {
		fmt.Fprintln(os.Stderr, "Error: Token missing user identifier (sub claim)")
		return 1
	}
	if decoded.Exp > 0 && time.Unix(decoded.Exp, 0).Before(time.Now()) {
		fmt.Fprintln(os.Stderr, "Error: Token has expired. Please get a fresh token from the web app.")
		return 1
	}

	parts := strings.Split(decoded.Sub, ":")
	userUUID := parts[len(parts)-1]

	fmt.Println("\nValidating token...")
	client := bring.FromToken(bring.TokenAuthOptions{AccessToken: token, UserUUID: userUUID, URL: baseURL})
	account, err := client.GetUserAccount(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError: Failed to validate token - %s\n", err)
		fmt.Fprintln(os.Stderr, "The token may be invalid or expired. Please try again with a fresh token.")
		return 1
	}

	cfg := Config{
		AccessToken:    token,
		UserUUID:       account.UserUUID,
		PublicUserUUID: account.PublicUserUUID,
		UserName:       account.Name,
		Email:          account.Email,
	}
	if err := saveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %s\n", err)
		return 1
	}

	fmt.Printf("\nLogged in as %s\n", coalesce(account.Name, account.Email))
	fmt.Printf("Config saved to %s\n", getConfigPath())
	return 0
}

func logoutCommand() int {
	if !isLoggedIn() {
		fmt.Println("Not logged in")
		return 0
	}
	if err := clearConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	fmt.Println("Logged out successfully")
	return 0
}

func statusCommand() int {
	cfg := loadConfig()
	if cfg.AccessToken == "" {
		fmt.Println("Not logged in")
		fmt.Println("\nRun `brings login` to authenticate")
		return 0
	}

	fmt.Println("Logged in")
	if cfg.UserName != "" {
		fmt.Printf("  Name: %s\n", cfg.UserName)
	}
	if cfg.Email != "" {
		fmt.Printf("  Email: %s\n", cfg.Email)
	}
	fmt.Printf("  Config: %s\n", getConfigPath())

	decoded, err := decodeJWT(cfg.AccessToken)
	if err == nil && decoded.Exp > 0 {
		exp := time.Unix(decoded.Exp, 0)
		if exp.Before(time.Now()) {
			fmt.Println("\n  Warning: Token has expired! Run `brings login` to refresh.")
		} else {
			daysLeft := int(math.Ceil(exp.Sub(time.Now()).Hours() / 24))
			fmt.Printf("  Token expires: %s (%d days)\n", exp.Format("2006-01-02"), daysLeft)
		}
	}
	return 0
}

func listsCommand() int {
	client, _, ok := getBringClient()
	if !ok {
		return 1
	}
	lists, err := client.LoadLists(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	fmt.Println("Shopping Lists:")
	fmt.Println()
	for _, list := range lists.Lists {
		fmt.Printf("  %s (%s)\n", list.Name, list.ListUUID)
	}
	return 0
}

func itemsCommand(positional []string, flags FlagSet) int {
	client, _, ok := getBringClient()
	if !ok {
		return 1
	}
	listUUID, listName, err := getListUUID(client, flags.Get("list"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	if flags.Get("list") == "" {
		fmt.Printf("List: %s\n\n", listName)
	}

	items, err := client.GetItems(context.Background(), listUUID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}

	if len(items.Purchase) == 0 && len(items.Recently) == 0 {
		fmt.Println("Shopping list is empty")
		return 0
	}

	if len(items.Purchase) > 0 {
		fmt.Println("To Purchase:")
		for _, item := range items.Purchase {
			spec := ""
			if item.Specification != "" {
				spec = fmt.Sprintf(" (%s)", item.Specification)
			}
			fmt.Printf("  - %s%s\n", item.Name, spec)
		}
	}

	if flags.Has("all") && len(items.Recently) > 0 {
		fmt.Println("\nRecent Items:")
		for _, item := range items.Recently {
			fmt.Printf("  - %s\n", item.Name)
		}
	}
	_ = positional
	return 0
}

func addCommand(positional []string, flags FlagSet) int {
	client, _, ok := getBringClient()
	if !ok {
		return 1
	}
	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: brings add <item> [--spec \"specification\"] [--list <uuid>]")
		return 1
	}
	itemName := positional[0]
	spec := flags.Get("spec")

	listUUID, listName, err := getListUUID(client, flags.Get("list"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	if _, err := client.SaveItem(context.Background(), listUUID, itemName, spec); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	if spec != "" {
		fmt.Printf("Added \"%s\" (%s) to %s\n", itemName, spec, listName)
	} else {
		fmt.Printf("Added \"%s\" to %s\n", itemName, listName)
	}
	return 0
}

func removeCommand(positional []string, flags FlagSet) int {
	client, _, ok := getBringClient()
	if !ok {
		return 1
	}
	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: brings remove <item> [--list <uuid>]")
		return 1
	}
	itemName := positional[0]
	listUUID, listName, err := getListUUID(client, flags.Get("list"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	if _, err := client.RemoveItem(context.Background(), listUUID, itemName); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	fmt.Printf("Removed \"%s\" from %s\n", itemName, listName)
	return 0
}

func completeCommand(positional []string, flags FlagSet) int {
	client, _, ok := getBringClient()
	if !ok {
		return 1
	}
	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: brings complete <item> [--list <uuid>]")
		return 1
	}
	itemName := positional[0]
	listUUID, listName, err := getListUUID(client, flags.Get("list"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	if _, err := client.MoveToRecentList(context.Background(), listUUID, itemName); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	fmt.Printf("Completed \"%s\" in %s\n", itemName, listName)
	return 0
}

func activityCommand(flags FlagSet) int {
	client, _, ok := getBringClient()
	if !ok {
		return 1
	}
	listUUID, listName, err := getListUUID(client, flags.Get("list"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	fmt.Printf("Activity for: %s\n\n", listName)

	activity, err := client.GetActivity(context.Background(), listUUID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}

	if len(activity.Timeline) == 0 {
		fmt.Println("No recent activity")
		return 0
	}

	for i, event := range activity.Timeline {
		if i >= 10 {
			break
		}
		ts := toString(event["timestamp"])
		if ts == "" {
			ts = toString(event["date"])
		}
		date := ts
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			date = parsed.Local().Format(time.RFC1123)
		}
		etype := coalesce(toString(event["type"]), toString(event["action"]))
		content := ""
		if item, ok := event["content"].(map[string]interface{}); ok {
			content = coalesce(toString(item["itemId"]), toString(item["itemName"]))
		}
		if content == "" {
			content = coalesce(toString(event["itemId"]), toString(event["itemName"]))
		}
		fmt.Printf("  [%s] %s: %s\n", date, etype, content)
	}

	fmt.Printf("\nTotal events: %d\n", activity.TotalEvents)
	return 0
}

func usersCommand(flags FlagSet) int {
	client, _, ok := getBringClient()
	if !ok {
		return 1
	}
	listUUID, listName, err := getListUUID(client, flags.Get("list"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	fmt.Printf("Users in: %s\n\n", listName)

	users, err := client.GetAllUsersFromList(context.Background(), listUUID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	for _, user := range users.Users {
		fmt.Printf("  - %s (%s)\n", user.Name, user.Email)
	}
	return 0
}

func accountCommand() int {
	client, _, ok := getBringClient()
	if !ok {
		return 1
	}
	account, err := client.GetUserAccount(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}

	fmt.Println("Account Information:")
	fmt.Println()
	fmt.Printf("  Name: %s\n", coalesce(account.Name, "N/A"))
	fmt.Printf("  Email: %s\n", account.Email)
	if account.EmailVerified {
		fmt.Println("  Email Verified: Yes")
	} else {
		fmt.Println("  Email Verified: No")
	}
	locale := account.UserLocale.String()
	if locale == "" {
		locale = "N/A"
	}
	fmt.Printf("  Locale: %s\n", locale)
	fmt.Printf("  User UUID: %s\n", account.UserUUID)
	fmt.Printf("  Public UUID: %s\n", account.PublicUserUUID)
	return 0
}

func settingsCommand() int {
	client, _, ok := getBringClient()
	if !ok {
		return 1
	}
	settings, err := client.GetUserSettings(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}

	fmt.Println("User Settings:")
	fmt.Println()
	for _, setting := range settings.UserSettings {
		fmt.Printf("  %s: %s\n", setting.Key, setting.Value)
	}

	if len(settings.UserListSettings) > 0 {
		fmt.Println()
		fmt.Println("List Settings:")
		for _, listSetting := range settings.UserListSettings {
			fmt.Printf("\n  List: %s\n", listSetting.ListUUID)
			for _, s := range listSetting.UserSettings {
				fmt.Printf("    %s: %s\n", s.Key, s.Value)
			}
		}
	}
	return 0
}

func configCommand(positional []string) int {
	cfg := loadConfig()
	if len(positional) == 0 {
		fmt.Println("Configuration:")
		fmt.Println()
		if cfg.Servings == 0 {
			fmt.Println("  servings: (not set)")
		} else {
			fmt.Printf("  servings: %d\n", cfg.Servings)
		}
		if cfg.DefaultList == "" {
			fmt.Println("  defaultList: (not set)")
		} else {
			fmt.Printf("  defaultList: %s\n", cfg.DefaultList)
		}
		if cfg.Locale == "" {
			fmt.Println("  locale: (not set)")
		} else {
			fmt.Printf("  locale: %s\n", cfg.Locale)
		}
		fmt.Printf("\nConfig file: %s\n", getConfigPath())
		return 0
	}

	key := positional[0]
	if len(positional) == 1 {
		switch key {
		case "servings":
			if cfg.Servings == 0 {
				fmt.Println("servings: (not set)")
			} else {
				fmt.Printf("servings: %d\n", cfg.Servings)
			}
		case "defaultList":
			fmt.Printf("defaultList: %s\n", coalesce(cfg.DefaultList, "(not set)"))
		case "locale":
			fmt.Printf("locale: %s\n", coalesce(cfg.Locale, "(not set)"))
		default:
			fmt.Fprintf(os.Stderr, "Unknown config key: %s\n", key)
			fmt.Fprintln(os.Stderr, "Valid keys: servings, defaultList, locale")
			return 1
		}
		return 0
	}

	value := positional[1]
	if value == "" {
		fmt.Fprintf(os.Stderr, "Unknown config key: %s\n", key)
		return 1
	}

	switch key {
	case "servings":
		num, err := strconv.Atoi(value)
		if err != nil || num < 1 {
			fmt.Fprintln(os.Stderr, "servings must be a positive number")
			return 1
		}
		cfg.Servings = num
	case "defaultList":
		cfg.DefaultList = value
	case "locale":
		cfg.Locale = value
	default:
		fmt.Fprintf(os.Stderr, "Unknown config key: %s\n", key)
		return 1
	}

	if err := saveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %s\n", err)
		return 1
	}
	fmt.Printf("Set %s = %s\n", key, value)
	return 0
}

func catalogCommand(positional []string) int {
	client, _, ok := getBringClient()
	if !ok {
		return 1
	}
	locale := "en-US"
	if len(positional) > 0 {
		locale = positional[0]
	}

	catalog, err := client.LoadCatalog(context.Background(), locale)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}

	fmt.Printf("Catalog (%s):\n", catalog.Language)
	for _, section := range catalog.Catalog.Sections {
		fmt.Printf("\n%s:\n", section.Name)
		items := []string{}
		for i, item := range section.Items {
			if i >= 10 {
				break
			}
			items = append(items, item.Name)
		}
		if len(items) > 0 {
			fmt.Printf("  %s", strings.Join(items, ", "))
			if len(section.Items) > 10 {
				fmt.Print("...")
			}
			fmt.Println()
		}
	}
	return 0
}

func addRecipeCommand(positional []string, flags FlagSet) int {
	client, cfg, ok := getBringClient()
	if !ok {
		return 1
	}
	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: brings add-recipe <recipe-uuid> [--list <uuid>] [--servings <n>]")
		fmt.Fprintln(os.Stderr, "\nGet the recipe UUID from `brings inspirations`")
		return 1
	}
	contentUUID := positional[0]

	recipe, err := client.GetInspirationDetails(context.Background(), contentUUID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	title := coalesce(toString(recipe["title"]), toString(recipe["name"]), "Recipe")

	listUUID, listName, err := getListUUID(client, flags.Get("list"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}

	recipeServings := parseServings(recipe["yield"], recipe["baseQuantity"], recipe["servings"])
	targetServings := 0
	if flags.Get("servings") != "" {
		if v, err := strconv.Atoi(flags.Get("servings")); err == nil {
			targetServings = v
		}
	} else if cfg.Servings > 0 {
		targetServings = cfg.Servings
	}

	scale := 1.0
	if recipeServings > 0 && targetServings > 0 {
		scale = float64(targetServings) / float64(recipeServings)
	}

	items := toSlice(recipe["items"])
	if len(items) == 0 {
		items = toSlice(recipe["ingredients"])
	}
	if len(items) == 0 {
		fmt.Fprintln(os.Stderr, "Recipe has no ingredients")
		return 1
	}

	batchItems := []bring.BatchUpdateItem{}
	for _, item := range items {
		m := toMap(item)
		if !flags.Has("all") && toBool(m["stock"]) {
			continue
		}
		name := coalesce(toString(m["itemId"]), toString(m["name"]), "")
		if name == "" {
			continue
		}
		spec := toString(m["spec"])
		spec = scaleSpec(spec, scale)
		batchItems = append(batchItems, bring.BatchUpdateItem{ItemID: name, Spec: spec})
	}

	if len(batchItems) == 0 {
		fmt.Println("All ingredients are pantry items. Use --all to add them anyway.")
		return 0
	}

	if _, err := client.BatchUpdateItems(context.Background(), listUUID, batchItems, bring.BringItemToPurchase); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}

	fmt.Printf("\nAdded %d ingredients from \"%s\" to %s\n", len(batchItems), title, listName)
	if scale != 1 && recipeServings > 0 && targetServings > 0 {
		fmt.Printf("(Scaled from %d to %d servings)\n", recipeServings, targetServings)
	}

	fmt.Println("\nItems added:")
	for _, item := range batchItems {
		if item.Spec != "" {
			fmt.Printf("  - %s (%s)\n", item.ItemID, item.Spec)
		} else {
			fmt.Printf("  - %s\n", item.ItemID)
		}
	}

	if !flags.Has("all") && len(batchItems) < len(items) {
		skipped := len(items) - len(batchItems)
		fmt.Printf("\n%d pantry item(s) skipped. Use --all to include them.\n", skipped)
	}

	return 0
}

func recipeCommand(positional []string, flags FlagSet) int {
	client, cfg, ok := getBringClient()
	if !ok {
		return 1
	}
	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: brings recipe <content-uuid>")
		fmt.Fprintln(os.Stderr, "\nGet the UUID from `brings inspirations --verbose`")
		return 1
	}
	contentUUID := positional[0]

	recipe, err := client.GetInspirationDetails(context.Background(), contentUUID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	format, pretty, err := parseOutputFormat(flags, "json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	if flags.Has("debug") {
		printJSON(recipe, true)
		return 0
	}

	title := coalesce(toString(recipe["title"]), toString(recipe["name"]), "Recipe")
	author := coalesce(toString(recipe["author"]), toString(recipe["attribution"]))
	likes := toInt(recipe["likeCount"])

	recipeServings := parseServings(recipe["yield"], recipe["baseQuantity"], recipe["servings"])
	targetServings := 0
	if flags.Get("servings") != "" {
		if v, err := strconv.Atoi(flags.Get("servings")); err == nil {
			targetServings = v
		}
	} else if cfg.Servings > 0 {
		targetServings = cfg.Servings
	}

	scale := 1.0
	if recipeServings > 0 && targetServings > 0 {
		scale = float64(targetServings) / float64(recipeServings)
	}

	ingredients := recipeIngredients(recipe, scale)
	nutrition := recipeNutrition(recipe)
	instructions := recipeInstructions(recipe)

	if format != "human" {
		output := recipeOutput{
			ID:        contentUUID,
			Title:     title,
			ImageURL:  imageURLFromContent(recipe),
			Nutrition: nutrition,
		}
		printJSON(output, pretty)
		return 0
	}

	fmt.Printf("\n%s\n", title)
	fmt.Println(strings.Repeat("=", len(title)))

	if author != "" {
		fmt.Printf("Source: %s\n", author)
	}
	if likes > 0 {
		fmt.Printf("Likes: %d\n", likes)
	}
	if flags.Has("images") || flags.Has("image") {
		if image := imageURLFromContent(recipe); image != "" {
			fmt.Printf("Image: %s\n", image)
		}
	}

	if recipeServings > 0 {
		if scale != 1 && targetServings > 0 {
			fmt.Printf("Servings: %d -> scaled to %d\n", recipeServings, targetServings)
		} else {
			fmt.Printf("Servings: %d\n", recipeServings)
		}
	}

	if len(nutrition) > 0 {
		nutritionKeys := []string{}
		for key := range nutrition {
			nutritionKeys = append(nutritionKeys, key)
		}
		sort.Strings(nutritionKeys)
		fmt.Println("\nNutrition:")
		for _, key := range nutritionKeys {
			fmt.Printf("  %s: %s\n", key, nutrition[key])
		}
	}

	if len(ingredients) > 0 {
		fmt.Println("\nIngredients:")
		for _, item := range ingredients {
			stockNote := ""
			if item.Pantry {
				stockNote = " (pantry)"
			}
			if item.Spec != "" {
				fmt.Printf("  - %s %s%s\n", item.Spec, item.Name, stockNote)
			} else {
				fmt.Printf("  - %s%s\n", item.Name, stockNote)
			}
		}
	}

	if len(instructions) > 0 {
		fmt.Println()
		fmt.Println("Instructions:")
		for i, step := range instructions {
			fmt.Printf("  %d. %s\n", i+1, step)
		}
	}

	if link := toString(recipe["linkOutUrl"]); link != "" {
		fmt.Printf("\nSource: %s\n", link)
	}

	return 0
}

func inspirationsCommand(positional []string, flags FlagSet) int {
	client, _, ok := getBringClient()
	if !ok {
		return 1
	}

	format, pretty, err := parseOutputFormat(flags, "json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}

	filter := "mine"
	if len(positional) > 0 {
		filter = positional[0]
	}

	if flags.Has("filters") {
		filters, err := client.GetInspirationFilters(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
			return 1
		}
		if format == "human" {
			fmt.Println("Available Filters:")
			fmt.Println()
			seen := map[string]bool{}
			for _, filter := range filters.Filters {
				m := toMap(filter)
				tag := coalesce(toString(m["tag"]), toString(m["id"]))
				seen[tag] = true
				fmt.Printf("  - %s: %s\n", tag, coalesce(toString(m["name"]), toString(m["label"])))
			}
			if !seen["all"] {
				fmt.Println("  - all: All (global stream)")
			}
			return 0
		}

		filterEntries := []inspirationFilterOutput{}
		seen := map[string]bool{}
		for _, filter := range filters.Filters {
			m := toMap(filter)
			tag := coalesce(toString(m["tag"]), toString(m["id"]))
			if tag == "" {
				continue
			}
			seen[tag] = true
			filterEntries = append(filterEntries, inspirationFilterOutput{
				ID:   tag,
				Name: coalesce(toString(m["name"]), toString(m["label"])),
			})
		}
		if !seen["all"] {
			filterEntries = append(filterEntries, inspirationFilterOutput{
				ID:   "all",
				Name: "All (global stream)",
			})
		}
		printJSON(inspirationFiltersOutput{Filters: filterEntries}, pretty)
		return 0
	}

	inspirations, err := client.GetInspirations(context.Background(), filter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}

	if flags.Has("debug") {
		printJSON(inspirations, true)
		return 0
	}

	if format != "human" {
		limit := len(inspirations.Entries)
		if limit > 20 {
			limit = 20
		}
		entries := make([]inspirationOutput, 0, limit)
		for _, entry := range inspirations.Entries[:limit] {
			content := toMap(entry["content"])
			if len(content) == 0 {
				content = entry
			}
			item := inspirationOutput{
				ID:       coalesce(toString(content["contentUuid"]), toString(content["uuid"]), toString(entry["uuid"])),
				Title:    coalesce(toString(content["title"]), toString(content["name"]), toString(content["campaign"])),
				ImageURL: imageURLFromContent(content),
			}
			entries = append(entries, item)
		}
		printJSON(inspirationsOutput{
			Filter:  filter,
			Count:   len(entries),
			Total:   inspirations.Total,
			Entries: entries,
		}, pretty)
		return 0
	}

	fmt.Printf("Inspirations (%s):\n\n", filter)
	if len(inspirations.Entries) == 0 {
		fmt.Println("No inspirations found")
		return 0
	}

	limit := len(inspirations.Entries)
	if limit > 20 {
		limit = 20
	}

	for _, entry := range inspirations.Entries[:limit] {
		content := toMap(entry["content"])
		if len(content) == 0 {
			content = entry
		}
		title := coalesce(toString(content["title"]), toString(content["name"]), toString(content["campaign"]), "Untitled")
		author := coalesce(toString(content["author"]), toString(content["attribution"]))
		likes := ""
		if count := toInt(content["likeCount"]); count > 0 {
			likes = fmt.Sprintf("%d likes", count)
		}
		uuid := toString(content["contentUuid"])

		fmt.Printf("\n  %s\n", title)
		meta := []string{}
		if author != "" {
			meta = append(meta, author)
		}
		if likes != "" {
			meta = append(meta, likes)
		}
		if ctype := toString(content["type"]); ctype != "" {
			meta = append(meta, ctype)
		}
		if len(meta) > 0 {
			fmt.Printf("    %s\n", strings.Join(meta, " | "))
		}
		if uuid != "" {
			fmt.Printf("    ID: %s\n", uuid)
		}
		if flags.Has("images") || flags.Has("image") {
			if image := imageURLFromContent(content); image != "" {
				fmt.Printf("    Image: %s\n", image)
			}
		}
		if tags := toSlice(content["tags"]); len(tags) > 0 {
			relevant := []string{}
			for _, tag := range tags {
				value := toString(tag)
				if value == "" {
					continue
				}
				if value == "all" || value == "mine" || value == "type_recipe" || value == "bring_recipe_parser" {
					continue
				}
				relevant = append(relevant, value)
			}
			if len(relevant) > 0 {
				fmt.Printf("    Tags: %s\n", strings.Join(relevant, ", "))
			}
		}
		if flags.Has("verbose") {
			if link := toString(content["linkOutUrl"]); link != "" {
				fmt.Printf("    URL: %s\n", link)
			}
		}
	}

	fmt.Printf("\nShowing %d of %d total\n", limit, inspirations.Total)
	return 0
}

func notifyCommand(positional []string, flags FlagSet) int {
	client, _, ok := getBringClient()
	if !ok {
		return 1
	}
	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: brings notify <type> [--message \"msg\"] [--list <uuid>]")
		fmt.Fprintln(os.Stderr, "\nTypes: GOING_SHOPPING, CHANGED_LIST, SHOPPING_DONE, URGENT_MESSAGE")
		return 1
	}

	notifyType := strings.ToUpper(positional[0])
	valid := map[string]bool{
		"GOING_SHOPPING": true,
		"CHANGED_LIST":   true,
		"SHOPPING_DONE":  true,
		"URGENT_MESSAGE": true,
	}
	if !valid[notifyType] {
		fmt.Fprintln(os.Stderr, "Usage: brings notify <type> [--message \"msg\"] [--list <uuid>]")
		fmt.Fprintln(os.Stderr, "\nTypes: GOING_SHOPPING, CHANGED_LIST, SHOPPING_DONE, URGENT_MESSAGE")
		return 1
	}

	listUUID, listName, err := getListUUID(client, flags.Get("list"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}

	message := flags.Get("message")
	if _, err := client.Notify(context.Background(), listUUID, bring.BringNotificationType(notifyType), message, nil, "", "", ""); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	fmt.Printf("Notification \"%s\" sent to %s\n", notifyType, listName)
	return 0
}

func getBringClient() (*bring.Bring, Config, bool) {
	cfg := loadConfig()
	if cfg.AccessToken == "" || cfg.UserUUID == "" {
		fmt.Fprintln(os.Stderr, "Not logged in. Run `brings login` first.")
		return nil, cfg, false
	}

	client := bring.FromToken(bring.TokenAuthOptions{
		AccessToken:    cfg.AccessToken,
		UserUUID:       cfg.UserUUID,
		PublicUserUUID: cfg.PublicUserUUID,
		URL:            getBaseURL(),
	})
	return client, cfg, true
}

func getListUUID(client *bring.Bring, listArg string) (string, string, error) {
	if listArg != "" {
		return listArg, listArg, nil
	}
	lists, err := client.LoadLists(context.Background())
	if err != nil {
		return "", "", err
	}
	if len(lists.Lists) == 0 {
		return "", "", fmt.Errorf("no shopping lists found")
	}
	return lists.Lists[0].ListUUID, lists.Lists[0].Name, nil
}

func showHelp() {
	fmt.Print(`
brings - CLI for Bring! Shopping Lists

Usage: brings <command> [options]

Authentication:
  login --browser           Open browser for login (recommended)
  login --token <token>     Login with token directly
  logout                    Clear saved credentials
  status                    Show login status and token expiry

Shopping List:
  lists                     Show all shopping lists
  items [--list <uuid>]     Show items to purchase
    --all                     Include recent/completed items
  add <item> [--spec ".."]  Add item to list
  remove <item>             Remove item from list
  complete <item>           Mark item as purchased

Recipes (for AI agents):
  inspirations [filter]     List saved recipes with IDs and tags
    --filters                 Show available filter tags
    --format <mode>            Output format: json (default) | human | pretty
    JSON fields (default):     {id, title, imageUrl} (best for agents)
    --images                  Include image URLs
  recipe <id>               Show recipe details and ingredients
    --format <mode>            Output format: json (default) | human | pretty
    JSON fields (default):     {id, title, imageUrl, nutrition} (best for agents)
    --images                  Include image URLs
  add-recipe <id>           Add recipe ingredients to shopping list
    --servings <n>            Scale for n servings (default: config or recipe)
    --all                     Include pantry items (salt, pepper, etc.)

Social:
  users                     Show users sharing the list
  notify <type>             Send notification (GOING_SHOPPING, SHOPPING_DONE, etc.)
  activity                  Show recent list activity

Settings:
  account                   Show account information
  config                    Show current configuration
  config servings <n>       Set default servings for recipes
  config defaultList <uuid> Set default shopping list
  catalog [locale]          Browse item catalog

Agent Workflow:
  1. brings inspirations         -> List recipes with IDs
  2. brings add-recipe <id>      -> Add to shopping list (scaled to config servings)

  Optional: brings recipe <id>   -> Preview ingredients before adding

Examples:
  brings inspirations              List saved recipes
  brings add-recipe abc-123        Add recipe to cart
  brings add-recipe abc-123 --servings 6
  brings items                     Show current shopping list
`)
}

func getBaseURL() string {
	if base := os.Getenv("BRINGS_BASE_URL"); base != "" {
		return base
	}
	return ""
}

func coalesce(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func toString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

func toInt(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i)
		}
	}
	if s := toString(value); s != "" {
		if i, err := strconv.Atoi(s); err == nil {
			return i
		}
	}
	return 0
}

func toFloat(value interface{}) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f
		}
	}
	if s := toString(value); s != "" {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	}
	return 0
}

func toBool(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1"
	case float64:
		return v != 0
	case int:
		return v != 0
	default:
		return false
	}
}

func toMap(value interface{}) map[string]interface{} {
	if m, ok := value.(map[string]interface{}); ok {
		return m
	}
	return map[string]interface{}{}
}

func toSlice(value interface{}) []interface{} {
	if s, ok := value.([]interface{}); ok {
		return s
	}
	return []interface{}{}
}

func toStringSlice(values []interface{}) []string {
	out := []string{}
	for _, value := range values {
		if s := toString(value); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func parseOutputFormat(flags FlagSet, defaultFormat string) (string, bool, error) {
	format := strings.ToLower(flags.Get("format"))
	if format == "" {
		if flags.Has("format") {
			return "", false, errors.New("format requires a value: json | human | pretty")
		}
		format = defaultFormat
	}

	switch format {
	case "json":
		return "json", false, nil
	case "pretty", "pretty-json", "json-pretty":
		return "json", true, nil
	case "human", "text":
		return "human", false, nil
	default:
		return "", false, fmt.Errorf("unknown format: %s (use json | human | pretty)", format)
	}
}

func printJSON(value interface{}, pretty bool) {
	var (
		data []byte
		err  error
	)
	if pretty {
		data, err = json.MarshalIndent(value, "", "  ")
	} else {
		data, err = json.Marshal(value)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return
	}
	fmt.Println(string(data))
}

func imageURLFromContent(content map[string]interface{}) string {
	if url := toString(content["imageUrl"]); url != "" {
		return url
	}
	if url := toString(content["imageURL"]); url != "" {
		return url
	}
	if url := toString(content["thumbnailUrl"]); url != "" {
		return url
	}
	if url := toString(content["previewImageUrl"]); url != "" {
		return url
	}
	if url := toString(content["imagePreviewUrl"]); url != "" {
		return url
	}
	if url := toString(content["imagePreviewUrlSquare"]); url != "" {
		return url
	}
	if image := content["image"]; image != nil {
		if url := toString(image); url != "" {
			return url
		}
		if m, ok := image.(map[string]interface{}); ok {
			if url := toString(m["url"]); url != "" {
				return url
			}
			if url := toString(m["imageUrl"]); url != "" {
				return url
			}
		}
	}
	if images := toSlice(content["images"]); len(images) > 0 {
		for _, image := range images {
			if url := toString(image); url != "" {
				return url
			}
			if m, ok := image.(map[string]interface{}); ok {
				if url := toString(m["url"]); url != "" {
					return url
				}
				if url := toString(m["imageUrl"]); url != "" {
					return url
				}
			}
		}
	}
	return ""
}

func recipeIngredients(recipe map[string]interface{}, scale float64) []recipeIngredientOutput {
	items := toSlice(recipe["items"])
	if len(items) == 0 {
		items = toSlice(recipe["ingredients"])
	}
	if len(items) == 0 {
		return nil
	}
	ingredients := make([]recipeIngredientOutput, 0, len(items))
	for _, item := range items {
		m := toMap(item)
		name := coalesce(toString(m["itemId"]), toString(m["name"]), toString(m["text"]))
		if name == "" {
			continue
		}
		spec := scaleSpec(toString(m["spec"]), scale)
		ingredients = append(ingredients, recipeIngredientOutput{
			Name:   name,
			Spec:   spec,
			Pantry: toBool(m["stock"]),
		})
	}
	return ingredients
}

func recipeInstructions(recipe map[string]interface{}) []string {
	steps := recipe["instructions"]
	if steps == nil {
		steps = recipe["steps"]
	}
	switch v := steps.(type) {
	case []interface{}:
		lines := []string{}
		for _, step := range v {
			if text, ok := step.(string); ok {
				if text != "" {
					lines = append(lines, text)
				}
				continue
			}
			m := toMap(step)
			text := coalesce(toString(m["text"]), toString(m["description"]))
			if text != "" {
				lines = append(lines, text)
			}
		}
		return lines
	case string:
		if v != "" {
			return []string{v}
		}
	}
	return nil
}

func recipeNutrition(recipe map[string]interface{}) map[string]string {
	raw, ok := recipe["nutrition"].(map[string]interface{})
	if !ok {
		return nil
	}
	out := map[string]string{}
	for key, value := range raw {
		if val := toString(value); val != "" {
			out[key] = val
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseServings(values ...interface{}) int {
	for _, value := range values {
		if value == nil {
			continue
		}
		if i := toInt(value); i > 0 {
			return i
		}
	}
	return 0
}

var specAmountRe = regexp.MustCompile(`^([\d.,]+)\s*`)

func scaleSpec(spec string, scale float64) string {
	if spec == "" || scale == 1 {
		return spec
	}
	match := specAmountRe.FindStringSubmatch(spec)
	if len(match) < 2 {
		return spec
	}
	remaining := strings.TrimSpace(spec[len(match[0]):])
	if strings.HasPrefix(remaining, "-") || strings.HasPrefix(remaining, "/") {
		return spec
	}
	numStr := strings.ReplaceAll(match[1], ",", ".")
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return spec
	}
	scaled := num * scale
	scaledStr := strconv.FormatFloat(scaled, 'f', 1, 64)
	scaledStr = strings.TrimSuffix(scaledStr, ".0")
	scaledStr = strings.ReplaceAll(scaledStr, ".", ",")

	return specAmountRe.ReplaceAllString(spec, scaledStr+" ")
}

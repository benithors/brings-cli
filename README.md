# brings-cli

<img width="400" height="400" alt="image" src="https://github.com/user-attachments/assets/bf603be3-a7e1-4f61-acd2-7c5116fc5a92" />


CLI for [Bring!](https://www.getbring.com/) Shopping Lists - manage lists, add recipes, and more.

## Installation

Build locally:

```bash
go build -o brings ./cmd/brings
```

Or install into your `$GOBIN`:

```bash
go install ./cmd/brings
```

## Homebrew

```bash
brew tap benithors/tap
brew install brings-cli
```

## Playwright (browser login)

Browser-based login uses Playwright. The CLI will try to install Playwright automatically if needed, but you can also pre-install:

```bash
go run github.com/playwright-community/playwright-go/cmd/playwright install
```

## Authentication

Login via browser (recommended - handles bot detection):

```bash
brings login --browser
```

This opens Chrome, you log in to Bring!, and the token is extracted automatically.

## Usage

```bash
# Show all shopping lists
brings lists

# Show items to purchase
brings items

# Add item to list
brings add Milk --spec "2%"

# Remove item
brings remove Milk

# Mark as purchased
brings complete Milk
```

## Recipes

Browse and add recipe ingredients to your shopping list:

```bash
# List saved recipes (JSON by default)
brings inspirations

# Human-friendly output
brings inspirations --format human

# Browse global inspirations stream
brings inspirations all

# Include image URLs (human output)
brings inspirations all --format human --images

# View recipe details (JSON by default)
brings recipe <id>

# Human-friendly recipe details
brings recipe <id> --format human

# Add recipe ingredients to cart
brings add-recipe <id>
```

## Configuration

Set default servings for recipe scaling:

```bash
# Set servings (scales all recipes)
brings config servings 4

# View current config
brings config
```

## All Commands

```
Authentication:
  login --browser           Open browser for login
  login --token <token>     Login with token directly
  logout                    Clear saved credentials
  status                    Show login status

Shopping List:
  lists                     Show all shopping lists
  items [--list <uuid>]     Show items to purchase
  add <item> [--spec ".."]  Add item to list
  remove <item>             Remove item
  complete <item>           Mark as purchased

Recipes:
  inspirations [filter]     List saved recipes with IDs
    --format <mode>         Output format: json (default) | human | pretty
    --images                Include image URLs
  recipe <id>               Show recipe details
    --format <mode>         Output format: json (default) | human | pretty
    --images                Include image URLs
  add-recipe <id>           Add ingredients to shopping list
    --servings <n>          Scale for n servings
    --all                   Include pantry items

Social:
  users                     Show users sharing the list
  notify <type>             Send notification
  activity                  Show recent activity

Settings:
  account                   Show account info
  config                    Show/set configuration
  catalog [locale]          Browse item catalog
```

## Agent Workflow

For AI agents integrating with Bring!:

```bash
# 1. List recipes with IDs
brings inspirations

# 2. Add recipe to shopping list (scaled to config servings)
brings add-recipe <id>
```

## Disclaimer

This project is not affiliated with Bring! Labs AG.

## License

MIT

---
name: agent-browser
description: Use agent-browser CLI to interact with web pages — navigate, click, fill forms, take screenshots, inspect accessibility tree, and verify UI on a headless server.
---

# Agent Browser

Agent-browser is a headless browser automation CLI installed on this server. Use it to navigate, interact with, and verify web applications without a desktop environment.

## Quick Reference

```bash
# Open a URL
agent-browser open "http://localhost:3000"

# See page structure (accessibility tree with element refs)
agent-browser snapshot

# Take a screenshot
agent-browser screenshot /tmp/page.png

# Click an element by ref
agent-browser click @e5

# Fill a form field
agent-browser fill @e3 "test@example.com"

# Type text (keystrokes)
agent-browser type @e3 "hello"

# Select dropdown option
agent-browser select @e4 "option-value"

# Press a key
agent-browser press Enter

# Hover over element
agent-browser hover @e2

# Scroll
agent-browser scroll down
agent-browser scroll @e5

# Get element text
agent-browser get text @e1

# Get page title/URL
agent-browser get title
agent-browser get url

# Set viewport size
agent-browser set viewport 1280 720

# Emulate mobile device
agent-browser set device "iPhone 15"

# Monitor network requests
agent-browser network --filter "api/"

# Wait for element/navigation
agent-browser wait @e5
agent-browser wait --url "**/dashboard"

# Compare screenshots (visual regression)
agent-browser diff before.png after.png

# Close browser
agent-browser close
```

## Workflow: Verify a Page

```bash
agent-browser open "http://localhost:3000"
agent-browser set viewport 1280 720
agent-browser snapshot                    # inspect structure
agent-browser screenshot /tmp/home.png    # visual check
```

## Workflow: Test a Form

```bash
agent-browser open "http://localhost:3000/checkout"
agent-browser snapshot
agent-browser fill @e3 "user@example.com"
agent-browser fill @e5 "2"
agent-browser click @e8                   # submit
agent-browser wait --url "**/success"
agent-browser screenshot /tmp/result.png
```

## Workflow: Debug API Calls

```bash
agent-browser open "http://localhost:3000/dashboard"
agent-browser network --filter "api/"     # watch XHR requests
agent-browser click @e5                   # trigger action
agent-browser network                     # inspect requests/responses
```

## Tips

- Always run `agent-browser snapshot` after navigation to get fresh element refs (`@e1`, `@e2`, etc.)
- Element refs change after page navigation or major DOM updates — re-snapshot
- Use `agent-browser screenshot` to visually verify; read the image with the Read tool to see it
- For mobile testing: `agent-browser set device "iPhone 15"` before navigating
- `agent-browser close` when done to free resources

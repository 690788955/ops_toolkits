# Directory Structure

> Frontend layout for the embedded Ops Console.

---

## Overview

The Web UI is a Vite + React single page app. It is built into static assets and embedded into the Go binary from `internal/server/web/`.

Production deployment must not require a separate Node or frontend server.

---

## Directory Layout

```text
web/
  package.json
  vite.config.js
  index.html
  src/
    main.jsx          # React app and API integration
    styles.css        # console styling
internal/server/web/
  index.html          # Vite build output, embedded by Go
  assets/             # built JS/CSS assets
```

---

## Build Contract

```bash
npm run build --prefix web
```

The Vite config must output to:

```text
internal/server/web/
```

Go embeds that directory in `internal/server/server.go`, so always rebuild the frontend before compiling `opsctl` when UI files change.

---

## UI Organization

The console should keep the following information hierarchy:

1. Left sidebar: categories from `configs/ops.yaml` and plugin contributions.
2. Main tabs: Tools and Workflows.
3. Entry cards: filtered by active category and active tab.
4. Execution panel: generated from `parameters` returned by `/api/catalog`.
5. Result panel: run response and run record from `/api/runs/<run_id>`.

---

## API Contract Used by UI

```text
GET /api/catalog
POST /api/tools/{tool_id}/run
POST /api/workflows/{workflow_id}/run
GET /api/runs/{run_id}
```

Run request payload:

```json
{
  "params": {
    "name": "Tester"
  },
  "confirm": false
}
```

---

## Required Patterns

- Use React state for selected category, active tab, selected entry, params, and result output.
- Keep UI generated from backend YAML/plugin metadata; do not hard-code tools/workflows in React.
- Use `useEffect` cleanup when fetching catalog data to avoid stale updates.
- Keep build output embedded; do not add a production frontend runtime dependency.

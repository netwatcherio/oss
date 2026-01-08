# NetWatcher Panel

The NetWatcher Panel is a Vue 3 SPA for monitoring dashboards, agent management, and visualization.

## Quick Start

```bash
# Install dependencies
npm install

# Development server (http://localhost:5173)
npm run dev

# Production build
npm run build

# Type check
npm run type-check
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `CONTROLLER_ENDPOINT` | API base URL (e.g., `https://api.netwatcher.io`) |

**.env**
```dotenv
CONTROLLER_ENDPOINT=https://api.netwatcher.io
```

**.env.local** (overrides, not committed)
```dotenv
CONTROLLER_ENDPOINT=http://localhost:8080
```

## Docker Build

```bash
# Build image
docker build -t netwatcher-panel .

# Run container
docker run -d \
  --name panel \
  -p 3000:3000 \
  netwatcher-panel
```

## Directory Structure

```
panel/
├── src/
│   ├── components/       # Reusable Vue components
│   ├── composables/      # Vue composables (usePermissions, etc.)
│   ├── router/           # Vue Router config with guards
│   ├── services/         # API service layer
│   ├── utils/            # Utility functions
│   └── views/            # Page components
│       ├── auth/         # Login/register
│       ├── workspace/    # Workspace views
│       ├── agent/        # Agent views
│       └── probes/       # Probe views
├── public/               # Static assets
├── assets/               # Screenshots and images
├── Dockerfile            # Multi-stage build
└── vite.config.ts        # Vite configuration
```

## Permissions

The panel enforces role-based access control at multiple levels:

- **Route Guards**: Protected routes check `meta.requiresRole`
- **UI Guards**: Buttons hidden based on `usePermissions` composable
- **403 Page**: Unauthorized access redirects to `/403`

| Role | Create/Edit | Delete | Manage Members |
|------|:-----------:|:------:|:--------------:|
| OWNER | ✅ | ✅ | ✅ |
| ADMIN | ✅ | ✅ | ✅ |
| USER | ✅ | ❌ | ❌ |
| VIEWER | ❌ | ❌ | ❌ |

See [Permissions](../docs/permissions.md) for details.

## Screenshots

![Workspaces](assets/workspaces.png)
![Workspace Dashboard](assets/workspaceDash.png)
![Agent Dashboard](assets/agentDash.png)
![Traceroute Map](assets/tracerouteMap.png)

## Development

```bash
# Run dev server
npm run dev

# Type check
npm run type-check

# Build for production
npm run build

# Run unit tests
npm run test:unit
```

## Build Output

After `npm run build`, the `dist/` folder contains:
- `index.html` - Entry point
- `assets/` - JS, CSS bundles (~1.2MB gzipped: ~340KB)

Serve with any static file server (nginx, Caddy, etc.).

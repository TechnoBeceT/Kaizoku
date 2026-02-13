# Kaizoku.GO

<table>
  <tr>
    <td width="150">
      <img width="150px" src="./KaizokuFrontend/public/kaizoku.net.png" alt="Kaizoku" />
    </td>
    <td>
      <strong>Kaizoku</strong> is a full rewrite of <a href="https://github.com/maxpiva/Kaizoku.NET"><strong>Kaizoku.NET</strong></a> in Go, built for performance and simplicity.<br/><br/>
      Subscribe to a manga series and it downloads automatically. When new chapters appear on any of your configured sources, they're fetched in the background. Drop it, forget it, read it.
    </td>
  </tr>
</table>

> This project is a Go reimplementation of the original [Kaizoku.NET](https://github.com/maxpiva/Kaizoku.NET) by **maxpiva**, which itself is a modern fork of [Kaizoku](https://github.com/oae/kaizoku) by **OAE**. Full credit goes to the original authors for the concept, UI, and architecture that made this possible.

---

## What It Does

Kaizoku.GO is a **manga series manager** that connects to multiple online sources through [Suwayomi Server](https://github.com/Suwayomi/Suwayomi-Server) and [Mihon](https://github.com/mihonapp/mihon) extensions. It handles the entire lifecycle: searching, subscribing, downloading, updating, and organizing your library with proper metadata.

> Kaizoku does **not** use Suwayomi's built-in download or scheduling logic — only its extension bridge to access Mihon Android APKs from a server environment.

---

## Key Features

- **Priority-Based Multi-Source** — Link one series to multiple sources ranked by priority. Chapters are fetched from your preferred source first; if it fails, the next source in line takes over automatically.
- **Automatic Downloads** — New chapters are detected and downloaded in the background with configurable retry policies.
- **Startup Import Wizard** — Scan an existing library from disk and match it against online sources automatically.
- **ComicInfo.xml Injection** — Every downloaded chapter archive (CBZ) includes rich metadata for readers like Komga, Kavita, or CDisplayEx.
- **Filename Normalization** — Consistent naming across your library for easy re-importing and organization.
- **Extension Management** — Install, update, and configure Mihon extensions directly from the UI.
- **Source Performance Reporting** — Track which sources are slow, failing, or rate-limiting with built-in analytics.
- **Real-Time Progress** — WebSocket-based live updates for all background jobs (downloads, imports, scans).
- **Archive Verification** — Detect corrupted downloads, missing chapters, and orphan files with one-click integrity checks.
- **Per-Series Extras** — `cover.jpg`, `kaizoku.json` metadata mapping, and category-based folder organization.

---

## Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌──────────────────┐
│   Nuxt 4 UI     │────>│   Go Backend     │────>│   PostgreSQL     │
│   (SPA / Bun)   │     │   (Echo + Ent)   │     │   (17-alpine)    │
└─────────────────┘     └────────┬─────────┘     └──────────────────┘
                                 │
                      WebSocket  │  GraphQL / REST
                                 v
                        ┌─────────────────┐
                        │ Suwayomi Server │
                        │ (Embedded Java) │
                        └─────────────────┘
```

| Layer | Technology | Purpose |
|-------|-----------|---------|
| Frontend | Nuxt 4, Vue 3, Nuxt UI, TanStack Vue Query | SPA with reactive server state |
| Backend | Go 1.25, Echo v4, Ent ORM, River job queue | HTTP API, job scheduling, download management |
| Database | PostgreSQL 17 | Persistent storage (replaces SQLite from .NET) |
| Real-time | gorilla/websocket (SignalR protocol) | Live job progress, download status |
| Bridge | Suwayomi Server (Java 21) | Mihon extension host, manga/chapter fetching |

---

## Quick Start with Docker

### Docker Compose (Recommended)

```yaml
services:
  kaizoku:
    image: kaizoku-go:latest  # or build from source
    container_name: kaizoku-go
    ports:
      - "9833:9833"
    environment:
      KAIZOKU_DOCKER: "true"
      KAIZOKU_DATABASE_HOST: postgres
      KAIZOKU_DATABASE_PORT: "5432"
      KAIZOKU_DATABASE_USER: kaizoku
      KAIZOKU_DATABASE_PASSWORD: kaizoku
      KAIZOKU_DATABASE_DBNAME: kaizoku
      KAIZOKU_DATABASE_SSLMODE: disable
    volumes:
      - ./config:/config
      - ./series:/series
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped

  postgres:
    image: postgres:17-alpine
    container_name: kaizoku-postgres
    environment:
      POSTGRES_DB: kaizoku
      POSTGRES_USER: kaizoku
      POSTGRES_PASSWORD: kaizoku
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U kaizoku"]
      interval: 5s
      timeout: 5s
      retries: 5
    restart: unless-stopped

volumes:
  pgdata:
```

Then run:

```bash
docker compose up -d
```

Open **http://localhost:9833** in your browser.

### Volumes

| Container Path | Description |
|----------------|-------------|
| `/config` | Application config, Suwayomi data, logs |
| `/series` | Downloaded manga series storage |

### Ports

| Port | Service | Required |
|------|---------|----------|
| 9833 | Kaizoku Web UI | Yes |
| 4567 | Suwayomi Server | No (optional, for direct access) |

---

## Build from Source

### Prerequisites

- **Go** 1.25+
- **Bun** (latest) — for frontend build
- **PostgreSQL** 17+
- **Java** JRE 21+ (for Suwayomi) — [Adoptium Temurin](https://adoptium.net/)

### Backend

```bash
cd KaizokuBackend
go mod download
go build -ldflags="-s -w" -o kaizoku ./cmd/kaizoku
```

### Frontend

```bash
cd KaizokuFrontend
bun install
bun run dev        # Development server (proxies API to localhost:9833)
bun run generate   # Production static build → .output/public/
```

### Docker Image

```bash
docker build -t kaizoku-go .
```

---

## Configuration

Configuration is loaded in order of priority:

1. **Environment variables** with `KAIZOKU_` prefix (highest priority)
2. **config.yaml** in the config directory
3. **Built-in defaults** (fallback)

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `KAIZOKU_SERVER_PORT` | `9833` | HTTP server port |
| `KAIZOKU_DATABASE_HOST` | `localhost` | PostgreSQL host |
| `KAIZOKU_DATABASE_PORT` | `5432` | PostgreSQL port |
| `KAIZOKU_DATABASE_USER` | `kaizoku` | Database user |
| `KAIZOKU_DATABASE_PASSWORD` | `kaizoku` | Database password |
| `KAIZOKU_DATABASE_DBNAME` | `kaizoku` | Database name |
| `KAIZOKU_DATABASE_SSLMODE` | `disable` | PostgreSQL SSL mode |
| `KAIZOKU_STORAGE_DIR` | (auto) | Override manga storage path |
| `KAIZOKU_DOCKER` | `false` | Set `true` in containers (uses `/config`, `/series`) |
| `KAIZOKU_SUWAYOMI_USE_CUSTOM_API` | `false` | Use an external Suwayomi instance |
| `KAIZOKU_SUWAYOMI_CUSTOM_ENDPOINT` | — | URL to external Suwayomi (e.g. `http://host:4567/api/v1`) |
| `KAIZOKU_SUWAYOMI_PORT` | `4567` | Embedded Suwayomi port |

### Using an External Suwayomi Instance

By default, Kaizoku embeds and auto-launches Suwayomi Server. To use your own:

```bash
KAIZOKU_SUWAYOMI_USE_CUSTOM_API=true
KAIZOKU_SUWAYOMI_CUSTOM_ENDPOINT=http://your-suwayomi:4567/api/v1
```

> **Warning:** Suwayomi assigns internal IDs per instance. If you change Suwayomi servers, ID mappings will no longer match and you'll need to start fresh.

### Config & Data Paths

| Environment | Config Path | Storage Path |
|-------------|-------------|-------------|
| Docker | `/config` | `/series` |
| Linux/macOS | `~/.config/KaizokuNET/` | (configurable) |
| Windows | `%LOCALAPPDATA%\KaizokuNET\` | (configurable) |

---

## Application Settings

These are managed through the web UI at **Settings** and stored in the database:

| Setting | Description |
|---------|-------------|
| Preferred Languages | ISO-639-1 codes for source/search filtering |
| Mihon Repositories | Extension repository URLs |
| Simultaneous Downloads | Max concurrent downloads per provider |
| Simultaneous Searches | Search concurrency across sources |
| Downloads Per Provider | Per-source download cap |
| Update Schedules | Cron expressions for series/source/extension updates |
| Retry Policy | Attempts and delay for failed chapter downloads |
| Categories | Custom folder categories for library organization |

---

## API Overview

All endpoints are under the `/api` prefix.

| Group | Base Path | Description |
|-------|-----------|-------------|
| Series | `/api/serie` | Library CRUD, search, thumbnails, verification |
| Search | `/api/search` | Cross-source search, augment with metadata |
| Downloads | `/api/downloads` | Queue status, metrics, retry/delete failed |
| Providers | `/api/provider` | Extension install/uninstall, preferences |
| Settings | `/api/settings` | Global configuration read/write |
| Setup | `/api/setup` | Import wizard (scan, search, augment, import) |
| Reporting | `/api/reporting` | Source performance analytics and event logs |
| WebSocket | `/progress` | Real-time job progress (SignalR protocol) |
| Health | `/health` | Health check endpoint |

---

## Background Jobs

Powered by [River](https://github.com/riverqueue/river) (PostgreSQL-native job queue):

| Job | Trigger | Description |
|-----|---------|-------------|
| ScanLocalFiles | Import wizard | Scan disk for existing manga folders |
| InstallExtensions | Import wizard | Install required Mihon extensions |
| SearchProviders | Import wizard | Match scanned series to online sources |
| ImportSeries | Import wizard | Create library entries from matches |
| GetChapters | Auto / manual | Fetch latest chapters for a provider |
| GetLatest | Scheduled | Pull latest series from sources |
| UpdateAllSeries | Scheduled | Refresh chapters for all series |
| UpdateExtensions | Scheduled | Check and update installed extensions |
| DailyUpdate | Scheduled | Maintenance: cleanup, prune old data |
| VerifyAll | Manual | Integrity check across entire library |

Downloads use a separate FIFO dispatcher (not River) with per-provider concurrency control and automatic retry with exponential backoff.

---

## Differences from Kaizoku.NET

| Aspect | Kaizoku.NET | Kaizoku.GO |
|--------|-------------|------------|
| **Source Strategy** | **Permanent** — sources are fixed; if the active source fails, chapters are not fetched from alternatives | **Priority-based** — sources are ranked; failures automatically cascade to the next available source |
| Backend | ASP.NET Core / C# | Go (Echo + Ent) |
| Database | SQLite | PostgreSQL |
| Job Queue | Custom Job/Enqueue tables | River (PostgreSQL-native) |
| Real-time | SignalR | WebSocket (SignalR protocol) |
| Frontend | Next.js 15 (React) | Nuxt 4 (Vue 3) |
| Desktop App | Avalonia tray app | Docker only (for now) |
| Config Format | appsettings.json | config.yaml + env vars |

The most significant behavioral change is **source failover**. In Kaizoku.NET, each series has a permanent source — if that source goes down or starts failing, downloads stall until you manually intervene. Kaizoku.GO treats sources as a prioritized list: you pick your preferred source, but if it becomes unavailable, the system automatically falls back to the next source in order. This makes libraries significantly more resilient to source outages without any manual action.

The frontend has been fully rewritten in Vue 3 with Nuxt UI components while preserving the same UX and workflow.

---

## Resource Usage

Kaizoku.GO and the embedded Suwayomi Server can be memory-intensive, especially when managing large libraries or running parallel searches and downloads. PostgreSQL adds a baseline footprint compared to the SQLite-based .NET version, but provides better concurrency and reliability.

---

## Contributing

PRs are welcome. The project is structured for easy onboarding:

- **Backend:** Standard Go project layout in `KaizokuBackend/`. Handlers, jobs, and services are cleanly separated.
- **Frontend:** Nuxt 4 app in `KaizokuFrontend/`. Components follow the `components/{domain}/` convention.
- **No test suite yet** — contributions here are especially appreciated.

---

## Acknowledgments

- [**Kaizoku.NET**](https://github.com/maxpiva/Kaizoku.NET) by **maxpiva** — The original .NET implementation that this project rewrites. The architecture, feature set, and UI design are directly derived from Kaizoku.NET.
- [**Kaizoku**](https://github.com/oae/kaizoku) by **OAE** — The original Kaizoku project and its Next Gen frontend that started it all.
- [**Suwayomi Server**](https://github.com/Suwayomi/Suwayomi-Server) — The Java bridge that makes Mihon Android extensions accessible from server environments.
- [**Mihon**](https://github.com/mihonapp/mihon) — The manga reader whose extension ecosystem powers the source connections.

---

## License

This project is licensed under the [GNU General Public License v3.0](LICENSE) — the same license as [Kaizoku.NET](https://github.com/maxpiva/Kaizoku.NET).

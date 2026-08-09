# Larynx

A linguistics API server for exploring word etymology, pronunciation, and history backed by a graph database of language relationships.

## Features

- **Etymology** — Trace a word's ancestry through a Neo4j graph of `CHILD_OF` relationships (borrowed_from, derived_from, cognate_of, has_root, has_affix, etc.), with matching language family geoJSON polygons.
- **History** — Scrape [etymonline.com](https://www.etymonline.com) and use an LLM to summarize word history into a concise sentence.
- **IPA pronunciation** — Look up International Phonetic Alphabet transcriptions for English words from a SQLite database.
- **Word search** — Search English words by prefix in the Neo4j graph.
- **Blog** — Simple article CRUD stored in SQLite.
- **Caching** — Redis-backed response cache for fast repeat queries.

## Architecture

```
┌────────────┐     ┌──────────┐     ┌──────────┐
│   Client   │────▶│  Chi     │────▶│  Redis   │
└────────────┘     │  Router  │     │  (cache) │
                   │          │     └──────────┘
                   │  Handlers│────▶│  Neo4j   │
                   │          │     │  (graph) │
                   │          │────▶│  SQLite  │
                   │          │     └──────────┘
                   │          │────▶│  OpenAI  │
                   └──────────┘     │  API     │
                                    └──────────┘
```

| Component | Technology | Purpose |
|-----------|-----------|---------|
| HTTP server | Go + Chi v5 | Routing, middleware |
| Graph database | Neo4j | Etymology relationships, language geoJSON |
| Relational database | SQLite (pure Go) | Articles, IPA pronunciations |
| Caching | Redis | Response cache keyed by request URI |
| LLM | OpenAI-compatible API | History summarization |
| HTML scraping | goquery | etymonline.com parsing |

## API Endpoints

All endpoints are mounted under `/api/v1`.

### Health

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health/` | Server version and Neo4j connectivity status |

### Words

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/words/{word}/etymology?lang=English` | Graph paths, ancestor language families, geoJSON |
| `GET` | `/words/{word}/history?lang=English` | LLM-summarized word history from etymonline |
| `GET` | `/words/{word}/ipa?lang=English` | IPA pronunciation from SQLite |
| `GET` | `/words/?prefix={prefix}` | Search English words by prefix |

### Blog

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/blog/articles` | List all articles |
| `POST` | `/blog/articles/create` | Create a new article |
| `GET` | `/blog/articles/{slug}` | Get an article by slug |
| `PATCH` | `/blog/articles/{slug}` | Update an article |
| `DELETE` | `/blog/articles/{slug}` | Delete an article |

## Setup

### Prerequisites

- Go 1.26+
- Docker & Docker Compose
- OpenAI-compatible API key (for history endpoint)
- Neo4j database dump (`neo4j.dump`)

### Quick Start

```bash
# 1. Clone the repository
git clone https://github.com/jmuszka/larynx.git
cd larynx

# 2. Configure environment
cp .env.example .env
# Edit .env with your API key and database credentials

# 3. Start services
cp docker-compose.yml.example docker-compose.yml
# Edit the Neo4j volume mount path in docker-compose.yml
docker compose up -d

# 4. Restore the Neo4j database
docker compose exec neo4j neo4j-admin database restore neo4j --from-path=/data/neo4j.dump

# 5. Build and run
go build -o larynx .
./larynx
```

The server listens on the port specified in `.env` (default: `8080`).

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error`, `fatal` |
| `LOG_FILE` | `./larynx.log` | Log file path (outputs to stdout as well) |
| `NEO4J_URI` | `neo4j://localhost:7687` | Neo4j bolt connection URI |
| `NEO4J_USER` | `neo4j` | Neo4j username |
| `NEO4J_PASSWORD` | *(required)* | Neo4j password |
| `SQLITE_PATH` | `./db.sqlite` | Path to SQLite database file |
| `AI_API_KEY` | — | OpenAI-compatible API key |
| `AI_BASE_URL` | — | LLM API base URL |
| `AI_MODEL` | — | LLM model name (e.g. `deepseek-v4-flash`) |

Relationship subtypes on `CHILD_OF`: `borrowed_from`, `derived_from`, `has_root`, `has_affix`, `cognate_of`.

## Caching

All word endpoints (etymology, history, IPA) check Redis before executing expensive operations. Cache keys are the full request URI. Misses populate the cache with no TTL expiration.

## Project Structure

```
.
├── main.go              # Entry point
├── go.mod / go.sum      # Go module
├── docker-compose.yml   # Neo4j + Redis containers
├── .env                 # Environment configuration
├── internal/
│   ├── ai/              # OpenAI-compatible chat client
│   ├── cache/           # Redis cache wrapper
│   ├── logging/         # Structured JSON logger (slog)
│   └── server/          # HTTP server, routing, handlers
├── scripts/
│   └── pre-commit       # Pre-commit lint hook
└── .github/workflows/   # CI (go vet + gofmt)
```

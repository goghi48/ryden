<p align="center">
  <img src="web/public/brand/ryden-frog-wave.webp" width="120" alt="Ryden frog logo">
</p>

<h1 align="center">Ryden</h1>

<p align="center">
  Turn “we should meet sometime” into a plan everyone can actually follow.
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/PostgreSQL-4169E1?style=for-the-badge&logo=postgresql&logoColor=white" alt="PostgreSQL">
  <img src="https://img.shields.io/badge/Docker-2496ED?style=for-the-badge&logo=docker&logoColor=white" alt="Docker">
  <img src="https://img.shields.io/badge/Prometheus-E6522C?style=for-the-badge&logo=prometheus&logoColor=white" alt="Prometheus">
  <img src="https://img.shields.io/badge/React-20232A?style=for-the-badge&logo=react&logoColor=61DAFB" alt="React">
  <img src="https://img.shields.io/badge/TypeScript-3178C6?style=for-the-badge&logo=typescript&logoColor=white" alt="TypeScript">
</p>

## About

Ryden is an open-source planner for private groups. It keeps the decisions that usually get lost in a group chat in one place: what to do, when everyone is available, who is attending, and who is bringing what.

It is not an event marketplace or a social feed. Meetings are private and available only to authenticated participants.

## Features

- create a ready-to-go event with a title, time, place, duration, and photo;
- plan a meeting together using plan options and availability voting;
- invite people with a private link or directly from a friend list;
- collect `going`, `maybe`, and `not going` responses;
- create anonymous or named polls with optional revoting;
- keep an auditable history of non-anonymous votes;
- confirm the final plan as the organizer;
- share preparation items, quantities, and responsibilities;
- add participant notes and export a confirmed meeting to a calendar;
- keep completed and cancelled meetings in a read-only archive.

## Technology

| Area | Technology |
| --- | --- |
| Backend | Go 1.26, REST API, `pgx`, `sqlc` |
| Database | PostgreSQL 17 |
| Frontend | React 19, TypeScript, Vite |
| Testing | Go test, Vitest, Testing Library |
| Observability | Structured logs, Prometheus metrics, health endpoints |
| Delivery | Docker, Docker Compose, nginx, GitHub Actions |

Ryden is a modular monolith. The application stays simple to deploy while account, meeting, voting, media, invitation, and preparation logic remain separated inside the codebase. PostgreSQL is the source of truth.

## Run locally

You need Git and Docker Desktop.

```bash
cp .env.example .env
docker compose up --build
```

Open [http://localhost:5173](http://localhost:5173). The API is available at `http://localhost:8080`.

Create two or more accounts to try invitations and group decisions. Local credentials from `.env.example` are development-only and must never be reused in a public deployment.

## Production deployment

The repository includes a production-oriented Compose configuration for a small single-server deployment.

```bash
cp .env.production.example .env.production
# Replace every CHANGE_ME value before continuing.
docker compose --env-file .env.production -f compose.production.yaml up -d --build
```

Before exposing Ryden to the internet:

1. Use a real domain and terminate HTTPS in a trusted reverse proxy.
2. Generate independent database and token secrets.
3. Keep `.env.production` on the server, outside Git and image layers.
4. Restrict the allowed origin to the public HTTPS domain.
5. Configure database backups and test a restore.
6. Monitor `/livez`, `/readyz`, `/startupz`, logs, metrics, disk space, and database connections.
7. Rebuild images regularly to receive dependency and base-image security updates.

The production Compose file keeps PostgreSQL and the API off public host ports, uses non-root containers where supported, sets resource and process limits, rotates container logs, and enables secure cookies.

## Secrets

Commit configuration examples, never real values. Store deployment values in GitHub Actions secrets, your hosting provider's secret store, or a protected environment file on the server.

The following files are intentionally ignored:

- `.env` and `.env.*`, except example templates;
- private keys and certificates;
- database backups and exported user data;
- local editor, cache, build, and internal planning files.

If a secret reaches Git history, rotate it immediately. Removing the line in a later commit does not make the old value safe.

## Project structure

```text
api/                    OpenAPI contract
cmd/ryden/              application entry point
internal/               backend modules and infrastructure
internal/migrations/    PostgreSQL migrations
load/k6/                bounded load-test scenarios
web/                    React application and nginx configuration
compose.yaml            local development stack
compose.production.yaml production-oriented stack
```

## Quality checks

GitHub Actions runs formatting, static analysis, backend tests with the race detector, frontend linting, type checks, tests, and a production build.

```bash
go fmt ./...
go vet ./...
go test ./...

cd web
pnpm install --frozen-lockfile
pnpm lint
pnpm typecheck
pnpm test
pnpm build
```

## Project status

Ryden is a working MVP and portfolio project. The core multi-account flow is implemented and covered by automated tests. It has production-oriented defaults, but every public deployment still needs real monitoring, backups, HTTPS, secret management, and capacity testing on its own infrastructure.

## Contributing

Issues and focused pull requests are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) before making a significant change.

## License

Ryden is available under the [MIT License](LICENSE).

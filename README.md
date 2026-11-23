# Focus Helper

A private workspace for tasks, timers, and Markdown documents. Tasks can finish by hand or
when a timer runs out. Documents can be grouped into folders and edited from multiple tabs or devices.

- **Backend:** Go, MongoDB, and the official MongoDB Go driver
- **Frontend:** React 19 + TypeScript, Vite, React Router

```
backend/    Go HTTP API and timer logic
frontend/   React app, nginx config, Dockerfile
deploy/     Optional HTTPS gateway
scripts/    Legacy SQLite export tool
docker-compose.yml
```

## Features

| # | Requirement | How it works |
|---|-------------|--------------|
| 1 | Two task types | `CLASSIC` is completed with a button. `TIMED` has its own timer and is completed automatically when the timer reaches zero, or by hand with **Done** (for work done off-site). |
| 2 | Timeout for CLASSIC | Optional "time constraint" set at creation (for example `1 hr 15 min`). When it passes, the task moves to **Cancelled**. `TIMED` tasks cannot have a timeout (rejected with 422). |
| 3 | Three views | **To do**, **Completed** and **Cancelled** tabs on the Tasks page. A task can be restored from either bin, and a restore clears its timeout and resets its timer. |
| 4 | Timers | Start, pause, resume, reset, and change the remaining time (click the digits). Timer state lives in the database, so it survives reloads and server restarts. Standalone timers are on the Timers page. Task timers appear on the Timers page and inline in the task. |
| 5 | Deadlines sort | The to-do list is ordered by nearest deadline. Tasks without a deadline go last. |
| 6 | Deadlines shown when near | A deadline is displayed only when it is 24 hours away or less, or already overdue. |
| – | Encryption | Optional, in Settings (⚙). A key is generated in the browser (AES-256-GCM) and kept in `localStorage`; titles, descriptions and timer names are sent as `enc:v1:<base64>`. Plaintext entries are still shown as they are. The server never sees the key. |
| – | Document vault | Create and move documents, edit Markdown, and preview the formatted text. Concurrent saves merge against the originally opened revision and retain both writers’ conflicting lines. |
| – | Themes | Paper, Midnight, Blossom (pink), Ocean and Forest, chosen in Settings. |
| – | Timespans typed as phrases | Time constraints, timed-task durations and timers take `15 minutes`, `1 hr 15 min`, `1 hour`. Whole minutes only, and seconds are rejected. See [TIMING_RULES.md](TIMING_RULES.md). |
| – | Deadlines typed as phrases | The deadline box understands `today`, `tomorrow 3:50 PM`, `next tuesday`, `until friday`, and so on, with a live preview. Times are 24-hour unless AM/PM is given, and the past is rejected. Full rules are in [TIMING_RULES.md](TIMING_RULES.md). |
| 7 | Minimal main page | Enter a task and tap **Add task** or press Enter. An "options" link reveals type, duration or timeout, and deadline. |

### Design notes

- **No background worker.** Remaining time is derived, not ticked:
  `remaining = duration − elapsed − (now − started_at if running)`. Each task, timer, or state request settles expired records using indexed MongoDB queries. The completion
  and cancellation timestamps use the exact moment of expiry, not the time of the request.
- **Clock-skew safe.** The API returns `server_time` and `remaining_ms`. The browser counts down
  from those and never compares its own clock to server timestamps. The UI also reloads at the
  next expiry, every 20 seconds, and when the tab becomes visible again.
- **Timestamps** are stored as UTC and returned as ISO 8601 with `Z`. Input datetimes must carry a
  timezone, and naive datetimes get a 422.
- **Task timers** are tied to the task. Cancelling or completing the task pauses its timer.
  Resetting the timer of a completed `TIMED` task reopens the task.

## Run locally

Create a bcrypt hash for your sign-in password with `htpasswd -nBC 12 focus`. Put the hash after the username in a private `.env` at the project root. Quote it with single quotes so the dollar signs are preserved.

```dotenv
APP_USERNAME=focus
APP_PASSWORD_HASH='<bcrypt hash>'
```

Then start the app with Docker Compose.

```bash
docker compose up --build --wait
```

Open http://localhost:8080. Compose binds the frontend to loopback by default and keeps MongoDB on a private network. The `mongo_data` volume stores tasks and timers across container restarts.

To run the backend separately, start MongoDB locally, export the login settings and `APP_SESSION_SECURE=false`, and run `go run .` in `backend/` with Go 1.25 or newer. Run `npm ci && npm run dev` in `frontend/` with Node 24. Vite proxies `/api` to port 8000. Set `API_URL` to use another backend.

| Backend variable | Default |
|---|---|
| `MONGODB_URI` | `mongodb://localhost:27017` |
| `MONGODB_DATABASE` | `focus_helper` |
| `HTTP_ADDR` | `:8000` |
| `CORS_ORIGINS` | `["http://localhost:5173"]` |
| `BIN_LIMIT` | `200` (the combined finished history is capped at twice this value) |
| `APP_USERNAME` | `focus` |
| `APP_PASSWORD_HASH` | Required to enable sign-in |
| `APP_SESSION_SECURE` | `true` when running Go directly or using the HTTPS override, `false` with the local HTTP Compose configuration |

Run `go test ./...` in `backend/` and `npm test && npm run build` in `frontend/`. Set `MONGODB_TEST_URI` to run the Go API integration test against a temporary database on a MongoDB test server. Open `/api/docs` for interactive API documentation. `/healthz` checks the MongoDB connection.

## Import an existing SQLite database

Export from a consistent copy of the old database, then stream into a fresh MongoDB deployment before accepting user traffic. Python 3 is only required for this one-time SQLite export.

```bash
set -o pipefail
python3 scripts/export-sqlite.py /path/to/focus_helper.db |
  docker compose exec -T backend focus-helper import
```

The import preserves IDs, task statuses, and timer state, including running timers. Existing IDs are skipped on retry. Keep the SQLite copy as a backup.

## Deploy from a Git checkout

Clone the GitHub repository on the server. Run `docker compose up -d --build --wait` from the checkout to start the application. For a public endpoint, use the HTTPS Compose override with a valid certificate. Put the bcrypt login settings in the server’s private deployment environment file before starting this version.

```bash
docker compose --env-file /path/to/deployment.env \
  -f docker-compose.yml -f deploy/compose.https.yml up -d --build --wait
```

Set `HTTPS_CERT_DIR` to the directory containing `fullchain.pem` and `privkey.pem`. Keep the certificate directory and the deployment environment file outside the checkout. The login form replaces the gateway’s Basic authentication. If upgrading, generate a bcrypt hash for the password you want to use with the form. The HTTPS port defaults to 8443. Reload nginx in the `gateway` service when certificates renew. To deploy an update, run `git pull --ff-only` and repeat the Compose command.

## API

All routes are under `/api`.

| Method & path | Purpose |
|---------------|---------|
| `GET /session` | Current signed-in username, or null |
| `POST /session` / `DELETE /session` | Sign in with username and password, or sign out |
| `GET /vault?folder={id}` | List folders and documents in the selected folder. Omit the query for the root. |
| `POST /vault/folders` | Create a folder with `{name}` |
| `POST /vault/documents` | Create a document with `{title, folder_id, markdown}` |
| `GET /vault/documents/{id}` | Read Markdown and its current revision |
| `PUT /vault/documents/{id}` | Save or move a document. Include the revision originally loaded by the editor. |
| `GET /state` | Server time, tasks of all three statuses, and active timers in one call |
| `POST /tasks` | `{title, type, deadline?, timeout_seconds? (CLASSIC), duration_seconds (TIMED)}` |
| `PATCH /tasks/{id}` | Change `title`, `deadline`, or `timeout_seconds` (null clears it) |
| `POST /tasks/{id}/complete` `/cancel` `/reopen` | State changes |
| `DELETE /tasks/{id}` | Also deletes its timer |
| `POST /timers` | Standalone timer `{name?, duration_seconds}` |
| `PATCH /timers/{id}` | `{name?, remaining_seconds?}`. Changes the time left, in any state. |
| `POST /timers/{id}/start` `/pause` `/reset` | Timer control |
| `DELETE /timers/{id}` | Standalone timers only |

## Known limits

- The app shares one password-protected workspace. Document Markdown is stored in MongoDB for server-side merging. Document saves are limited to 1 MiB of Markdown. A save that exceeds the limit keeps the previous saved version and leaves the draft in the editor.
- Deadlines cannot be in the past (the API returns 422).
- Timers and timeouts are settled when someone makes a request. If nobody opens the app, a task
  stays in "To do" in the database until the next request. The next view shows it with the
  correct timestamps.

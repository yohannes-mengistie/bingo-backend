# Project Context — Habesha Bingo

A real-money, multiplayer **75-ball bingo** platform played through a **Telegram
Mini App**, with an admin dashboard for operators. This file is the high-level
map of the whole system plus a log of recent work. For backend internals see
[`ARCHITECTURE.md`](./ARCHITECTURE.md); for game rules see
[`GAME_RULES.md`](./GAME_RULES.md) / [`GAME_RULES_QUICK.md`](./GAME_RULES_QUICK.md).

---

## 1. The three repositories

| Repo | Path | Remote | Hosts | Deploy |
|---|---|---|---|---|
| **Backend** | `~/bingo-backend` | `bingo-backend` | Go (Gin) HTTP + WebSocket API | Render (Docker) — `https://bingo-api-c6un.onrender.com` |
| **Frontend** | `~/bingo-miniapp` | `bingo-frontend` | Player Mini App (`src/`) **and** Admin dashboard (`admin/`) | Vercel (two projects) |

> ⚠️ `~/bingo-admin/` is a **stale, divergent, non-deployed** clone. Do **not**
> edit it — the live admin lives at `~/bingo-miniapp/admin/`.

Both frontends are React + Vite + TypeScript + Tailwind. The player app uses
`react-router` + `zustand` + `@tanstack/react-query`; the admin app uses
`react-router` + `zustand`.

---

## 2. What the system does

- **Players** open the Mini App inside Telegram. They are authenticated via
  Telegram `initData` (HMAC verified with the bot token → JWT).
- **Admins** log in with **phone + password** (bcrypt → JWT) to the admin
  dashboard. Admin routes additionally require an `admin` role claim.
- A **Telegram bot** is the registration gateway (webhook secured by a shared
  secret header).
- **Money** (wallets, transactions, games) lives in **PostgreSQL** (durable,
  row-locked for safety). **Redis** holds ephemeral live-game state and is the
  pub/sub backbone for the real-time WebSocket stream.
- Payments (CBE / Telebirr) are **manual**: players send money to the house
  accounts and submit a reference; an admin approves the deposit. No automated
  payment gateway.

---

## 3. Game model (75-ball bingo)

- **Card:** 5×5 grid, columns **B I N G O**. Each letter owns 15 numbers:
  B 1–15, I 16–30, N 31–45, G 46–60, O 61–75 → **75 numbers total**. Center
  cell (`N` middle) is **FREE**. Cards are deterministic, IDs **1–200**.
- **Stakes / game types:** G1=5, G2=7, G3=10, G4=20, G5=50, G6=100, G7=200 birr.
- **House cut:** 20%. Prize pool per game = `stake × players × 0.8`. Winner takes
  the whole pool.
- **Flow:** `WAITING` → (2nd player joins) `COUNTDOWN` (60s) → `DRAWING`
  (a number every 3s) → `FINISHED` (someone claims a valid bingo) or
  `CANCELLED` (refunded).
- **Min players:** 2. **Win patterns:** any row, column, either diagonal, or
  four corners (validated server-side in `pkg/bingo`, mirrored client-side in
  `src/lib/bingo.ts`).
- **Refund rules:** leave during `WAITING`/`COUNTDOWN` → full refund; leave
  during `DRAWING` → forfeit. Invalid claim → eliminated (no refund).

---

## 4. Backend layout (clean architecture)

```
cmd/server/main.go        composition root: wires repos → usecases → handlers, routes
internal/handler/         Gin HTTP/WS handlers (auth, user, wallet, game, websocket, telegram)
internal/middleware/      JWT auth + admin gate
internal/usecase/         business rules (auth, user, wallet, game)
internal/domain/          entities, repo interfaces, DTOs, constants
internal/repository/postgres/  SQL implementations
pkg/                      jwt, auth(bcrypt), telegram, bingo(engine), redis, referral, utils
migrations/               init.sql + numbered migrations
```

Money operations run inside DB transactions with `SELECT … FOR UPDATE` row
locks. The single-winner guard is an atomic conditional `UPDATE … WHERE
state='DRAWING'`.

### Key API surface
- Public: `GET /games`, `/games/recent-winners`, `/games/:id/state`, `/cards/:id`
- Player (JWT): `/me`, `/me/wallet`, `POST /wallet/{deposit,withdraw,transfer}`,
  `POST /games/:id/{join,leave,bingo}`
- WebSocket: `/ws/game?type=G5` or `/ws/game/:gameId`
- Admin (JWT + admin): `/admin/users…`, `/admin/transactions…`,
  `/admin/stats/dashboard`, **`/admin/games`**, **`/admin/games/:id`**,
  **`/admin/games/:id/cancel`**

---

## 5. Real-time WebSocket events

The WS handler forwards the backend's `{event, data}` envelope verbatim from
Redis pub/sub. Events: `INITIAL_STATE`, `GAME_STATUS` (carries `status`),
`COUNTDOWN`, `NUMBER_DRAWN`, `PLAYER_JOINED`, `PLAYER_LEFT`,
`PLAYER_ELIMINATED` (`userId`), `WINNER` (`user_id`, `winner_name`, `prize`),
`NEW_GAME_AVAILABLE`. The player app maps these to live UI updates.

---

## 6. Recent progress (this work cycle)

### Backend (`bingo-backend`, all merged to `main`)
- **Admin game management** — `GET /admin/games` (list, filters, pagination),
  `GET /admin/games/:id` (detail + players), `POST /admin/games/:id/cancel`
  (force-cancel + refund every active player, atomic, game-row locked).
- **Stuck-game auto-refund (money-trap fix)** — previously, when all 75 numbers
  were drawn with no winner, the draw loop spun forever and stakes were locked
  permanently. Now it auto-cancels and refunds everyone. Admin cancel and this
  path share one `cancelGameAndRefund` function.
- **Idempotent rejoin** — `JoinGame` returns the existing player if already in
  the game (reconnect-safe; no double charge).
- **Public recent-winners endpoint** — `GET /games/recent-winners` for the
  lobby trust feed.

### Player Mini App (`bingo-miniapp/src/`, merged to `main`)
- **WebSocket fixes** — read `GAME_STATUS.status` (was `state`); show a
  "cancelled — refunded" overlay; handle `PLAYER_JOINED`/`PLAYER_LEFT`; read
  `PLAYER_ELIMINATED.userId`.
- **One-page game room** — fixed to the viewport (no scroll); the tall 75-cell
  master board replaced by a compact "called" strip (current ball + recent
  calls, lettered like `N42`); the card flexes to fit.
- **Manual marking** — removed auto-daub; the player taps each called number.
- **Winner reveal** — every player (incl. losers/eliminated) sees who won and
  how much at game end.
- **Recent-winners lobby feed** — persistent, auto-refreshing list of recent
  winners (name · stake · prize · time).
- **Deposit destinations** — the deposit screen shows the house Telebirr number
  / CBE account (by method) with a copy button. Configured via `VITE_*` env —
  ⚠️ **still placeholders; set the real numbers before players deposit**
  (`VITE_TELEBIRR_NUMBER`, `VITE_CBE_ACCOUNT`, `VITE_PAYMENT_NAME`).
- **UI refinement** — tighter, more professional sizing via the shared
  `Button`/`Card`/`Sheet` primitives.
- **Back navigation** — wired Telegram's native `BackButton` app-wide (returns
  to lobby on every non-home screen) + a game-room back chevron (non-destructive
  minimize, distinct from the "Leave"/refund button).

### Admin dashboard (`bingo-miniapp/admin/`, merged to `main`)
- **Games section** — list (state/type filters), detail + player roster,
  **Cancel & refund** a game. The tool to recover money from stuck/abandoned
  games.
- **Mobile responsive** — off-canvas drawer + hamburger; tables scroll inside
  their card instead of stretching the page.

### Verification
End-to-end tested against a local stack rebuilt from `main` (identical to
deployed code): seeded users, minted JWTs, drove join/leave/countdown, a real
win + payout, admin cancel+refund, and the 75-draw auto-refund — **44 checks,
all green**. (One blank-names failure was local schema drift — migration
`012_add_user_banned` not applied locally; production has it.)

---

## 7. Configuration & gotchas

- **Backend env:** `DATABASE`/`DB_*`, `REDIS_*`, `JWT_SECRET` (Render-generated,
  never exposed — so prod JWTs can't be minted locally), `SECRET_CODE`,
  `TELEGRAM_BOT_TOKEN`, `TELEGRAM_WEBHOOK_SECRET`, `TELEGRAM_MINIAPP_URL`.
- **Frontend env (player):** `VITE_API_BASE`, `VITE_WS_BASE`,
  `VITE_BOT_USERNAME`, and the deposit-account `VITE_*` above. Dev shims
  (`VITE_DEV_TELEGRAM_SHIM`, `VITE_DEV_MOCK_AUTH`) must be **off** in prod.
- **Render free Postgres** is deleted ~30 days after creation — fine for now,
  not durable long-term.
- **Local dev:** `docker compose up -d` runs api(:8000) + postgres(:5432) +
  redis. Rebuild the api image (`docker compose build api`) to pick up code
  changes — a stale container will 404 on new routes.
- **Deploy order when a frontend change depends on a new backend endpoint:**
  ship the **backend first**, then the frontend (the lobby feed degrades
  gracefully — just hidden — if the endpoint isn't there yet).

---

## 8. Open follow-ups

- Set the **real CBE / Telebirr deposit account numbers** (currently placeholders).
- Eyeball the **UI sizing refinement** and **Telegram back button** on a real
  device (taste/native behavior can't be verified in CI).
- Backend `add-telegram-bot` branch is still open (not created in this cycle).
- Optional: louder error surfacing in admin game-detail enrichment; concurrency
  stress-testing of simultaneous claims/joins (guards exist, not load-tested).

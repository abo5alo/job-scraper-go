# job-scraper-go

A job search platform for the Middle East, written in Go. It collects open
positions straight from companies' own hiring systems, normalizes them into
one schema, stores them in PostgreSQL, and serves them through a REST API and
a search page.

![Search page showing backend jobs in Egypt](docs/screenshot.jpg)

## Highlights

- **~3,400 jobs from 18 sources** across 10 countries: Workable's job
  search (one per country), company job boards on 5 different applicant
  tracking systems, and the Remote OK API. One scrape takes about 6 minutes,
  and that's deliberate: the scrapers are rate limited per host.
- **Full-text search** with filters for country, city, seniority, company and
  remote work, built on Postgres, with no separate search engine.
- **Knows when a job closes.** Company feeds list every open job, so one that
  disappears has been filled. Closed jobs stay in the database for history
  but drop out of search, and so do postings older than 3 months.
- **Cleans up messy real-world data:** country detection from text like
  "Cairo Office", seniority inferred from titles (including regional terms
  like Saudi Arabia's *Tamheer* graduate program), and repair of broken
  text encoding and leftover HTML entities.
- **Polite scraping and a protected API:** per-host rate limits, retries
  with backoff and jitter, respect for `Retry-After`, bounded concurrency,
  and per-client rate limiting on the API.
- **Tested at every layer:** scrapers against local fake servers built from
  real responses, API handlers against a fake store, and SQL against a real
  Postgres.

## How it works

```mermaid
flowchart LR
    subgraph sources [Job sources]
        W[Workable search<br/>10 countries]
        A[Company ATS boards<br/>Greenhouse, Ashby,<br/>SmartRecruiters, Recruitee]
        R[Remote OK API]
    end

    subgraph scraper [cmd/scraper]
        C[Rate-limited<br/>HTTP client]
        N[Normalize<br/>country, seniority, text]
    end

    DB[(PostgreSQL<br/>full-text index)]

    subgraph api [cmd/api]
        H[REST API<br/>+ rate limiter]
        P[Search page]
    end

    W & A & R --> C --> N --> DB
    DB --> H --> P
```

Scraping and searching are separate programs. The scraper fills the
database on a schedule; the API only reads it. Searching never waits on a
job site, and our traffic to those sites doesn't grow with the number of
users.

## Running locally

Requires Go 1.27+ and Docker.

```sh
docker compose up -d        # start PostgreSQL
go run ./cmd/scraper        # collect jobs from every source (~6 minutes)
go run ./cmd/api            # start the server
```

Then open **http://localhost:8080**.

| Variable | Default |
|---|---|
| `DATABASE_URL` | `postgres://jobs:jobs@localhost:5432/jobs?sslmode=disable` |
| `ADDR` | `:8080` |

## API

### `GET /jobs`: search open jobs

| Parameter | Example | Notes |
|---|---|---|
| `q` | `backend engineer` | Full-text search over title, company and description. Supports `"exact phrases"` and `-exclusions`. Title matches rank highest. |
| `level` | `senior,lead` | Any of `intern`, `junior`, `mid`, `senior`, `lead`, `executive` |
| `country` | `EG,SA` or `saudi arabia` | Codes or names |
| `location` | `riyadh` | Substring of the location text |
| `company` | `careem` | Substring of the company name |
| `remote` | `true` | |
| `include_closed` | `true` | Also return jobs that have been filled |
| `sort` | `newest` | `relevance` (default when `q` is set) or `newest` |
| `page`, `limit` | `2`, `50` | `limit` is at most 100 |

```sh
curl "localhost:8080/jobs?q=backend&country=EG&level=senior"
```

```json
{
  "total": 25,
  "page": 1,
  "limit": 20,
  "jobs": [
    {
      "id": 2940,
      "title": "Senior Java Backend Engineer | Spring Boot | Microservices | Kafka",
      "company": "SSC HR Solutions",
      "location": "Cairo, Egypt",
      "country": "EG",
      "remote": true,
      "level": "senior",
      "url": "https://jobs.workable.com/view/...",
      "source": "workable",
      "description": "First 300 characters…",
      "posted_at": "2026-09-17T11:27:58+03:00",
      "first_seen_at": "2026-09-27T17:18:32+03:00"
    }
  ]
}
```

Other endpoints:

- `GET /jobs/{id}`: one job with its full description
- `GET /countries`: open job counts per country (used by the search page's dropdown)
- `GET /healthz`: 200 when the server can reach the database

Invalid input gets a `400` that says what was wrong, e.g.
`unknown level "wizard"; use one of: intern, junior, mid, senior, lead, executive`.

## Configuring sources

Everything the scraper reads is listed in [`sources.yaml`](sources.yaml).
Adding a country or company is a one-line change:

```yaml
workable_search:
  countries: [Saudi Arabia, United Arab Emirates, Egypt, ...]

companies:
  - { company: Careem, ats: greenhouse, slug: careem }
```

A company's slug is its account name in its applicant tracking system, and
it's visible in the URL of its careers page. Check it before adding: generic
slugs like `rain` and `spare` exist, but belong to unrelated companies in
New York and Vancouver.

Removing a source is also safe: its jobs are closed on the next run instead
of lingering in search forever.

## Design decisions

**Go to where jobs originate, not to aggregators.** Companies post jobs in
an applicant tracking system (ATS) like Greenhouse or Workable, which
publishes them through a public feed that LinkedIn and Indeed copy from.
Reading those feeds gets the same jobs, often sooner, without breaking any
site's terms. LinkedIn, Indeed and the big regional boards forbid scraping,
so they aren't used.

**One unified schema, normalized once.** Each source converts its own format
into a shared `Job` type. Missing fields are then filled by the same rules
for every source. Structured data wins when a source has it (Workable's
country code, SmartRecruiters' experience level), with text inference as the
fallback. The exception is an explicit title keyword like "Senior" or
"Intern", which beats an ATS level field that companies often leave on its
default. Unknown salaries are stored as `NULL`, never `0`, so they can't
drag averages down.

**Polite scraping.** All scrapers share one HTTP client that:

- rate-limits **per host** with a token bucket, so 10 country searches on
  the same site are spaced out instead of sent in a burst
- retries temporary failures (network errors, `429`, `5xx`) with exponential
  backoff and jitter, but not permanent ones like `404`
- honors `Retry-After`, and **gives up** when a server asks for a longer wait
  than we're willing to spend, rather than retrying sooner than asked

**Bounded concurrency.** Scrapers run in parallel, capped by a semaphore
(a buffered channel), so the source list can grow without opening hundreds of
connections. Each scraper has its own timeout, and a failing or panicking
scraper can't affect the others.

**Closing jobs without false alarms.** A job missing from a full feed is
marked closed, and reopened if it reappears. Two safeguards stop a glitch from
wiping good data: a feed that suddenly returns zero jobs closes nothing, and
the Workable search refuses to save a run where it collected under 90% of
the total the API itself reported.

**Postgres full-text search.** A generated `tsvector` column with a GIN
index handles stemming ("engineering" matches "Engineer") and weighted
ranking (title over company over description). That's all this project
needs, without running Elasticsearch next to the database.

**Plain SQL with pgx, no ORM.** Every query is visible. User input reaches SQL
only as query parameters, and `LIKE` wildcards in input are escaped. All
jobs from a source are saved in one batched round trip.

**A protected API.** Each client IP gets its own token bucket (bursts
allowed, sustained rate capped), and idle buckets are swept so memory
doesn't grow forever. `X-Forwarded-For` is deliberately ignored, because any
client can forge it to dodge the limit. The server sets read and write
timeouts against slow-client attacks and shuts down gracefully.

**A search page without a frontend build.** Plain HTML, CSS and JavaScript,
embedded into the Go binary, so the whole app still ships as one file. Job
data comes from third-party sites, so the page only ever inserts it as text
(never as HTML), allows only `http(s)` links, and sends a
Content-Security-Policy so the browser refuses to run any script that isn't
ours.

## Lessons from real data

Things the scrapers ran into on real data, each handled in code and covered
by a test:

| What happened | How it's handled |
|---|---|
| Remote OK sends accented text garbled ("MecÃ¡nico") | Detect UTF-8 that was decoded as Latin-1 and reverse it, leaving genuine Latin-1 text alone |
| Greenhouse escapes its HTML twice (`&lt;p&gt;`) | Unescape once before stripping tags |
| A company's titles contain `&amp` with no semicolon | Decode HTML entities in all text fields |
| Remote OK tags cider technicians and voice actors as `golang` | Source tags are treated as hints, not facts |
| Namshi still lists jobs posted in 2017 | Search hides postings older than 3 months |
| "CEO Office Manager" was classified as an executive | Phrases that contain a level word without meaning that level are removed before matching |
| Workable answered with `Retry-After: 86205`, a daily quota, and the client retried after 60 seconds anyway | Requests asked to wait longer than a minute now fail immediately |

## Testing

```sh
go test ./...
```

Unit tests need no network or database. Scrapers run against local fake
servers that serve trimmed copies of real responses, and the API handlers
run against a fake store.

The store's integration tests run real SQL against a separate test database:

```sh
docker compose exec postgres createdb -U jobs jobs_test
TEST_DATABASE_URL="postgres://jobs:jobs@localhost:5432/jobs_test?sslmode=disable" go test ./internal/store
```

## Project layout

```
cmd/scraper/                     collect jobs from every source once
cmd/api/                         HTTP server: REST API and search page
sources.yaml                     which countries and companies to scrape
internal/job/                    the unified Job type, seniority and country detection
internal/scraper/                Scraper interface, concurrent runner, rate-limited HTTP client
internal/scraper/ats/            Greenhouse, Ashby, Workable, SmartRecruiters, Recruitee
internal/scraper/workablesearch/ Workable's cross-company job search
internal/scraper/remoteok/       Remote OK API
internal/store/                  PostgreSQL: migrations, upserts, search
internal/api/                    handlers, validation, rate limiting, logging
internal/web/                    the search page (embedded HTML, CSS, JS)
```

## Roadmap

- [ ] Fit the Workable searches within its daily request quota (larger pages, or spreading countries across days)
- [ ] Run the scraper on a daily schedule
- [ ] Workday-hosted career sites, for large employers like airlines, banks and energy companies
- [ ] Analytics endpoints: in-demand skills, salary ranges, how long jobs stay open
- [ ] Dockerfile for the app and CI with GitHub Actions
- [ ] Deploy a public demo

## Data sources

Job data from [Remote OK](https://remoteok.com) is used under their API
terms, which require crediting them and linking back to each posting. Other
jobs come from the public job feeds of each company's applicant tracking
system and from Workable's public job search. Every job links back to its
original posting.

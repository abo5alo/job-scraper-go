# job-scraper-go

[![CI](https://github.com/abo5alo/job-scraper-go/actions/workflows/ci.yml/badge.svg)](https://github.com/abo5alo/job-scraper-go/actions/workflows/ci.yml)

A job search platform for the Middle East, written in Go. It collects open
positions straight from companies' own hiring systems, normalizes them into
one schema, stores them in PostgreSQL, and serves them through a REST API and
a search page.

![Search page showing backend jobs in Egypt](docs/screenshot.jpg)

## Highlights

- **18 sources covering 10 countries:** Workable's job search (one per
  country), company job boards on 5 different applicant tracking systems,
  and the Remote OK API. The last run collected ~3,400 jobs, about 800 of
  them tech. Saudi Arabia and the UAE are still missing because Workable's
  daily request quota cut that run short (see the roadmap). One scrape takes
  about 6 minutes, and that's deliberate: the scrapers are rate limited per
  host.
- **Full-text search** with filters for country, city, seniority, company,
  skill and remote work, built on Postgres, with no separate search engine.
- **Market insights for any search:** which skills the matching jobs ask
  for, who's hiring most, and how they split by seniority. 56 skills are
  detected with patterns tuned against real postings, including business
  tools, certifications and spoken languages. English and Arabic are among
  the most requested skills in the region.
- **Tech jobs by default, everything on request.** Each job is classified
  as tech or not: clear titles decide ("Sales Engineer" and "Site Engineer"
  aren't tech, "Site Reliability Engineer" is), and vague ones like
  "Specialist" fall back to the skills the job asks for. Non-tech jobs stay
  stored, and the search page shows them with one checkbox.
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

## Project layout

```
cmd/scraper/                     collect jobs from every source once
cmd/api/                         HTTP server: REST API and search page
sources.yaml                     which countries and companies to scrape
internal/job/                    the unified Job type, seniority, country and skill detection
internal/scraper/                Scraper interface, concurrent runner, rate-limited HTTP client, text cleanup
internal/scraper/ats/            company job boards: Greenhouse, Ashby, Workable, SmartRecruiters, Recruitee
internal/scraper/workablesearch/ Workable's cross-company job search, one country at a time
internal/scraper/remoteok/       Remote OK API
internal/store/                  PostgreSQL: migrations, upserts, search, stats
internal/api/                    handlers, validation, rate limiting, logging
internal/web/                    the search page (embedded HTML, CSS, JS)
```

Sources are grouped by what you configure to add more of them:

- **`ats/`** holds one file per applicant tracking system. Each company in
  `sources.yaml` names its system and account, and gets its own scraper.
- **`workablesearch/`** is one scraper per country. It's Workable's public
  job search across every company that hires through Workable, which is why
  Workable appears twice: `ats/workable.go` reads one company's board, this
  searches them all.
- **`remoteok/`** is a single fixed feed with nothing to configure.

## Adding a source

A new applicant tracking system, say Workday, is a new file in
`internal/scraper/ats/`:

1. Write a type that embeds `base` and implements `Scrape`, mapping the
   system's JSON onto `job.Job`. `scraper.CleanText`, `HTMLToText` and
   `ParseTime` handle the usual mess in raw fields, and `Normalize` fills in
   country, seniority and skills afterwards.
2. Add a `case` for it in `ats.New`.
3. Add a test that runs it against a local fake server serving a trimmed
   copy of a real response, like the other systems' tests in `ats_test.go`.
4. List companies under it in `sources.yaml`.

A source that isn't one company's board, like another cross-company search,
gets its own package next to `workablesearch/` and is wired up in
`cmd/scraper/main.go`. There it also declares whether its feed lists every
open job. Only then is it safe to close the jobs it stops returning.

## Running locally

With Docker only:

```sh
docker compose up -d --build        # start PostgreSQL, the API and the daily scraper
docker compose run --rm scraper     # collect jobs right now instead of waiting (~6 minutes)
```

Or with Go 1.27+ for development, using Docker just for the database:

```sh
docker compose up -d postgres       # start PostgreSQL
go run ./cmd/scraper                # collect jobs from every source once (~6 minutes)
go run ./cmd/api                    # start the server
```

Then open **http://localhost:8080**.

The [Dockerfile](Dockerfile) builds both programs into one small image (a
multi-stage build onto distroless, running as a non-root user). The API is
its default command.

The scraper runs once and exits, unless it's given a time of day:
`-daily-at 03:00` keeps it running and scrapes every day at 03:00 UTC.
That's how the `scheduler` service in [docker-compose.yml](docker-compose.yml)
runs it. A failed run is logged and the next day's still happens.

| Variable | Default |
|---|---|
| `DATABASE_URL` | `postgres://jobs:jobs@localhost:5432/jobs?sslmode=disable` |
| `ADDR` | `:8080` |
| `CLIENT_IP_HEADER` | unset; behind a reverse proxy, the header it puts the client's IP in |

To run it on a public server with HTTPS, see
[Deploying to AWS](docs/deploy-aws.md).

## API

### `GET /jobs`: search open jobs

| Parameter | Example | Notes |
|---|---|---|
| `q` | `backend engineer` | Full-text search over title, company and description. Supports `"exact phrases"` and `-exclusions`. Title matches rank highest. |
| `level` | `senior,lead` | Any of `intern`, `junior`, `mid`, `senior`, `lead`, `executive` |
| `country` | `EG,SA` or `saudi arabia` | Codes or names |
| `location` | `riyadh` | Substring of the location text |
| `company` | `careem` | Substring of the company name |
| `skill` | `python,sql` | Jobs that ask for all of these skills |
| `tech` | `true` | Only tech jobs (`false` for only non-tech). The search page sets it by default |
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

### `GET /stats`: insights about a search

Takes the same filters as `/jobs` and describes all the matching jobs, not
just one page. `share` is the fraction of those jobs that mention a skill.

```sh
curl "localhost:8080/stats?q=engineer&country=EG"
```

```json
{
  "total": 481,
  "levels": [
    { "level": "intern", "count": 7 },
    { "level": "junior", "count": 16 },
    { "level": "mid", "count": 270 },
    { "level": "senior", "count": 142 },
    { "level": "lead", "count": 29 },
    { "level": "executive", "count": 17 }
  ],
  "top_companies": [
    { "company": "SSC HR Solutions", "count": 60 },
    { "company": "Advansys", "count": 24 }
  ],
  "top_skills": [
    { "skill": "English", "category": "spoken language", "count": 135, "share": 0.281 },
    { "skill": "Python", "category": "programming language", "count": 90, "share": 0.187 },
    { "skill": "SQL", "category": "programming language", "count": 73, "share": 0.152 }
  ]
}
```

Levels always come back in order with zeros included, so a chart gets the
same axis every time. The four counts run as one batch: a single round trip
to the database.

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

**Detection rules can be re-applied.** Level, country, skills, tech and
placeholder companies are all derived from the scraped text, and the rules
keep improving. `go run ./cmd/scraper -renormalize` re-runs them over every
stored job and saves what changed, without re-scraping any source.

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
| "Go" means the language in "Python, Go, Rust" but not in "Go-Live", "Go-to-Market" or "Go the extra mile" | Go only counts inside a list of technologies or after "in"/"with"; the test cases are phrases from real postings |
| "Excel in a fast-paced team" isn't a spreadsheet skill, and "react quickly" isn't React | Verb phrases are removed before matching, and words with an everyday meaning must be capitalized |
| 361 unrelated Egyptian jobs all listed their employer as "Company" | Placeholder names are stored as unknown, so they don't top the "who's hiring" list |
| "Engineer" is 76 sales engineers, site engineers and BIM engineers before it's software | Non-tech job words are checked before tech ones, and "site" only counts as "site engineer", so Site Reliability Engineers stay tech |

## Testing

```sh
go test ./...
```

GitHub Actions runs formatting checks, `go vet`, and the full test suite
with the race detector on every push, against a real Postgres
([workflow](.github/workflows/ci.yml)).

Unit tests need no network or database. Scrapers run against local fake
servers that serve trimmed copies of real responses, and the API handlers
run against a fake store.

The store's integration tests run real SQL against a separate test database:

```sh
docker compose exec postgres createdb -U jobs jobs_test
TEST_DATABASE_URL="postgres://jobs:jobs@localhost:5432/jobs_test?sslmode=disable" go test ./internal/store
```

## Roadmap

- [ ] Fit the Workable searches within its daily request quota (larger pages, or spreading countries across days)
- [x] Run the scraper on a daily schedule
- [ ] Workday-hosted career sites, for large employers like airlines, banks and energy companies
- [x] Insights for any search: in-demand skills, top hiring companies, seniority split
- [ ] How long jobs stay open (needs a few weeks of daily scrapes first)
- [x] CI with GitHub Actions
- [x] Dockerfile for the app
- [ ] Deploy a public demo

## Data sources

Job data from [Remote OK](https://remoteok.com) is used under their API
terms, which require crediting them and linking back to each posting. Other
jobs come from the public job feeds of each company's applicant tracking
system and from Workable's public job search. Every job links back to its
original posting.

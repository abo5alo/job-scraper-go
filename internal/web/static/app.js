// Search page logic. Plain JavaScript, no framework: it reads the form,
// calls GET /jobs and GET /stats, and renders the results.

const LIMIT = 20;
const FIELDS = ["q", "location", "country", "level", "skill"];

const form = document.getElementById("search");
const statusEl = document.getElementById("status");
const resultsEl = document.getElementById("results");
const pagerEl = document.getElementById("pager");
const prevBtn = document.getElementById("prev");
const nextBtn = document.getElementById("next");
const pageInfo = document.getElementById("page-info");
const remoteBox = document.getElementById("remote");
const skillInput = document.getElementById("skill");
const skillChip = document.getElementById("skill-chip");
const insightsEl = document.getElementById("insights");
const topSkillsEl = document.getElementById("top-skills");
const topCompaniesEl = document.getElementById("top-companies");

const regionNames = new Intl.DisplayNames(["en"], { type: "region" });
const relativeTime = new Intl.RelativeTimeFormat("en", { numeric: "auto" });

let page = 1;
let inFlight = null; // lets a new search cancel one still loading

function countryName(code) {
  try {
    return regionNames.of(code);
  } catch {
    return code;
  }
}

// The form's current state as query parameters, skipping empty fields.
function formParams() {
  const params = new URLSearchParams();
  for (const name of FIELDS) {
    const value = document.getElementById(name).value.trim();
    if (value) params.set(name, value);
  }
  if (remoteBox.checked) params.set("remote", "true");
  return params;
}

// Restore the form from the address bar, so a search can be shared or reloaded.
function restoreForm() {
  const params = new URLSearchParams(location.search);
  for (const name of FIELDS) {
    document.getElementById(name).value = params.get(name) ?? "";
  }
  remoteBox.checked = params.get("remote") === "true";
  page = Math.max(1, parseInt(params.get("page"), 10) || 1);
}

async function loadCountries() {
  try {
    const res = await fetch("/countries");
    if (!res.ok) return;
    const select = document.getElementById("country");
    for (const c of await res.json()) {
      const option = document.createElement("option");
      option.value = c.code;
      option.textContent = `${countryName(c.code)} (${c.count})`;
      select.append(option);
    }
  } catch {
    // The page still works without the country list.
  }
}

async function search() {
  const params = formParams();
  if (page > 1) params.set("page", page);
  history.replaceState(null, "", params.size ? `?${params}` : location.pathname);

  showSkillFilter();
  inFlight?.abort();
  const request = (inFlight = new AbortController());
  statusEl.textContent = "Searching…";

  // Stats describe the whole search, not one page, so they get the filters
  // without paging. They load alongside the jobs; if they fail, the page
  // just goes without them.
  const stats = fetch(`/stats?${formParams()}`, { signal: request.signal })
    .then((r) => (r.ok ? r.json() : null))
    .catch(() => null);

  params.set("limit", LIMIT);
  let res, body;
  try {
    res = await fetch(`/jobs?${params}`, { signal: request.signal });
    body = await res.json();
  } catch (err) {
    if (err.name === "AbortError") return; // replaced by a newer search
    statusEl.textContent = "Couldn't reach the server. Is it running?";
    return;
  }

  if (!res.ok) {
    statusEl.textContent = body.error ?? "Something went wrong.";
    resultsEl.replaceChildren();
    pagerEl.hidden = true;
    insightsEl.hidden = true;
    return;
  }
  render(body);

  const st = await stats;
  if (request === inFlight) renderStats(st); // skip if a newer search started
}

function renderStats(st) {
  insightsEl.hidden = !st || st.total === 0;
  if (insightsEl.hidden) return;

  // A skill that's already a filter would show as 100%, which says nothing.
  const active = selectedSkills();
  const skills = st.top_skills.filter((s) => !active.includes(s.skill)).slice(0, 10);
  topSkillsEl.replaceChildren(
    ...skills.map((s) => {
      const button = el("button", null, s.skill);
      button.type = "button";
      button.title = `${s.count.toLocaleString()} of ${st.total.toLocaleString()} jobs · click to filter`;
      button.append(el("span", "share", `${Math.round(s.share * 100)}%`));
      button.addEventListener("click", () => {
        skillInput.value = [...active, s.skill].join(",");
        page = 1;
        search();
      });
      const li = document.createElement("li");
      li.append(button);
      return li;
    }),
  );

  const companies = st.top_companies.slice(0, 5).map((c) => `${c.company} (${c.count})`);
  topCompaniesEl.textContent = companies.length ? `Hiring most: ${companies.join(" · ")}` : "";
}

function selectedSkills() {
  return skillInput.value.split(",").filter(Boolean);
}

// The skill filter has no form control of its own, so it shows as a chip
// that clears it when clicked.
function showSkillFilter() {
  const skills = selectedSkills();
  skillChip.hidden = skills.length === 0;
  skillChip.textContent = `${skills.join(" + ")} ✕`;
  skillChip.setAttribute("aria-label", `Remove skill filter: ${skills.join(", ")}`);
}

function render({ total, jobs }) {
  statusEl.textContent =
    total === 0 ? "No jobs match. Try fewer filters." : `${total.toLocaleString()} ${total === 1 ? "job" : "jobs"}`;
  resultsEl.replaceChildren(...jobs.map(jobCard));

  const pages = Math.ceil(total / LIMIT);
  pagerEl.hidden = pages <= 1;
  pageInfo.textContent = `Page ${page} of ${pages}`;
  prevBtn.disabled = page <= 1;
  nextBtn.disabled = page >= pages;
}

// Every piece of job data is set with textContent, never innerHTML. The data
// comes from third-party sites, and textContent can't be tricked into
// running HTML or scripts.
function jobCard(job) {
  const li = el("li", "card");

  const title = el("h2", "title");
  const link = el("a", null, job.title);
  if (isSafeURL(job.url)) {
    link.href = job.url;
    link.target = "_blank";
    link.rel = "noopener noreferrer";
  }
  title.append(link);

  // Prefer the company's own wording ("Jeddah, Saudi Arabia"); fall back to
  // the detected country when the location is blank.
  const place = job.location || (job.country ? countryName(job.country) : "");
  const company = job.company || "Undisclosed company";
  const meta = el("p", "meta", [company, place].filter(Boolean).join(" · "));

  const tags = el("div", "tags");
  tags.append(el("span", `tag level-${job.level}`, levelLabel(job.level)));
  if (job.remote) tags.append(el("span", "tag remote", "Remote"));
  if (job.salary) tags.append(el("span", "tag salary", formatSalary(job.salary)));
  for (const skill of (job.skills ?? []).slice(0, 5)) tags.append(el("span", "tag skill", skill));
  if (job.posted_at) tags.append(el("span", "tag date", timeAgo(job.posted_at)));

  const desc = el("p", "desc", job.description ?? "");

  const apply = el("a", "apply", "View & apply →");
  if (isSafeURL(job.url)) {
    apply.href = job.url;
    apply.target = "_blank";
    apply.rel = "noopener noreferrer";
  }

  li.append(title, meta, tags, desc, apply);
  return li;
}

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text) node.textContent = text;
  return node;
}

// Only http(s) links. A "javascript:" URL in a job's link would run code
// when clicked.
function isSafeURL(url) {
  try {
    return ["http:", "https:"].includes(new URL(url).protocol);
  } catch {
    return false;
  }
}

function levelLabel(level) {
  return {
    intern: "Intern",
    junior: "Junior",
    mid: "Mid-level",
    senior: "Senior",
    lead: "Lead",
    executive: "Executive",
  }[level] ?? level;
}

function formatSalary({ min, max, currency }) {
  const fmt = new Intl.NumberFormat("en", {
    style: "currency",
    currency: currency || "USD",
    notation: "compact",
    maximumFractionDigits: 0,
  });
  if (min && max) return `${fmt.format(min)}–${fmt.format(max)} / yr`;
  return `${fmt.format(min ?? max)} / yr`;
}

function timeAgo(iso) {
  const days = Math.round((new Date(iso) - Date.now()) / 86_400_000);
  if (days > -1) return "Posted today";
  if (days > -30) return `Posted ${relativeTime.format(days, "day")}`;
  if (days > -365) return `Posted ${relativeTime.format(Math.round(days / 30), "month")}`;
  return `Posted ${relativeTime.format(Math.round(days / 365), "year")}`;
}

// Events: a new search or filter goes back to page 1.
form.addEventListener("submit", (e) => {
  e.preventDefault();
  page = 1;
  search();
});
for (const id of ["country", "level", "remote"]) {
  document.getElementById(id).addEventListener("change", () => {
    page = 1;
    search();
  });
}
skillChip.addEventListener("click", () => {
  skillInput.value = "";
  page = 1;
  search();
});
prevBtn.addEventListener("click", () => {
  page--;
  search();
  scrollTo({ top: 0, behavior: "smooth" });
});
nextBtn.addEventListener("click", () => {
  page++;
  search();
  scrollTo({ top: 0, behavior: "smooth" });
});

// Countries load first so a country in the address bar can be selected.
await loadCountries();
restoreForm();
search();

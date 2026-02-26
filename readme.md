# Citadel: Advanced DNS Proxy & Parental Controls

Welcome to **Citadel**, your comprehensive DNS proxy service designed with device- and domain-aware parental controls. Citadel empowers families and network administrators to take back control of internet access, offering fine-grained budgeting, time-based windows, and a robust rule engine—all functioning locally without the need for cloud subscriptions. 

With its rich [Web Dashboard](./citadel-web) and configurable policy engine, Citadel bridges the gap between complex router settings and intuitive user control.

![Citadel Architecture](https://img.shields.io/badge/Architecture-Go_%7C_Next.js-blue?style=flat-square)
![Status](https://img.shields.io/badge/Status-Active_Development-brightgreen?style=flat-square)

---
## Quickstart
- Build: `make build`
- Validate sample configs: `make validate-config`
- Run locally with a block rule: `./bin/dnsproxy configs/minimal.json`
  - In another shell: `dig @127.0.0.1 -p 2053 blocked.test` (should be blocked)
  - Allow-list traffic by adjusting `Hosts`/`Domains` in `configs/minimal.json`
- Explain a decision without running the server:
  - `./bin/dnsproxy -xhost=127.0.0.1 -xdomain=blocked.test -xtime=2024-01-01T12:00:00Z configs/minimal.json`
  - Outputs matched patterns, timespan evaluation, and final action.

## Policy store and budgets
- Persistent store: SQLite-backed `data/policy.db` by default (override with `-policy=<path>`; use `.json` to force the JSON fallback). Stores admin auth, users, devices, domain rules, sessions, per-day usage totals, settings, and audit events (schema versioned).
- Factory reset: run with `-factory-reset` to wipe the policy store safely; it is recreated empty on start.
- Per-user budgets: multiple devices share a user’s daily budget; idle timeout defaults to 10 minutes and closes sessions with no traffic. Daily reset uses the configured timezone (default system local).
- Allow windows and per-user domain allow/block patterns are combined with the config-driven rules engine; block reasons (budget, window, policy rule, config rule) are logged for explainability.

## Config examples
- `configs/minimal.json`: loopback-only with a simple blocked domain pattern.
- `configs/static-devices-only.json`: uses static device mappings, no router.
- `configs/unifi.json`: Unifi-powered device discovery and time-window rules.

Validate any config against the schema: `make validate-config CONFIG=path/to/config.json`

## Rule precedence (deterministic)
- Host rules evaluated before domain rules.
- Most specific pattern wins (exact > fewer wildcards > longer pattern; ties resolved alphabetically); within a matched host, rules are evaluated in order and the first non-None action wins.
- Domain selection follows the same specificity rule; then the most specific host within that domain is used.
- DefaultRule is returned only if nothing matches.

## Time spans
- Formats: `HH:MM-HH:MM`, `mon-fri@HH:MM-HH:MM`, `mon,tue@22:00-06:00|tz=Europe/Stockholm`.
- Cross-midnight windows supported (e.g., `22:00-06:00`).
- Weekdays are optional; timezone defaults to system local but can be set per timespan.

## Development & Testing
- Run tests: `make test`
- Build binary into `bin/`: `make build`

### Docker Testing Environment
We extensively test Citadel's core filtering system using containerized environments. By running Citadel within isolated Docker containers, we reliably mock DNS queries, simulate multiple devices (as distinct container IPs), and validate time-based window policies effectively.
- **Why Docker?** It ensures consistent cross-platform behaviors and helps rigorously evaluate edge-cases corresponding to complex session-budget rules before they hit a local network.
- See detailed docker-compose configurations in the integration tests suite.

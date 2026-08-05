# Load smoke tests

The read scenario measures the authenticated meeting-detail hot path without mutating
user data. It is deliberately bounded and is not a production-capacity claim.

Run it against a local API:

```powershell
docker run --rm `
  -v "${PWD}/load/k6:/scripts:ro" `
  -e BASE_URL=http://host.docker.internal:8080 `
  -e EMAIL=live.owner@ryden.local `
  -e PASSWORD=Ryden-demo-2026! `
  -e VUS=10 `
  -e DURATION=20s `
  grafana/k6 run /scripts/meeting-read.js
```

Defaults produce roughly 50 requests per second because each of 10 virtual users waits
200 ms between reads. Override `MEETING_ID` to select a meeting, and `PAUSE_SECONDS` to
change request pressure.

Record the host CPU/RAM, PostgreSQL version, container limits, dataset size, request
rate, p95/p99 latency, error rate, and database-pool metrics with every result. Do not
run the scenario against production without an explicit maintenance window and traffic
budget.

`multi-user-fixed.js` creates a disposable fixed meeting and one account per virtual
user. On the first iteration all users join the same invitation concurrently; every
iteration then changes attendance, revotes in one poll, claims or releases one shared
preparation item, and loads the same bounded meeting, poll, attendance, preparation,
and note summaries as the web client. This deliberately exercises row-lock contention
and live-reload read amplification instead of reporting a read-only number as overall
capacity.

```powershell
docker run --rm `
  -v "${PWD}/load/k6:/scripts:ro" `
  -e BASE_URL=http://host.docker.internal:8080 `
  -e VUS=20 `
  -e DURATION=30s `
  grafana/k6 run /scripts/multi-user-fixed.js
```

Treat thresholds as regression guards for the measured environment, not as a universal
capacity claim. Record the environment and results outside the repository when using
these scenarios for a real deployment.

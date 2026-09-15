# Redis Strategy: Upstash vs Leapcell

Since we are deploying on Leapcell, the choice of Redis provider significantly impacts performance and cost.

## 1. Upstash Redis (Free Tier)

**Pros:**

- **Generous Free Tier:** 10,000 requests/day free.
- **Easy Setup:** HTTP/REST based, works everywhere.
- **Persistence:** Good data safety.

**Cons:**

- **Latency:** Upstash runs in standard AWS/GCP regions. If Leapcell runs elsewhere, the round-trip time (RTT) for every queue check adds up.
- **Polling Cost:** Our worker polls every 1 second.
  - `1 request/sec` _ `60` _ `60` \* `24` = `86,400` requests/day.
  - **Alert:** A defined polling interval of `1s` **exceeds the Upstash Free Tier** (10k limit) quickly if run 24/7.
  - _Mitigation:_ In "Monolith Mode" on Leapcell, the container sleeps, so we only poll when active.

## 2. Leapcell Redis

**Pros:**

- **Zero Latency:** Running in the same cluster/context as the Service.
- **Integrated Billing:** Part of the Leapcell ecosystem.

**Cons:**

- **Pricing Structure:** "Each write command counts as 10 requests".
- **Quota:** Shared with other services in the 3GB-hour limit? Needs verification.

## Recommendation

**For Hobby/MVP:**
**Stick with Upstash (for now)** but be careful with the Polling Interval.

- We set `TaskCheckInterval` to `1s`.
- If the app is "Scale-to-Zero" (sleeps when unused), this is fine.
- If the app gets traffic 24/7, you will burn the 10k daily limit in ~3 hours.

**Optimized Config for Free Tier:**

- Increase check interval to `5s` (default) if you notice high usage.
- Or, use **Leapcell Redis** if your "Serverless Duration" allows, as it removes the network latency bottleneck.

**Verdict:**
Use **Upstash** with **TLS enabled** (configured in Tomoshibi). It's the standard for serverless Redis. The Monolith architecture saves you because the worker stops polling when the container spins down.

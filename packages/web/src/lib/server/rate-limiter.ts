import { RateLimiterMemory } from 'rate-limiter-flexible';

// 1. GLOBAL SAFETY LIMIT
// Cap total traffic to avoid platform bans.
// 80 requests per minute total.
export const globalLimiter = new RateLimiterMemory({
	points: 80,
	duration: 60,
	keyPrefix: 'gl'
});

// 2. PER-IP FAIRNESS LIMIT
// 60 requests per 5 minutes per user.
// Averaging 12 req/min, but allows flexible usage over 5 mins.
export const ipLimiter = new RateLimiterMemory({
	points: 60,
	duration: 5 * 60,
	keyPrefix: 'ip'
});

// 3. BURST PROTECTION
// 5 requests per 2 seconds.
// Penalty: 12 hours block if exceeded.
export const burstLimiter = new RateLimiterMemory({
	points: 5,
	duration: 2,
	blockDuration: 12 * 60 * 60, // 12 hours in seconds
	keyPrefix: 'burst'
});

// 4. DAILY SEARCH LIMIT
// Global cap on web searches per day to protect API quota.
// TODO: Change to 10 for production.
export const dailySearchLimiter = new RateLimiterMemory({
	points: 2, // Testing: 2 searches/day. Production: 10.
	duration: 24 * 60 * 60, // 24 hours
	keyPrefix: 'daily_search'
});

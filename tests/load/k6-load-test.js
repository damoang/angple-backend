/**
 * k6 Load Test for Angple Backend
 *
 * Usage:
 *   # Quick smoke test
 *   k6 run --env BASE_URL=http://localhost:8081 tests/load/k6-load-test.js
 *
 *   # Full load test (20k concurrent)
 *   k6 run --env BASE_URL=http://localhost:8081 --env SCENARIO=full tests/load/k6-load-test.js
 *
 *   # CI mode with thresholds
 *   k6 run --env BASE_URL=http://localhost:8081 --env SCENARIO=ci tests/load/k6-load-test.js
 */

import http from "k6/http";
import { check, sleep } from "k6";
import { Rate, Trend } from "k6/metrics";

// Custom metrics
const errorRate = new Rate("errors");
const postListDuration = new Trend("post_list_duration", true);
const postDetailDuration = new Trend("post_detail_duration", true);
const commentsDuration = new Trend("comments_duration", true);
const authLoginDuration = new Trend("auth_login_duration", true);

const BASE_URL = __ENV.BASE_URL || "http://localhost:8081";
const SCENARIO = __ENV.SCENARIO || "smoke";

// Thresholds: P99 <= 100ms, error rate < 1%
export const options = {
  thresholds: {
    http_req_duration: ["p(95)<200", "p(99)<500"],
    post_list_duration: ["p(99)<100"],
    post_detail_duration: ["p(99)<100"],
    comments_duration: ["p(99)<100"],
    errors: ["rate<0.01"],
  },
  scenarios: getScenarios(),
};

function getScenarios() {
  switch (SCENARIO) {
    case "full":
      return {
        ramp_up: {
          executor: "ramping-vus",
          startVUs: 0,
          stages: [
            { duration: "30s", target: 100 },
            { duration: "1m", target: 500 },
            { duration: "2m", target: 2000 },
            { duration: "5m", target: 5000 },
            { duration: "2m", target: 0 },
          ],
          gracefulRampDown: "30s",
        },
      };
    case "ci":
      return {
        ci_test: {
          executor: "ramping-vus",
          startVUs: 0,
          stages: [
            { duration: "10s", target: 20 },
            { duration: "30s", target: 50 },
            { duration: "10s", target: 0 },
          ],
          gracefulRampDown: "10s",
        },
      };
    default:
      // smoke
      return {
        smoke: {
          executor: "constant-vus",
          vus: 5,
          duration: "30s",
        },
      };
  }
}

export default function () {
  const actions = [
    { weight: 10, fn: browsePosts },
    { weight: 5, fn: viewPost },
    { weight: 3, fn: viewComments },
    { weight: 2, fn: listBoards },
    { weight: 1, fn: listUsers },
    { weight: 1, fn: healthCheck },
  ];

  // Weighted random selection
  const totalWeight = actions.reduce((sum, a) => sum + a.weight, 0);
  let rand = Math.random() * totalWeight;
  for (const action of actions) {
    rand -= action.weight;
    if (rand <= 0) {
      action.fn();
      break;
    }
  }

  sleep(Math.random() * 2 + 0.5); // 0.5-2.5s between requests
}

function browsePosts() {
  const res = http.get(`${BASE_URL}/api/v2/boards/free/posts?page=1&per_page=20`);
  postListDuration.add(res.timings.duration);
  const ok = check(res, {
    "post list status 200": (r) => r.status === 200,
  });
  errorRate.add(!ok);
}

function viewPost() {
  const res = http.get(`${BASE_URL}/api/v2/boards/free/posts/1`);
  postDetailDuration.add(res.timings.duration);
  const ok = check(res, {
    "post detail status 200 or 404": (r) => r.status === 200 || r.status === 404,
  });
  errorRate.add(!ok);
}

function viewComments() {
  const res = http.get(`${BASE_URL}/api/v2/boards/free/posts/1/comments`);
  commentsDuration.add(res.timings.duration);
  const ok = check(res, {
    "comments status 200 or 404": (r) => r.status === 200 || r.status === 404,
  });
  errorRate.add(!ok);
}

function listBoards() {
  const res = http.get(`${BASE_URL}/api/v2/boards`);
  const ok = check(res, {
    "boards status 200": (r) => r.status === 200,
  });
  errorRate.add(!ok);
}

function listUsers() {
  const res = http.get(`${BASE_URL}/api/v2/users`);
  const ok = check(res, {
    "users status 200": (r) => r.status === 200,
  });
  errorRate.add(!ok);
}

function healthCheck() {
  const res = http.get(`${BASE_URL}/health`);
  check(res, {
    "health ok": (r) => r.status === 200,
  });
}

// Auth login scenario (separate, only if credentials are available)
export function authScenario() {
  const payload = JSON.stringify({
    username: "testuser",
    password: "testpassword",
  });
  const params = { headers: { "Content-Type": "application/json" } };
  const res = http.post(`${BASE_URL}/api/v2/auth/login`, payload, params);
  authLoginDuration.add(res.timings.duration);
  const ok = check(res, {
    "login responds": (r) => r.status === 200 || r.status === 401,
  });
  errorRate.add(!ok);
}

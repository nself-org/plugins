// k6 synthetic monitoring runner for nSelf.
// Executes flow files and exports results as Prometheus metrics via OTel.
//
// Usage: k6 run --out experimental-prometheus-rw k6-runner.js
//
// Env vars:
//   SYNTHETIC_FLOWS_DIR — path to flow YAML directory (default: ./flows)
//   K6_PROMETHEUS_RW_SERVER_URL — Prometheus remote-write URL
//   K6_PROMETHEUS_RW_TREND_AS_NATIVE_HISTOGRAM — set to true

import http from "k6/http";
import ws from "k6/ws";
import { check, sleep } from "k6";
import { Counter, Gauge, Trend } from "k6/metrics";

// Custom metrics exported to Prometheus.
const syntheticSuccess = new Counter("synthetic_check_success");
const syntheticFailure = new Counter("synthetic_check_failure");
const syntheticDuration = new Trend("synthetic_check_duration_seconds");

// Flow definitions (compiled from YAML at build time).
const flows = {
  login: {
    fn: loginFlow,
    tags: { flow: "login" },
  },
  chat: {
    fn: chatFlow,
    tags: { flow: "chat" },
  },
  receive: {
    fn: receiveFlow,
    tags: { flow: "receive" },
  },
  "email-triage": {
    fn: emailTriageFlow,
    tags: { flow: "email-triage" },
  },
};

export const options = {
  scenarios: {
    synthetic: {
      executor: "constant-arrival-rate",
      rate: 1,
      timeUnit: `${__ENV.SYNTHETIC_INTERVAL_SECONDS || 300}s`,
      duration: "24h",
      preAllocatedVUs: 4,
      maxVUs: 8,
    },
  },
  thresholds: {
    synthetic_check_success: ["count>0"],
  },
};

export default function () {
  for (const [name, flow] of Object.entries(flows)) {
    const start = Date.now();
    let success = false;
    try {
      success = flow.fn();
    } catch (e) {
      console.error(`Flow ${name} failed: ${e.message}`);
    }
    const elapsed = (Date.now() - start) / 1000;
    syntheticDuration.add(elapsed, flow.tags);
    if (success) {
      syntheticSuccess.add(1, flow.tags);
    } else {
      syntheticFailure.add(1, flow.tags);
    }
  }
}

function getToken() {
  const res = http.post(
    `${__ENV.NSELF_AUTH_URL}/signin/email-password`,
    JSON.stringify({
      email: __ENV.SYNTHETIC_USER_EMAIL,
      password: __ENV.SYNTHETIC_USER_PASSWORD,
    }),
    { headers: { "Content-Type": "application/json" } }
  );
  if (res.status !== 200) return null;
  return JSON.parse(res.body).session.accessToken;
}

function loginFlow() {
  const token = getToken();
  return check(token, { "login: token received": (t) => t !== null });
}

function chatFlow() {
  const token = getToken();
  if (!token) return false;

  const chatRes = http.post(
    `${__ENV.NSELF_API_URL}/api/chat`,
    JSON.stringify({ message: "synthetic health check ping", stream: false }),
    {
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      timeout: "15s",
    }
  );

  return check(chatRes, {
    "chat: status 200": (r) => r.status === 200,
  });
}

function receiveFlow() {
  const token = getToken();
  if (!token) return false;

  let received = false;
  ws.connect(
    `${__ENV.NSELF_WS_URL}/ws/chat`,
    { headers: { Authorization: `Bearer ${token}` } },
    function (socket) {
      socket.on("message", function (msg) {
        if (msg.includes("pong")) received = true;
        socket.close();
      });
      socket.send('{"type":"ping","id":"synthetic-check"}');
      socket.setTimeout(function () {
        socket.close();
      }, 1000);
    }
  );

  return received;
}

function emailTriageFlow() {
  const token = getToken();
  if (!token) return false;

  const sendRes = http.post(
    `${__ENV.NSELF_API_URL}/api/mux/test-email`,
    JSON.stringify({
      subject: `synthetic-check-${Date.now()}`,
      body: "Synthetic monitoring check. Classify and triage.",
    }),
    {
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
    }
  );
  if (sendRes.status !== 200) return false;

  const emailId = JSON.parse(sendRes.body).id;

  // Poll for classification (max 30s).
  for (let i = 0; i < 6; i++) {
    sleep(5);
    const checkRes = http.get(
      `${__ENV.NSELF_API_URL}/api/mux/emails/${emailId}`,
      { headers: { Authorization: `Bearer ${token}` } }
    );
    if (checkRes.status === 200) {
      const data = JSON.parse(checkRes.body);
      if (data.classification && data.action_triggered) return true;
    }
  }
  return false;
}

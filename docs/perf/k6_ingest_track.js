import http from "k6/http";
import { check, sleep } from "k6";

const BASE_URL = __ENV.BASE_URL || "http://127.0.0.1:8080";
const PROJECT_ID = __ENV.PROJECT_ID || "1";
const PROJECT_KEY = __ENV.PROJECT_KEY || "";
const BATCH_SIZE = parseInt(__ENV.BATCH_SIZE || "50", 10);
const SLEEP_MS = parseInt(__ENV.SLEEP_MS || "0", 10);

export default function () {
  const url = `${BASE_URL}/api/${PROJECT_ID}/track/`;
  const ts = new Date().toISOString();

  const items = [];
  for (let i = 0; i < BATCH_SIZE; i++) {
    items.push({
      name: "k6_event",
      timestamp: ts,
      properties: { i, source: "k6" },
    });
  }

  const params = {
    headers: {
      "Content-Type": "application/json",
      "X-Project-Key": PROJECT_KEY,
    },
    timeout: "30s",
  };
  const res = http.post(url, JSON.stringify(items), params);
  check(res, { "status is 202": (r) => r.status === 202 });

  if (SLEEP_MS > 0) sleep(SLEEP_MS / 1000);
}


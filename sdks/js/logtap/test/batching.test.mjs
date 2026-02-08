import assert from "node:assert/strict";
import http from "node:http";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { LogtapClient } from "../index.js";

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function waitFor(cond, timeoutMs = 1500) {
  const started = Date.now();
  while (Date.now() - started < timeoutMs) {
    if (cond()) return;
    await sleep(25);
  }
  throw new Error("timeout");
}

// Scenario 1: batching by minBatchSize (no timer).
{
  /** @type {{count: number}} */
  const state = { count: 0 };

  const srv = http.createServer((req, res) => {
    if (req.method === "POST" && (req.url === "/api/1/logs/" || req.url === "/api/1/track/")) {
      state.count += 1;
      res.writeHead(202);
      res.end();
      return;
    }
    res.writeHead(404);
    res.end();
  });
  await new Promise((resolve) => srv.listen(0, "127.0.0.1", resolve));
  const { port } = srv.address();
  const baseUrl = `http://127.0.0.1:${port}`;

  const client = new LogtapClient({
    baseUrl,
    projectId: 1,
    projectKey: "pk_test",
    flushIntervalMs: -1,
    minBatchSize: 3,
    gzip: false,
  });

  client.info("m1");
  client.info("m2");
  await sleep(150);
  assert.equal(state.count, 0);

  client.info("m3");
  await waitFor(() => state.count === 1);

  await client.close();
  await new Promise((resolve) => srv.close(resolve));
}

// Scenario 2: batching by flushIntervalMs (time threshold).
{
  /** @type {{count: number}} */
  const state = { count: 0 };

  const srv = http.createServer((req, res) => {
    if (req.method === "POST" && (req.url === "/api/1/logs/" || req.url === "/api/1/track/")) {
      state.count += 1;
      res.writeHead(202);
      res.end();
      return;
    }
    res.writeHead(404);
    res.end();
  });
  await new Promise((resolve) => srv.listen(0, "127.0.0.1", resolve));
  const { port } = srv.address();
  const baseUrl = `http://127.0.0.1:${port}`;

  const client = new LogtapClient({
    baseUrl,
    projectId: 1,
    projectKey: "pk_test",
    flushIntervalMs: 120,
    minBatchSize: 100,
    gzip: false,
  });

  client.info("m1");
  await waitFor(() => state.count === 1);

  await client.close();
  await new Promise((resolve) => srv.close(resolve));
}

// Scenario 3: immediate events bypass batching (track only).
{
  /** @type {{count: number}} */
  const state = { count: 0 };

  const srv = http.createServer((req, res) => {
    if (req.method === "POST" && req.url === "/api/1/track/") {
      state.count += 1;
      res.writeHead(202);
      res.end();
      return;
    }
    res.writeHead(404);
    res.end();
  });
  await new Promise((resolve) => srv.listen(0, "127.0.0.1", resolve));
  const { port } = srv.address();
  const baseUrl = `http://127.0.0.1:${port}`;

  const client = new LogtapClient({
    baseUrl,
    projectId: 1,
    projectKey: "pk_test",
    flushIntervalMs: -1,
    minBatchSize: 100,
    immediateEvents: ["purchase"],
    gzip: false,
  });

  client.track("signup", { plan: "free" });
  await sleep(120);
  assert.equal(state.count, 0);

  client.track("purchase", { amount: 1 });
  await waitFor(() => state.count === 1);

  await client.close();
  await new Promise((resolve) => srv.close(resolve));
}

// Scenario 4: retry after failure for immediate track (no new items).
{
  /** @type {{count: number, failFirst: boolean}} */
  const state = { count: 0, failFirst: true };

  const srv = http.createServer((req, res) => {
    if (req.method === "POST" && req.url === "/api/1/track/") {
      state.count += 1;
      if (state.failFirst) {
        state.failFirst = false;
        res.writeHead(503);
        res.end();
        return;
      }
      res.writeHead(202);
      res.end();
      return;
    }
    res.writeHead(404);
    res.end();
  });
  await new Promise((resolve) => srv.listen(0, "127.0.0.1", resolve));
  const { port } = srv.address();
  const baseUrl = `http://127.0.0.1:${port}`;

  const client = new LogtapClient({
    baseUrl,
    projectId: 1,
    projectKey: "pk_test",
    flushIntervalMs: -1,
    minBatchSize: 100,
    immediateEvents: ["purchase"],
    gzip: false,
  });

  client.track("purchase", { amount: 1 }); // immediateEvents triggers immediate send
  await waitFor(() => state.count >= 2, 2500);

  await client.close();
  await new Promise((resolve) => srv.close(resolve));
}

// Scenario 5: persistQueue replays after restart (Node file).
{
  const dir = await fs.mkdtemp(path.join(os.tmpdir(), "logtap-js-queue-"));
  const queueFilePath = path.join(dir, "queue.json");

  const failSrv = http.createServer((req, res) => {
    if (req.method === "POST" && (req.url === "/api/1/logs/" || req.url === "/api/1/track/")) {
      res.writeHead(503);
      res.end();
      return;
    }
    res.writeHead(404);
    res.end();
  });
  await new Promise((resolve) => failSrv.listen(0, "127.0.0.1", resolve));
  const failPort = failSrv.address().port;
  const failBaseUrl = `http://127.0.0.1:${failPort}`;

  const c1 = new LogtapClient({
    baseUrl: failBaseUrl,
    projectId: 1,
    flushIntervalMs: -1,
    persistQueue: true,
    persistDebounceMs: 0,
    queueFilePath,
  });
  c1.error("boom");
  await c1.flush();
  await c1.close();
  await new Promise((resolve) => failSrv.close(resolve));

  await fs.stat(queueFilePath);

  /** @type {{count: number}} */
  const state = { count: 0 };
  const okSrv = http.createServer((req, res) => {
    if (req.method === "POST" && req.url === "/api/1/logs/") {
      state.count += 1;
      res.writeHead(202);
      res.end();
      return;
    }
    res.writeHead(404);
    res.end();
  });
  await new Promise((resolve) => okSrv.listen(0, "127.0.0.1", resolve));
  const okPort = okSrv.address().port;
  const okBaseUrl = `http://127.0.0.1:${okPort}`;

  const c2 = new LogtapClient({
    baseUrl: okBaseUrl,
    projectId: 1,
    flushIntervalMs: -1,
    persistQueue: true,
    persistDebounceMs: 0,
    queueFilePath,
  });

  await waitFor(() => state.count >= 1, 2500);
  await c2.close();
  await new Promise((resolve) => okSrv.close(resolve));

  await assert.rejects(() => fs.stat(queueFilePath));
}

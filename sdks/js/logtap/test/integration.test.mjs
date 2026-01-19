import assert from "node:assert/strict";
import { LogtapClient } from "../index.js";

const baseUrl = process.env.LOGTAP_BASE_URL;
const projectIdRaw = process.env.LOGTAP_PROJECT_ID;
const projectKey = process.env.LOGTAP_PROJECT_KEY;
const message = process.env.LOGTAP_TEST_MESSAGE ?? "js-sdk-e2e";
const eventName = process.env.LOGTAP_TEST_EVENT ?? "js-sdk-signup";

assert.ok(baseUrl, "LOGTAP_BASE_URL is required");
assert.ok(projectIdRaw, "LOGTAP_PROJECT_ID is required");
assert.ok(projectKey, "LOGTAP_PROJECT_KEY is required");

const projectId = Number(projectIdRaw);
assert.ok(Number.isFinite(projectId) && projectId > 0, "invalid LOGTAP_PROJECT_ID");

assert.equal(typeof fetch, "function", "Node 18+ global fetch() is required");

const client = new LogtapClient({
  baseUrl,
  projectId,
  projectKey,
  flushIntervalMs: -1,
  gzip: true,
  globalTags: { env: "test" },
});

client.info(message, { k: "v" }, { tags: { req: "1" } });
client.track(eventName, { plan: "pro" }, { tags: { req: "1" } });

await client.flush();
await client.close();

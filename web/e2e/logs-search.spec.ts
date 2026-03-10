import { expect, test } from "@playwright/test";
import { ensureLoggedInAndEnterProject, readRuntimeAuth } from "./helpers";

test("log ingest is searchable on logs page", async ({ page }) => {
  await ensureLoggedInAndEnterProject(page);

  const { apiBase, projectId, token } = await readRuntimeAuth(page);

  const keyName = `e2e-logs-key-${Date.now()}`;
  const createKeyRes = await page.request.post(`${apiBase}/api/projects/${projectId}/keys`, {
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    data: { name: keyName },
  });
  expect(createKeyRes.status()).toBe(200);
  const createKeyBody = await createKeyRes.json();
  const projectKey = String(createKeyBody?.data?.key ?? createKeyBody?.key ?? "");
  expect(projectKey.startsWith("pk_")).toBeTruthy();

  const message = `e2e-log-message-${Date.now()}`;
  const traceID = `e2e-trace-${Date.now()}`;
  const ingestRes = await page.request.post(`${apiBase}/api/${projectId}/logs/`, {
    headers: {
      "Content-Type": "application/json",
      "X-Project-Key": projectKey,
    },
    data: {
      level: "info",
      message,
      trace_id: traceID,
      timestamp: new Date().toISOString(),
      fields: { e2e: "logs-search" },
    },
  });
  expect(ingestRes.status()).toBe(202);

  await page.goto("/logs");
  await expect(page).toHaveURL(/\/logs$/);

  await page.getByPlaceholder("abc123").fill(traceID);
  await page.getByRole("button", { name: "查询" }).click();

  await expect(page.getByText(traceID)).toBeVisible();
  await expect(page.getByText(message)).toBeVisible();
});

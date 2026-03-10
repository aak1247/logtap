import { expect, test } from "@playwright/test";
import { ensureLoggedInAndEnterProject, readRuntimeAuth } from "./helpers";

test("alerts rules test tab previews webhook delivery", async ({ page }) => {
  await ensureLoggedInAndEnterProject(page);

  const { apiBase, projectId, token } = await readRuntimeAuth(page);

  const webhookName = `e2e-webhook-${Date.now()}`;
  const webhookURL = `https://example.com/hook/${Date.now()}`;
  const endpointRes = await page.request.post(`${apiBase}/api/${projectId}/alerts/webhook-endpoints`, {
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    data: {
      name: webhookName,
      url: webhookURL,
    },
  });
  expect(endpointRes.status()).toBe(200);
  const endpointBody = await endpointRes.json();
  const endpointID = Number(endpointBody?.data?.id ?? endpointBody?.id ?? 0);
  expect(endpointID).toBeGreaterThan(0);

  const keyword = `e2e-alert-keyword-${Date.now()}`;
  const ruleName = `e2e-rule-${Date.now()}`;
  const ruleRes = await page.request.post(`${apiBase}/api/${projectId}/alerts/rules`, {
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    data: {
      name: ruleName,
      enabled: true,
      source: "logs",
      match: {
        levels: ["error"],
        messageKeywords: [keyword],
      },
      repeat: {
        windowSec: 60,
        threshold: 1,
        baseBackoffSec: 60,
        maxBackoffSec: 60,
      },
      targets: {
        webhookEndpointIds: [endpointID],
      },
    },
  });
  expect(ruleRes.status()).toBe(200);

  await page.goto("/alerts");
  await expect(page).toHaveURL(/\/alerts$/);

  await page.getByRole("button", { name: "规则测试" }).click();
  await expect(page.getByText("规则测试（dry-run）")).toBeVisible();

  await page.getByPlaceholder("boom!").fill(keyword);
  await page.getByRole("button", { name: /^测试$/ }).click();

  const ruleCard = page.locator("div.rounded-lg", { hasText: ruleName }).first();
  await expect(ruleCard).toBeVisible();
  await expect(ruleCard).toContainText("matched");
  await expect(ruleCard).toContainText("will enqueue");
  await expect(ruleCard).toContainText(webhookURL);
});

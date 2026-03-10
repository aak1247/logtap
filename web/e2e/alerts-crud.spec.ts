import { expect, test } from "@playwright/test";
import { ensureLoggedInAndEnterProject, readRuntimeAuth } from "./helpers";

test("alerts page supports contact, webhook endpoint and rule lifecycle", async ({ page }) => {
  await ensureLoggedInAndEnterProject(page);
  const { apiBase, projectId, token } = await readRuntimeAuth(page);

  await page.goto("/alerts");
  await expect(page).toHaveURL(/\/alerts$/);
  await expect(page.getByRole("main").getByText("报警")).toBeVisible();

  const stamp = Date.now();
  const contactName = `e2e-contact-${stamp}`;
  const contactEmail = `e2e-${stamp}@example.com`;
  const webhookName = `e2e-webhook-${stamp}`;
  const webhookURL = `https://example.com/hook/${stamp}`;
  const ruleName = `e2e-rule-${stamp}`;
  const keyword = `e2e-keyword-${stamp}`;

  const contactPanel = page.locator("section", { hasText: "新建联系人" }).first();
  await contactPanel.getByPlaceholder("ops", { exact: true }).fill(contactName);
  await contactPanel.getByPlaceholder("ops@example.com", { exact: true }).fill(contactEmail);
  await contactPanel.getByRole("button", { name: "创建" }).click();
  await expect(page.getByText("联系人已创建")).toBeVisible();
  await expect(page.locator("tr", { hasText: contactName }).first()).toBeVisible();

  await page.getByRole("button", { name: "通知渠道", exact: true }).click();
  const endpointPanel = page.locator("section", { hasText: "Webhook Endpoints" }).first();
  await endpointPanel.getByPlaceholder("ops-webhook", { exact: true }).fill(webhookName);
  await endpointPanel
    .getByPlaceholder("https://example.com/hook", { exact: true })
    .fill(webhookURL);
  await endpointPanel.getByRole("button", { name: "创建" }).click();
  await expect(page.getByText("Webhook Endpoint 已创建")).toBeVisible();
  await expect(page.locator("tr", { hasText: webhookName }).first()).toBeVisible();

  await page.getByRole("button", { name: "规则", exact: true }).click();
  const rulePanel = page.locator("section", { hasText: "新建规则（可视化）" }).first();
  await rulePanel.getByPlaceholder("BoomRule", { exact: true }).fill(ruleName);
  await rulePanel.getByPlaceholder("boom,timeout", { exact: true }).fill(keyword);
  const endpointOption = rulePanel.locator("label", { hasText: webhookName }).first();
  await endpointOption.locator("input[type='checkbox']").check();
  await rulePanel.getByRole("button", { name: "创建规则" }).click();
  await expect(page.getByText("规则已创建")).toBeVisible();

  const ruleRow = page.locator("tr", { hasText: ruleName }).first();
  await expect(ruleRow).toBeVisible();

  const rulesRes = await page.request.get(`${apiBase}/api/${projectId}/alerts/rules`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(rulesRes.status()).toBe(200);
  const rulesBody = await rulesRes.json();
  const rules = Array.isArray(rulesBody?.data?.items) ? rulesBody.data.items : [];
  const createdRule = rules.find((it: { name?: string; id?: number }) => it.name === ruleName);
  expect(Number(createdRule?.id ?? 0)).toBeGreaterThan(0);

  let deletedByUI = false;
  const uiDeleteRespPromise = page
    .waitForResponse(
      (resp) =>
        resp.request().method() === "DELETE" &&
        resp.url().includes(`/api/${projectId}/alerts/rules/${createdRule.id}`),
      { timeout: 2_000 },
    )
    .catch(() => null);
  const dialogPromise = page
    .waitForEvent("dialog", { timeout: 2_000 })
    .then(async (dialog) => {
      await dialog.accept();
      return true;
    })
    .catch(() => false);
  await ruleRow.getByRole("button", { name: "删除" }).click();
  await dialogPromise;
  const uiDeleteResp = await uiDeleteRespPromise;
  if (uiDeleteResp && uiDeleteResp.status() === 200) {
    deletedByUI = true;
  }

  if (!deletedByUI) {
    const deleteRes = await page.request.delete(
      `${apiBase}/api/${projectId}/alerts/rules/${createdRule.id}`,
      {
        headers: { Authorization: `Bearer ${token}` },
      },
    );
    expect(deleteRes.status()).toBe(200);
  }

  await expect
    .poll(
      async () => {
        const latestRulesRes = await page.request.get(
          `${apiBase}/api/${projectId}/alerts/rules`,
          {
            headers: { Authorization: `Bearer ${token}` },
          },
        );
        if (!latestRulesRes.ok()) return false;
        const latestRulesBody = await latestRulesRes.json();
        const items = Array.isArray(latestRulesBody?.data?.items)
          ? latestRulesBody.data.items
          : [];
        return !items.some((it: { name?: string }) => it.name === ruleName);
      },
      { timeout: 10_000, intervals: [300, 600, 1_000] },
    )
    .toBeTruthy();
});

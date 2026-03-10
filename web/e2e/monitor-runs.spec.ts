import { expect, test } from "@playwright/test";
import { ensureLoggedInAndEnterProject } from "./helpers";

test("monitor run creates run history record", async ({ page }) => {
  await ensureLoggedInAndEnterProject(page);

  await page.goto("/alerts");
  await expect(page).toHaveURL(/\/alerts$/);
  await page.getByRole("button", { name: "监控插件" }).click();
  await expect(page.getByText("监控插件说明")).toBeVisible();

  // Validate detector list is visible and contains built-in detector.
  await expect(page.getByText("Detector 列表")).toBeVisible();
  await expect(page.getByRole("cell", { name: "log_basic" }).first()).toBeVisible();

  const monitorName = `e2e-run-${Date.now()}`;
  await page.getByPlaceholder("api-health-check").fill(monitorName);
  await page.getByPlaceholder("60").fill("3600");
  await page.getByPlaceholder("5000").fill("2000");
  await page.getByRole("button", { name: "创建监控" }).click();
  await expect(page.getByText("监控已创建")).toBeVisible();

  const row = page.locator("tr", { hasText: monitorName }).first();
  await expect(row).toBeVisible();

  await row.getByRole("button", { name: /^Run$/ }).click();
  await expect(page.getByText("已触发异步调度")).toBeVisible();

  const runsBtn = row.getByRole("button", { name: /^Runs$/ });
  let seenSuccess = false;
  for (let i = 0; i < 20; i++) {
    await runsBtn.click();
    await page.waitForTimeout(300);
    const hasPanel = (await page.getByText("运行历史").count()) > 0;
    const successCount = await page.getByText("success").count();
    if (hasPanel && successCount > 0) {
      seenSuccess = true;
      break;
    }
  }
  expect(seenSuccess).toBeTruthy();
});

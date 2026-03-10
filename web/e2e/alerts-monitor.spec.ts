import { expect, test } from "@playwright/test";
import { ensureLoggedInAndEnterProject } from "./helpers";

test("bootstrap/login and monitor plugin workflow", async ({ page }) => {
  await ensureLoggedInAndEnterProject(page);

  await page.getByRole("link", { name: "报警" }).click();
  await expect(page).toHaveURL(/\/alerts$/);

  await page.getByRole("button", { name: "监控插件" }).click();
  await expect(page.getByText("监控插件说明")).toBeVisible();

  const monitorName = `e2e-monitor-${Date.now()}`;
  await page.getByPlaceholder("api-health-check").fill(monitorName);
  await page.getByRole("button", { name: "创建监控" }).click();
  await expect(page.getByText("监控已创建")).toBeVisible();

  const row = page.locator("tr", { hasText: monitorName }).first();
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: "Test" }).click();

  await expect(page.getByText("试运行结果")).toBeVisible();
  await expect(page.getByText("signalCount")).toBeVisible();
});

import { expect, test } from "@playwright/test";
import { ensureLoggedInAndEnterProject, readRuntimeSettings } from "./helpers";

test("settings key lifecycle enforces ingest auth after revoke", async ({ page }) => {
  test.setTimeout(90_000);

  await ensureLoggedInAndEnterProject(page);

  await page.goto("/settings");
  await expect(page).toHaveURL(/\/settings(#.*)?$/);
  await expect(page.getByText("项目设置")).toBeVisible();

  const keyName = `e2e-key-${Date.now()}`;
  await page.getByPlaceholder("default").fill(keyName);
  await page.getByRole("button", { name: "新建 Key" }).click();

  const row = page.locator("tr", { hasText: keyName }).first();
  await expect(row).toBeVisible();

  const keyValue = (await row.locator("td").nth(1).textContent())?.trim() ?? "";
  expect(keyValue.startsWith("pk_")).toBeTruthy();

  const settings = await readRuntimeSettings(page);
  const ingestURL = `${settings.apiBase}/api/${settings.projectId}/logs/`;

  const okRes = await page.request.post(ingestURL, {
    headers: {
      "Content-Type": "application/json",
      "X-Project-Key": keyValue,
    },
    data: {
      level: "info",
      message: `e2e-key-before-revoke-${Date.now()}`,
    },
  });
  expect(okRes.status()).toBe(202);

  await row.getByRole("button", { name: "吊销" }).click();
  await expect(row.getByText("revoked")).toBeVisible();

  // Project key auth has a short-lived server cache; revoke is eventually enforced.
  await expect
    .poll(
      async () => {
        const deniedRes = await page.request.post(ingestURL, {
          headers: {
            "Content-Type": "application/json",
            "X-Project-Key": keyValue,
          },
          data: {
            level: "info",
            message: `e2e-key-after-revoke-${Date.now()}`,
          },
        });
        return [401, 403].includes(deniedRes.status());
      },
      { timeout: 45_000, intervals: [500, 1_000, 2_000, 5_000] },
    )
    .toBeTruthy();
});

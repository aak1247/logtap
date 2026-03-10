import { expect, type Page } from "@playwright/test";

const E2E_EMAIL = "e2e@example.com";
const E2E_PASSWORD = "pass123456";
const E2E_PROJECT = "E2E Default";

export async function ensureLoggedInAndEnterProject(page: Page): Promise<void> {
  await page.goto("/bootstrap");

  if (page.url().includes("/bootstrap")) {
    const initBtn = page.getByRole("button", { name: "初始化" });
    await Promise.race([
      page.waitForURL(/\/login(\?.*)?$/, { timeout: 7_000 }),
      expect(initBtn).toBeEnabled({ timeout: 7_000 }),
    ]).catch(() => undefined);
  }

  if (page.url().includes("/bootstrap")) {
    await expect(page.getByText("系统初始化")).toBeVisible();
    await page.getByPlaceholder("admin@example.com").fill(E2E_EMAIL);
    await page.getByPlaceholder("********").fill(E2E_PASSWORD);
    await page.getByPlaceholder("Default").fill(E2E_PROJECT);
    await page.getByRole("button", { name: "初始化" }).click();
    await expect(page).toHaveURL(/\/login(\?.*)?$/);
  }

  if (!page.url().includes("/login")) {
    await page.goto("/login");
  }

  await expect(page.getByRole("button", { name: "登录" })).toBeVisible();
  await page.getByPlaceholder("you@example.com").fill(E2E_EMAIL);
  await page.getByPlaceholder("********").fill(E2E_PASSWORD);
  await page.getByRole("button", { name: "登录" }).click();
  await expect(page).toHaveURL(/\/projects$/);

  await page.getByRole("button", { name: "进入" }).first().click();
  await expect(page).toHaveURL(/\/$/);
}

export async function readRuntimeSettings(
  page: Page,
): Promise<{ apiBase: string; projectId: string }> {
  const settings = await readRuntimeAuth(page);
  return {
    apiBase: settings.apiBase,
    projectId: settings.projectId,
  };
}

export async function readRuntimeAuth(
  page: Page,
): Promise<{ apiBase: string; projectId: string; token: string }> {
  const settings = await page.evaluate(
    (): { apiBase: string; projectId: string; token: string } | null => {
    const keys = Object.keys(window.localStorage);
    for (const key of keys) {
      const raw = window.localStorage.getItem(key);
      if (!raw) continue;
      try {
        const parsed = JSON.parse(raw) as Record<string, unknown>;
        const apiBase = typeof parsed.apiBase === "string" ? parsed.apiBase.trim() : "";
        const projectId = typeof parsed.projectId === "string" ? parsed.projectId.trim() : "";
        const token = typeof parsed.token === "string" ? parsed.token.trim() : "";
        if (apiBase && projectId && token) {
          return { apiBase, projectId, token };
        }
      } catch {
      }
    }
    return null;
  });
  if (!settings) {
    throw new Error("failed to read runtime settings from localStorage");
  }
  return {
    apiBase: settings.apiBase.replace(/\/+$/, ""),
    projectId: settings.projectId,
    token: settings.token,
  };
}

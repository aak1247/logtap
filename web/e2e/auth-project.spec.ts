import { expect, test } from "@playwright/test";
import { ensureLoggedInAndEnterProject } from "./helpers";

test("auth and project smoke flow", async ({ page }) => {
  await ensureLoggedInAndEnterProject(page);

  await page.goto("/projects");
  await expect(page.getByText("切换项目")).toBeVisible();

  const projectName = `e2e-project-${Date.now()}`;
  await page.getByPlaceholder("My Project").fill(projectName);
  await page.getByRole("button", { name: "新建" }).click();

  const projectTitle = page.locator("div.text-sm.font-semibold", { hasText: projectName }).first();
  const projectCard = projectTitle.locator("xpath=ancestor::div[contains(@class,'rounded-lg')][1]");
  await expect(projectCard).toBeVisible();
  await projectCard.getByRole("button", { name: "进入" }).first().click();

  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByText("logtap 控制台")).toBeVisible();
});

test("unauthenticated user is redirected to login", async ({ page }) => {
  await page.goto("/login");
  await page.evaluate(() => {
    window.localStorage.clear();
  });

  await page.goto("/alerts");
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByRole("button", { name: "登录" })).toBeVisible();
});

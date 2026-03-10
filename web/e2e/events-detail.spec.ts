import { expect, test } from "@playwright/test";
import { ensureLoggedInAndEnterProject, readRuntimeAuth } from "./helpers";

test("event ingest is visible in events list and detail page", async ({ page }) => {
  await ensureLoggedInAndEnterProject(page);

  const { apiBase, projectId, token } = await readRuntimeAuth(page);

  const keyName = `e2e-events-key-${Date.now()}`;
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

  const eventTitle = `e2e-event-title-${Date.now()}`;
  const eventID = crypto.randomUUID();
  const storeRes = await page.request.post(`${apiBase}/api/${projectId}/store/`, {
    headers: {
      "Content-Type": "application/json",
      "X-Project-Key": projectKey,
    },
    data: {
      event_id: eventID,
      level: "error",
      message: eventTitle,
      timestamp: new Date().toISOString(),
      user: { id: "e2e-user-1" },
    },
  });
  expect(storeRes.status()).toBe(200);
  const storeBody = await storeRes.json();
  const storedID = String(storeBody?.id ?? eventID);

  await expect
    .poll(
      async () => {
        const recentRes = await page.request.get(
          `${apiBase}/api/${projectId}/events/recent?limit=20`,
          {
            headers: { Authorization: `Bearer ${token}` },
          },
        );
        if (!recentRes.ok()) return false;
        const recentBody = await recentRes.json();
        const items = Array.isArray(recentBody?.data) ? recentBody.data : [];
        return items.some((it: { id?: string; title?: string }) => it.id === storedID && it.title === eventTitle);
      },
      { timeout: 10_000, intervals: [300, 600, 1_000] },
    )
    .toBeTruthy();

  await page.goto("/events");
  await expect(page).toHaveURL(/\/events$/);

  const titleLink = page.getByRole("link", { name: eventTitle }).first();
  await expect(titleLink).toBeVisible();
  await titleLink.click();

  await expect(page).toHaveURL(new RegExp(`/events/${escapeRegExp(storedID)}$`));
  await expect(page.getByText("事件详情")).toBeVisible();
  await expect(page.getByText(eventTitle)).toBeVisible();
});

function escapeRegExp(input: string): string {
  return input.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

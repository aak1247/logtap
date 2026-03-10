import { expect, test } from "@playwright/test";
import { ensureLoggedInAndEnterProject, readRuntimeAuth } from "./helpers";

test("analytics page shows ingested track event in top events", async ({ page }) => {
  await ensureLoggedInAndEnterProject(page);

  const { apiBase, projectId, token } = await readRuntimeAuth(page);

  const keyName = `e2e-analytics-key-${Date.now()}`;
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

  const eventName = `e2e-analytics-event-${Date.now()}`;
  const events = [
    { user: "u-1" },
    { user: "u-2" },
  ];
  for (const item of events) {
    const ingestRes = await page.request.post(`${apiBase}/api/${projectId}/track/`, {
      headers: {
        "Content-Type": "application/json",
        "X-Project-Key": projectKey,
      },
      data: {
        name: eventName,
        timestamp: new Date().toISOString(),
        user: { id: item.user },
      },
    });
    expect(ingestRes.status()).toBe(202);
  }

  await expect
    .poll(
      async () => {
        const topRes = await page.request.get(
          `${apiBase}/api/${projectId}/analytics/events/top?limit=20`,
          {
            headers: { Authorization: `Bearer ${token}` },
          },
        );
        if (!topRes.ok()) return false;
        const topBody = await topRes.json();
        const items = Array.isArray(topBody?.data?.items) ? topBody.data.items : [];
        return items.some((it: { name?: string }) => it.name === eventName);
      },
      { timeout: 10_000, intervals: [300, 600, 1_000] },
    )
    .toBeTruthy();

  const nowISO = new Date().toISOString();
  const apiProjectPrefix = `${apiBase}/api/${projectId}`;
  await page.route(`${apiProjectPrefix}/analytics/active*`, async (route) => {
    const reqURL = new URL(route.request().url());
    const bucket = reqURL.searchParams.get("bucket") ?? "day";
    const bucketLabel = bucket === "month" ? nowISO.slice(0, 7) : nowISO.slice(0, 10);
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        code: 0,
        data: {
          project_id: Number(projectId),
          bucket,
          start: nowISO,
          end: nowISO,
          series: [{ bucket: bucketLabel, active: 2 }],
        },
      }),
    });
  });
  await page.route(`${apiProjectPrefix}/analytics/dist*`, async (route) => {
    const reqURL = new URL(route.request().url());
    const dim = reqURL.searchParams.get("dim") ?? "os";
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        code: 0,
        data: {
          project_id: Number(projectId),
          dim,
          start: nowISO,
          end: nowISO,
          items: [],
        },
      }),
    });
  });
  await page.route(`${apiProjectPrefix}/analytics/retention*`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        code: 0,
        data: {
          project_id: Number(projectId),
          start: nowISO,
          end: nowISO,
          days: [1, 7, 30],
          rows: [],
        },
      }),
    });
  });

  await page.goto("/analytics");
  await expect(page).toHaveURL(/\/analytics$/);
  await expect(page.getByText("事件分析（自定义日志 message）")).toBeVisible();
  await expect(page.getByRole("cell", { name: eventName })).toBeVisible();
});

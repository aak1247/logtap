import { expect, test, type Page } from "@playwright/test";
import { ensureLoggedInAndEnterProject, readRuntimeAuth } from "./helpers";

test("analytics page can calculate funnel from ingested track events", async ({ page }) => {
  await ensureLoggedInAndEnterProject(page);

  const { apiBase, projectId, token } = await readRuntimeAuth(page);

  const keyName = `e2e-funnel-key-${Date.now()}`;
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

  const baseTs = Date.now();
  const entries = [
    { user: "u1", name: "signup", offsetMs: 1000 },
    { user: "u1", name: "checkout", offsetMs: 2000 },
    { user: "u1", name: "paid", offsetMs: 3000 },
    { user: "u2", name: "signup", offsetMs: 1000 },
    { user: "u2", name: "checkout", offsetMs: 2000 },
  ];
  for (const item of entries) {
    const ingestRes = await page.request.post(`${apiBase}/api/${projectId}/track/`, {
      headers: {
        "Content-Type": "application/json",
        "X-Project-Key": projectKey,
      },
      data: {
        name: item.name,
        timestamp: new Date(baseTs + item.offsetMs).toISOString(),
        user: { id: item.user },
      },
    });
    expect(ingestRes.status()).toBe(202);
  }

  await expect
    .poll(
      async () => {
        const funnelRes = await page.request.get(
          `${apiBase}/api/${projectId}/analytics/funnel?steps=signup,checkout,paid&within=24h`,
          {
            headers: { Authorization: `Bearer ${token}` },
          },
        );
        if (!funnelRes.ok()) return false;
        const funnelBody = await funnelRes.json();
        const steps = Array.isArray(funnelBody?.data?.steps) ? funnelBody.data.steps : [];
        const byName = new Map(
          steps.map((it: { name?: string; users?: number }) => [
            String(it?.name ?? ""),
            Number(it?.users ?? 0),
          ]),
        );
        return byName.get("signup") === 2 && byName.get("checkout") === 2 && byName.get("paid") === 1;
      },
      { timeout: 10_000, intervals: [300, 600, 1_000] },
    )
    .toBeTruthy();

  await mockAnalyticsReadonlyEndpoints(page, apiBase, projectId);

  await page.goto("/analytics");
  await expect(page).toHaveURL(/\/analytics$/);
  await expect(page.getByText("事件分析（自定义日志 message）")).toBeVisible();

  await page.getByRole("button", { name: "计算" }).click();
  const funnelTable = page
    .locator("table")
    .filter({ has: page.getByRole("columnheader", { name: "步骤" }) })
    .filter({ has: page.getByRole("columnheader", { name: "转化" }) })
    .first();
  await expect(funnelTable.getByRole("cell", { name: "signup" }).first()).toBeVisible();
  await expect(funnelTable.getByRole("cell", { name: "checkout" }).first()).toBeVisible();
  await expect(funnelTable.getByRole("cell", { name: "paid" }).first()).toBeVisible();
});

async function mockAnalyticsReadonlyEndpoints(
  page: Page,
  apiBase: string,
  projectId: string,
): Promise<void> {
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
}

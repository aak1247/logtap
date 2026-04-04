import { chromium } from "playwright";

const BASE = "http://127.0.0.1:8090";

(async () => {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1920, height: 1080 } });
  const page = await context.newPage();
  
  // Monitor network requests
  page.on("request", req => {
    if (req.url().includes("/api/")) {
      console.log(`→ ${req.method()} ${req.url()}`);
    }
  });
  page.on("response", res => {
    if (res.url().includes("/api/")) {
      console.log(`← ${res.status()} ${res.url()}`);
    }
  });
  page.on("console", msg => {
    if (msg.type() === "error") console.log(`CONSOLE ERROR: ${msg.text().substring(0, 200)}`);
  });

  await page.goto(BASE, { waitUntil: "networkidle", timeout: 10000 });
  await page.waitForTimeout(3000);
  
  console.log(`\nFinal URL: ${page.url()}`);
  const bodyText = await page.textContent("body");
  console.log(`Body text (first 500 chars): ${bodyText.substring(0, 500)}`);
  
  await page.screenshot({ path: "/tmp/e2e-debug.png" });
  
  // Try navigating directly to /login
  console.log("\n--- Navigating to /login ---");
  await page.goto(`${BASE}/login`, { waitUntil: "networkidle", timeout: 10000 });
  await page.waitForTimeout(3000);
  
  console.log(`Final URL: ${page.url()}`);
  const bodyText2 = await page.textContent("body");
  console.log(`Body text (first 500 chars): ${bodyText2.substring(0, 500)}`);
  
  await page.screenshot({ path: "/tmp/e2e-debug-login.png" });
  
  await browser.close();
})();

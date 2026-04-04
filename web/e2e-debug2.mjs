import { chromium } from "playwright";

const BASE = "http://127.0.0.1:8090";

(async () => {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1920, height: 1080 } });
  const page = await context.newPage();
  
  page.on("console", async msg => {
    if (msg.type() === "error") {
      const args = msg.args();
      for (const arg of args) {
        try {
          const val = await arg.jsonValue();
          if (typeof val === "string") console.log(`ERR: ${val.substring(0, 500)}`);
          else if (val && val.message) console.log(`ERR: ${val.message}`);
        } catch {}
      }
    }
  });

  await page.goto(`${BASE}/login`, { waitUntil: "networkidle", timeout: 10000 });
  await page.waitForTimeout(3000);
  
  // Get the error boundary message
  const errorText = await page.evaluate(() => {
    const el = document.querySelector('[style*="color: red"], .error, pre');
    return el ? el.textContent : document.body.textContent;
  });
  console.log(`Error text: ${errorText.substring(0, 1000)}`);
  
  await page.screenshot({ path: "/tmp/e2e-debug2.png" });
  await browser.close();
})();

import { chromium } from "playwright";

const BASE = "http://127.0.0.1:8090";

let passed = 0;
let failed = 0;

function assert(condition, msg) {
  if (!condition) {
    console.log(`  ❌ FAIL: ${msg}`);
    failed++;
  } else {
    console.log(`  ✅ PASS: ${msg}`);
    passed++;
  }
}

(async () => {
  console.log("\n🧪 LogTap Cloud UI E2E Tests\n");
  console.log("=".repeat(50));

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1920, height: 1080 } });
  const page = await context.newPage();

  // Collect console errors
  const consoleErrors = [];
  page.on("console", msg => {
    if (msg.type() === "error") consoleErrors.push(msg.text());
  });

  // ==================== Test 1: Login Page ====================
  console.log("\n📋 Test 1: Login Page");
  try {
    await page.goto(BASE, { waitUntil: "networkidle", timeout: 10000 });
    const title = await page.title();
    assert(title.includes("logtap"), `Page title: "${title}"`);
    await page.screenshot({ path: "/tmp/e2e-01-login.png" });
  } catch (e) {
    assert(false, `Login page: ${e.message}`);
  }

  // ==================== Test 2: Login via UI ====================
  console.log("\n📋 Test 2: Login via UI");
  try {
    // Wait for the status check to complete and form to be enabled
    await page.waitForTimeout(2000);
    
    // Fill login form
    const emailInput = await page.$('input[placeholder="you@example.com"]');
    const passwordInput = await page.$('input[type="password"][placeholder="********"]');
    
    assert(emailInput !== null, "Email input found");
    assert(passwordInput !== null, "Password input found");
    
    if (emailInput && passwordInput) {
      await emailInput.fill("test@example.com");
      await passwordInput.fill("Test123!");
      
      await page.screenshot({ path: "/tmp/e2e-02-login-filled.png" });
      
      // Click login button
      const loginBtn = await page.$('button:has-text("登录")');
      if (loginBtn) {
        const isEnabled = await loginBtn.isEnabled();
        assert(isEnabled, "Login button is enabled");
        
        if (isEnabled) {
          await loginBtn.click();
          await page.waitForTimeout(3000);
          
          const url = page.url();
          assert(!url.includes("/login"), `Redirected after login: ${url}`);
        }
      } else {
        assert(false, "Login button not found");
      }
    }
    
    await page.screenshot({ path: "/tmp/e2e-02-after-login.png" });
  } catch (e) {
    assert(false, `Login: ${e.message}`);
  }

  // ==================== Test 3: Dashboard ====================
  console.log("\n📋 Test 3: Dashboard");
  try {
    // After login we might be on /projects (no project selected) or /
    const url = page.url();
    console.log(`  Current URL: ${url}`);
    
    // Navigate to projects to select one
    await page.goto(`${BASE}/projects`, { waitUntil: "networkidle", timeout: 10000 });
    await page.waitForTimeout(1500);
    await page.screenshot({ path: "/tmp/e2e-03-projects.png" });
    
    const bodyText = await page.textContent("body");
    const hasProjects = bodyText.includes("Default") || bodyText.includes("test-project") || bodyText.includes("项目");
    assert(hasProjects, `Projects page shows content`);
  } catch (e) {
    assert(false, `Dashboard: ${e.message}`);
  }

  // ==================== Test 4: Organizations Page ====================
  console.log("\n📋 Test 4: Organizations Page");
  try {
    await page.goto(`${BASE}/orgs`, { waitUntil: "networkidle", timeout: 10000 });
    await page.waitForTimeout(2000);
    await page.screenshot({ path: "/tmp/e2e-04-orgs.png" });
    
    const bodyText = await page.textContent("body");
    const hasOrgContent = bodyText.includes("test-org") || bodyText.includes("Test Organization") || bodyText.includes("组织");
    assert(hasOrgContent, `Orgs page shows organization content`);
  } catch (e) {
    assert(false, `Orgs: ${e.message}`);
  }

  // ==================== Test 5: Organization Detail ====================
  console.log("\n📋 Test 5: Organization Detail");
  try {
    await page.goto(`${BASE}/orgs/1`, { waitUntil: "networkidle", timeout: 10000 });
    await page.waitForTimeout(2000);
    await page.screenshot({ path: "/tmp/e2e-05-org-detail.png" });
    
    const bodyText = await page.textContent("body");
    const hasDetail = bodyText.includes("成员") || bodyText.includes("member") || bodyText.includes("test-org");
    assert(hasDetail, `Org detail shows member info`);
  } catch (e) {
    assert(false, `Org detail: ${e.message}`);
  }

  // ==================== Test 6: Plans Page ====================
  console.log("\n📋 Test 6: Plans Page");
  try {
    await page.goto(`${BASE}/plans`, { waitUntil: "networkidle", timeout: 10000 });
    await page.waitForTimeout(2000);
    await page.screenshot({ path: "/tmp/e2e-06-plans.png" });
    
    const bodyText = await page.textContent("body");
    const hasPlans = bodyText.includes("Startup") || bodyText.includes("Free") || bodyText.includes("套餐");
    assert(hasPlans, `Plans page shows plan content`);
  } catch (e) {
    assert(false, `Plans: ${e.message}`);
  }

  // ==================== Test 7: Org Subscription ====================
  console.log("\n📋 Test 7: Org Subscription");
  try {
    await page.goto(`${BASE}/orgs/1/subscription`, { waitUntil: "networkidle", timeout: 10000 });
    await page.waitForTimeout(2000);
    await page.screenshot({ path: "/tmp/e2e-07-subscription.png" });
    
    const bodyText = await page.textContent("body");
    const hasSub = bodyText.includes("Startup") || bodyText.includes("订阅") || bodyText.includes("subscription");
    assert(hasSub, `Subscription page shows subscription info`);
  } catch (e) {
    assert(false, `Subscription: ${e.message}`);
  }

  // ==================== Test 8: Org Billing ====================
  console.log("\n📋 Test 8: Org Billing");
  try {
    await page.goto(`${BASE}/orgs/1/billing`, { waitUntil: "networkidle", timeout: 10000 });
    await page.waitForTimeout(2000);
    await page.screenshot({ path: "/tmp/e2e-08-billing.png" });
    
    const bodyText = await page.textContent("body");
    const hasBilling = bodyText.includes("账单") || bodyText.includes("billing") || bodyText.includes("充值") || bodyText.includes("100");
    assert(hasBilling, `Billing page shows billing info`);
  } catch (e) {
    assert(false, `Billing: ${e.message}`);
  }

  // ==================== Summary ====================
  console.log("\n" + "=".repeat(50));
  console.log(`\n📊 Results: ${passed} passed, ${failed} failed, ${passed + failed} total\n`);
  
  if (consoleErrors.length > 0) {
    console.log("⚠️  Console errors:");
    consoleErrors.forEach(e => console.log(`  - ${e}`));
    console.log("");
  }
  
  if (failed > 0) {
    console.log("❌ Check screenshots in /tmp/e2e-*.png\n");
  } else {
    console.log("✅ All tests passed!\n");
  }

  await browser.close();
  process.exit(failed > 0 ? 1 : 0);
})();

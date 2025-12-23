import { createServer } from "node:http";
import { statSync, createReadStream } from "node:fs";
import { extname, join, resolve, normalize } from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = resolve(__filename, "..");

const repoRoot = resolve(__dirname, "..", "..");
const port = Number(process.env.PORT || 5174);

const mime = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".mjs": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".jpg": "image/jpeg",
  ".jpeg": "image/jpeg",
  ".txt": "text/plain; charset=utf-8",
};

function safeResolve(urlPath) {
  const p = normalize(urlPath).replace(/^([/\\])+/, "");
  const abs = resolve(repoRoot, p);
  if (!abs.startsWith(repoRoot)) return null;
  return abs;
}

const server = createServer((req, res) => {
  try {
    const u = new URL(req.url || "/", `http://${req.headers.host || "localhost"}`);
    let pathname = u.pathname;

    if (pathname === "/") {
      res.writeHead(302, { Location: "/demo/js-browser/" });
      res.end();
      return;
    }

    if (pathname.endsWith("/")) pathname += "index.html";

    const abs = safeResolve(pathname);
    if (!abs) {
      res.writeHead(403);
      res.end("forbidden");
      return;
    }

    const st = statSync(abs, { throwIfNoEntry: false });
    if (!st || !st.isFile()) {
      res.writeHead(404);
      res.end("not found");
      return;
    }

    const ext = extname(abs).toLowerCase();
    res.writeHead(200, {
      "Content-Type": mime[ext] || "application/octet-stream",
      "Cache-Control": "no-store",
    });
    createReadStream(abs).pipe(res);
  } catch (e) {
    res.writeHead(500);
    res.end(String(e?.message || e));
  }
});

server.listen(port, "127.0.0.1", () => {
  console.log(`Static server: http://localhost:${port}/demo/js-browser/`);
  console.log(`Serving repo root: ${repoRoot}`);
});


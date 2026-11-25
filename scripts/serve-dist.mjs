// A minimal static server with the same rules as the production Nginx config (deploy/nginx.conf):
//   try_files $uri $uri/ =404;  directory → index.html;  "/dir" → 301 "/dir/";  error_page 404 /404.html
// Used by the browser tests so they exercise exactly what Nginx will serve. Usage: node scripts/serve-dist.mjs [port] [dir]
import { createServer } from "node:http";
import { createReadStream, existsSync, statSync } from "node:fs";
import { extname, join, normalize } from "node:path";

const PORT = Number(process.argv[2] ?? 4329);
const ROOT = process.argv[3] ?? "dist";
const TYPES = {
  ".html": "text/html; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".jpg": "image/jpeg",
  ".webp": "image/webp",
  ".avif": "image/avif",
  ".woff2": "font/woff2",
  ".xml": "application/xml; charset=utf-8",
  ".txt": "text/plain; charset=utf-8",
  ".json": "application/json",
};

function send(res, file, status = 200) {
  res.writeHead(status, { "Content-Type": TYPES[extname(file)] ?? "application/octet-stream" });
  createReadStream(file).pipe(res);
}

createServer((req, res) => {
  const url = new URL(req.url ?? "/", "http://x");
  const rel = normalize(decodeURIComponent(url.pathname)).replace(/^(\.\.[/\\])+/, "");
  const file = join(ROOT, rel);
  if (existsSync(file) && statSync(file).isFile()) return send(res, file);
  if (existsSync(file) && statSync(file).isDirectory()) {
    if (!url.pathname.endsWith("/")) {
      res.writeHead(301, { Location: url.pathname + "/" + url.search });
      return res.end();
    }
    const index = join(file, "index.html");
    if (existsSync(index)) return send(res, index);
  }
  send(res, join(ROOT, "404.html"), 404);
}).listen(PORT, "127.0.0.1", () => console.log(`serving ${ROOT} on http://127.0.0.1:${PORT}`));

import { $ } from "bun";
import { readFileSync, writeFileSync, rmSync, watch } from "fs";
import { basename } from "path";

const BACKEND = "http://localhost:8080";
const PORT = 3000;

// ── Build helpers ─────────────────────────────────────────────────

let firstBuild = true;

async function buildAll(): Promise<boolean> {
  // 首次启动时清空 dist，防止历史哈希 chunk 堆积
  if (firstBuild) {
    rmSync("./dist", { recursive: true, force: true });
    firstBuild = false;
  }

  const start = performance.now();

  const result = await Bun.build({
    entrypoints: ["./src/main.tsx"],
    outdir: "./dist",
    format: "esm",
    minify: false,
    splitting: true,
    target: "browser",
    sourcemap: "linked",
    naming: "[name]-[hash].[ext]",
  });

  if (!result.success) {
    for (const log of result.logs) console.error(log);
    return false;
  }

  const entryOutput = result.outputs.find((o) => o.kind === "entry-point");
  if (!entryOutput) {
    console.error("entry-point not found in build outputs");
    return false;
  }
  const entryName = basename(entryOutput.path);

  const cssOutputs = result.outputs.filter((o) => o.path.endsWith(".css"));
  const cssLinks = cssOutputs
    .map((o) => `  <link rel="stylesheet" href="/${basename(o.path)}">`)
    .join("\n");

  // stderr: "inherit" 让 Tailwind 的报错直接输出，便于调试
  await $`bunx @tailwindcss/cli -i ./src/app.css -o ./dist/app.css --minify`
    .quiet()
    .nothrow();

  const html = readFileSync("./public/index.html", "utf-8");
  writeFileSync(
    "./dist/index.html",
    html
      .replace(/src="\/main\.js"/, `src="/${entryName}"`)
      .replace(/<\/head>/, `${cssLinks}\n</head>`),
  );

  console.log(`\x1b[32m✓\x1b[0m Built in ${(performance.now() - start).toFixed(0)}ms`);
  return true;
}

// ── Initial build ─────────────────────────────────────────────────

await buildAll();

// ── File watcher ──────────────────────────────────────────────────

let debounce: ReturnType<typeof setTimeout> | null = null;
let building = false;

function scheduleRebuild() {
  if (debounce) clearTimeout(debounce);
  debounce = setTimeout(async () => {
    if (building) return;
    building = true;
    console.log("\x1b[36m⟳\x1b[0m Rebuilding...");
    try {
      await buildAll();
    } finally {
      building = false;
    }
  }, 200);
}

for (const dir of ["./src", "./public"]) {
  watch(dir, { recursive: true }, (_event, filename) => {
    if (filename && !filename.includes("node_modules")) {
      scheduleRebuild();
    }
  });
}

// ── Dev server ────────────────────────────────────────────────────

const backendHost = new URL(BACKEND).host;

Bun.serve({
  port: PORT,
  idleTimeout: 0,
  async fetch(req) {
    const url = new URL(req.url);

    // Proxy API requests to Go backend
    if (url.pathname.startsWith("/v1/") || url.pathname.startsWith("/sub/") || url.pathname.startsWith("/api/")) {
      const target = new URL(url.pathname + url.search, BACKEND);
      try {
        const proxyHeaders = new Headers(req.headers);
        proxyHeaders.set("Host", backendHost);
        return await fetch(target.toString(), {
          method: req.method,
          headers: proxyHeaders,
          body: req.body,
          redirect: "manual",
          signal: AbortSignal.timeout(30_000),
        });
      } catch (err) {
        if (err instanceof Error && err.name === "TimeoutError") {
          return Response.json({ error: "Request timed out" }, { status: 504 });
        }
        return Response.json({ error: "Backend unavailable" }, { status: 502 });
      }
    }

    // Serve static files from dist/
    const filePath = `./dist${url.pathname === "/" ? "/index.html" : url.pathname}`;
    const file = Bun.file(filePath);
    if (await file.exists()) return new Response(file);

    // SPA fallback — serve index.html for all other routes
    return new Response(Bun.file("./dist/index.html"));
  },
});

console.log(`Dev server → http://localhost:${PORT} (watching src/ for changes)`);

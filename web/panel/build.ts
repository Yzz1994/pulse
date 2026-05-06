import { $ } from "bun";
import { readFileSync, writeFileSync, rmSync } from "fs";
import { basename } from "path";

// 每次构建前清空输出目录，防止旧哈希文件堆积
rmSync("./dist", { recursive: true, force: true });

// Build React app
const result = await Bun.build({
  entrypoints: ["./src/main.tsx"],
  outdir: "./dist",
  format: "esm",
  minify: true,
  splitting: true,
  target: "browser",
  sourcemap: false,
  naming: "[name]-[hash].[ext]",
});

if (!result.success) {
  for (const log of result.logs) console.error(log);
  process.exit(1);
}

const entryOutput = result.outputs.find((o) => o.kind === "entry-point");
if (!entryOutput) {
  console.error("entry-point not found in build outputs");
  process.exit(1);
}
const entryName = basename(entryOutput.path);

// 收集 Bun 输出的 CSS chunk（来自 JS 里 import 的第三方库 CSS）
const cssOutputs = result.outputs.filter((o) => o.path.endsWith(".css"));
const cssLinks = cssOutputs
  .map((o) => `  <link rel="stylesheet" href="/${basename(o.path)}">`)
  .join("\n");

// Build Tailwind CSS
await $`bunx @tailwindcss/cli -i ./src/app.css -o ./dist/app.css --minify`.quiet();

// 生成 index.html，注入 JS 入口和 CSS chunks
const html = readFileSync("./public/index.html", "utf-8");
writeFileSync(
  "./dist/index.html",
  html
    .replace(/src="\/main\.js"/, `src="/${entryName}"`)
    .replace(/<\/head>/, `${cssLinks}\n</head>`),
);

console.log(`Build complete → dist/ (entry: ${entryName})`);

import { copyFile, mkdir } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const projectRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const source = resolve(
  projectRoot,
  "node_modules/@tailwindcss/browser/dist/index.global.js",
);
const destination = resolve(
  projectRoot,
  "public/vendor/tailwindcss-browser-4.3.3.js",
);

await mkdir(dirname(destination), { recursive: true });
await copyFile(source, destination);
console.log(`Copied Tailwind browser runtime to ${destination}`);

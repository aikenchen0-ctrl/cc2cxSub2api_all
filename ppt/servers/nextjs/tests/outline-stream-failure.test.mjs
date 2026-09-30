import assert from "node:assert/strict";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";
import { pathToFileURL } from "node:url";
import { build } from "esbuild";

let directory, useOutlineStreaming;
test.before(async () => {
  directory = await mkdtemp(path.join(tmpdir(), "ppt-outline-stream-"));
  const output = path.join(directory, "hook.mjs");
  await build({
    entryPoints: [path.resolve("app/(presentation-generator)/outline/hooks/useOutlineStreaming.ts")],
    outfile: output, bundle: true, platform: "node", format: "esm", tsconfig: "tsconfig.json",
    plugins: [{ name: "hook-harness", setup(build) {
      const mocks = {
        react: `export const useState=(value)=>{const i=globalThis.h.states.length;globalThis.h.states.push(value);return [value,(next)=>{globalThis.h.states[i]=typeof next==='function'?next(globalThis.h.states[i]):next}];};export const useRef=(current)=>({current});export const useEffect=(fn)=>globalThis.h.effects.push(fn);`,
        "react-redux": `export const useDispatch=()=>action=>{globalThis.h.outlines=action};export const useSelector=fn=>fn({presentationGeneration:{outlines:[]}});`,
        "@/components/ui/sonner": `export const notify={info(){},error(){},dismiss(){}};`,
        "@/store/slices/presentationGeneration": `export const setOutlines=value=>value;`,
        "@/utils/api": `export const getApiUrl=value=>value;`,
        "@/utils/chatgptAuth": `export const isChatGptAuthRequiredMessage=()=>false;export const requestChatGptReauth=()=>{};`,
      };
      build.onResolve({ filter: /.*/ }, args => args.path in mocks ? { path: args.path, namespace: "mock" } : undefined);
      build.onLoad({ filter: /.*/, namespace: "mock" }, args => ({ contents: mocks[args.path], loader: "js" }));
    }}],
  });
  ({ useOutlineStreaming } = await import(pathToFileURL(output).href));
});
test.after(async () => { await rm(directory, { recursive: true, force: true }); });

function start() {
  const saved = { EventSource: globalThis.EventSource, setTimeout: globalThis.setTimeout, clearTimeout: globalThis.clearTimeout };
  globalThis.h = { states: [], effects: [], sources: [], timers: [], outlines: [] };
  globalThis.EventSource = class {
    closed = false;
    constructor() { h.sources.push(this); }
    addEventListener(_name, callback) { this.callback = callback; }
    close() { this.closed = true; }
    emit(data) { this.callback({ data: JSON.stringify(data) }); }
  };
  globalThis.setTimeout = fn => { h.timers.push(fn); return h.timers.length; };
  globalThis.clearTimeout = () => {};
  const hook = useOutlineStreaming("test-presentation");
  const cleanup = h.effects.map(fn => fn());
  return { hook, finish() { cleanup.forEach(fn => fn?.()); Object.assign(globalThis, saved); delete globalThis.h; } };
}

test("retry exhaustion retains an error, closes the stream and supports explicit retry", () => {
  const run = start();
  try {
    for (let attempt = 0; attempt < 5; attempt++) {
      h.sources.at(-1).emit({ type: "error", detail: "upstream failed" });
      h.timers.shift()?.();
    }
    assert.equal(h.sources.length, 5);
    assert.match(h.states[5], /大纲生成失败/);
    assert.equal(h.states[0], false);
    assert.equal(h.states[1], false);
    assert.ok(h.sources.every(source => source.closed));
    run.hook.retry();
    assert.equal(h.states[6], 1);
    assert.deepEqual(h.outlines, []);
  } finally { run.finish(); }
});

test("empty completion and premature closing cannot report Outline ready", () => {
  for (const event of [{ type: "complete", presentation: { outlines: { slides: [] } } }, { type: "closing" }]) {
    const run = start();
    try {
      h.sources[0].emit(event);
      assert.notEqual(h.states[4], "Outline ready");
      assert.equal(h.timers.length, 1);
    } finally { run.finish(); }
  }
});

test("valid completed outlines stop retrying and remain available", () => {
  const run = start();
  try {
    const slides = [{ content: "作者简介" }, { content: "课文解析" }];
    h.sources[0].emit({ type: "complete", presentation: { outlines: { slides } } });
    assert.deepEqual(h.outlines, slides);
    assert.equal(h.states[4], "Outline ready");
    assert.equal(h.states[5], null);
    h.sources[0].onerror();
    assert.equal(h.timers.length, 0);
  } finally { run.finish(); }
});

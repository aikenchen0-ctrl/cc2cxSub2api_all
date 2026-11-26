import assert from "node:assert/strict";
import test from "node:test";
import { imageTaskStorageWithinDeadline } from "./image-task-polling";

test("completed image stays available when optional storage never responds", async () => {
    const result = await imageTaskStorageWithinDeadline(new Promise<never>(() => {}), 10);
    assert.equal(result, null);
});

test("optional storage failure does not turn a completed generation into a polling failure", async () => {
    assert.equal(await imageTaskStorageWithinDeadline(Promise.reject(new Error("storage offline"))), null);
});

test("successful storage keeps the uploaded image metadata", async () => {
    const image = { url: "/api/files/result/content", storageKey: "server:result" };
    assert.deepEqual(await imageTaskStorageWithinDeadline(Promise.resolve([image])), [image]);
});

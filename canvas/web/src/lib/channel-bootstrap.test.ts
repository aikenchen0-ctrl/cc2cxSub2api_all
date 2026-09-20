import assert from "node:assert/strict";
import test from "node:test";
import { readChannelBootstrap } from "./channel-bootstrap";

test("Sub2API fragment base URL overrides query base URL and legacy keys are scrubbed", () => {
    assert.deepEqual(readChannelBootstrap("http://localhost:3522/?baseUrl=https%3A%2F%2Fold.example&apiKey=old&tab=1#baseUrl=http%3A%2F%2Flocalhost%3A18080&apiKey=sk-test&view=canvas"), {
        baseUrl: "http://localhost:18080", cleanUrl: "/?tab=1#view=canvas",
    });
});
test("legacy query links and unrelated anchors remain supported", () => {
    assert.deepEqual(readChannelBootstrap("https://canvas.example/canvas?apikey=sk-test&baseurl=https%3A%2F%2Fapi.example#node-42"), {
        baseUrl: "https://api.example", cleanUrl: "/canvas#node-42",
    });
    assert.equal(readChannelBootstrap("https://canvas.example/#node-42"), null);
});

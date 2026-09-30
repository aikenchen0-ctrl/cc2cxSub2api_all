import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { autoDLVideoWorkflow, autoDLVideoWorkflows } from "./autodl-video-catalog";
import { getAutoDLCapabilities } from "./autodl-capabilities";

test("all sixteen default video workflows have Chinese labels and match the gateway contract", () => {
    const gateway = JSON.parse(readFileSync(new URL("../../../../sub2api/backend/internal/domain/autodl_video_workflows.json", import.meta.url), "utf8"));
    assert.deepEqual(autoDLVideoWorkflows, gateway);
    assert.equal(new Set(autoDLVideoWorkflows.map((item) => item.uuid)).size, 16);
    for (const workflow of autoDLVideoWorkflows) {
        assert.match(workflow.name, /[\u4e00-\u9fff]/);
        assert.equal(workflow.kind, "video");
        assert.ok(getAutoDLCapabilities(workflow));
    }
    assert.equal(autoDLVideoWorkflow("indextts2-v1"), undefined);
});

test("text, reference, frame and motion workflows expose their actual controls", () => {
    const text = getAutoDLCapabilities(autoDLVideoWorkflow("minimax_h3_z0901"));
    assert.equal(text?.imageMax, 0);
    assert.equal(text?.promptRequired, true);
    assert.equal(text?.duration?.max, 15);
    const multimodal = getAutoDLCapabilities(autoDLVideoWorkflow("minimax_h3_z0903"));
    assert.equal(multimodal?.imageMax, 6);
    assert.equal(multimodal?.audioMax, 3);
    assert.equal(multimodal?.imageMin, 1);
    assert.equal(multimodal?.audioMin, 1);
    const frames = getAutoDLCapabilities(autoDLVideoWorkflow("minimax_h3_lightx2v"));
    assert.equal(frames?.firstFrame, true);
    assert.equal(frames?.lastFrame, true);
    const motion = getAutoDLCapabilities(autoDLVideoWorkflow("wan2.2animate-v4-motion_retargeting"));
    assert.equal(motion?.imageMax, 1);
    assert.equal(motion?.videoMax, 1);
    assert.equal(motion?.imageMin, 1);
    assert.equal(motion?.videoMin, 1);
    assert.equal(motion?.promptRequired, false);
    assert.equal(motion?.duration, undefined);
});

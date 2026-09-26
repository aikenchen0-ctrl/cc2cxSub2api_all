import assert from "node:assert/strict";
import test from "node:test";
import { getAutoDLCapabilities } from "./autodl-capabilities";

test("AutoDL capabilities count documented indexed and unindexed reference rules", () => {
    const capabilities = getAutoDLCapabilities({
        uuid: "minimax_h3_z0903",
        name: "H3 video",
        kind: "video",
        input_rules: {
            prompt: { type: "string", required: true },
            duration: { type: "integer", default: 5, min: 1, max: 15 },
            resolution: { type: "select", default: "768p portrait", options: [{ label: "768p portrait" }] },
            ref_image_0: { type: "image", required: true },
            ref_image_1: { type: "image" },
            ref_audio_0: { type: "audio", required: true },
            ref_audio_1: { type: "audio" },
            ref_audio_2: { type: "audio" },
        },
    });
    assert.equal(capabilities?.imageMax, 2);
    assert.equal(capabilities?.audioMax, 3);
    assert.equal(capabilities?.duration?.default, 5);
    assert.equal(capabilities?.resolution?.default, "768p portrait");
});

test("AutoDL capabilities count unindexed and indexed video reference rules", () => {
    const capabilities = getAutoDLCapabilities({
        uuid: "workflow",
        name: "Video workflow",
        kind: "video",
        input_rules: { ref_video: { type: "video" }, ref_video_1: { type: "video" }, ref_audio: { type: "audio" } },
    });
    assert.equal(capabilities?.videoMax, 2);
    assert.equal(capabilities?.audioMax, 1);
});

import assert from "node:assert/strict";
import test from "node:test";
import { amamVideoDefaultResolution, amamVideoReferenceLimits, isAmamVideoModel, normalizeAmamVideoRatio, supportsAmamVideoRatio } from "./video-model-capabilities";

test("Amam presets are recognized with documented defaults and reference limits", () => {
    assert.equal(isAmamVideoModel("xinghe-2.0"), true);
    assert.equal(isAmamVideoModel("A-SD2.0"), true);
    assert.equal(isAmamVideoModel(`sd-2.5-30${String.fromCodePoint(31186)}`), true);
    assert.equal(isAmamVideoModel("zhiying-seedance-2.5-line2-1080p"), true);
    assert.equal(isAmamVideoModel("unrelated-video-model"), false);
    assert.equal(amamVideoDefaultResolution("xinghe-fast"), "480p");
    assert.equal(amamVideoDefaultResolution("A-SD2.0"), "720p");
    assert.equal(amamVideoDefaultResolution("zhiying-minimax-h3-2k"), "2k");
    assert.deepEqual(amamVideoReferenceLimits("zhiying-wan3.0-video"), { videos: 5, audios: 5 });
    assert.deepEqual(amamVideoReferenceLimits("zhiying-seedance-2.5-720p"), { videos: 10, audios: 10 });
    assert.deepEqual(amamVideoReferenceLimits("zhiying-google-omni-1080p"), { videos: 1, audios: 0 });
    assert.equal(normalizeAmamVideoRatio("xinghe-2.0", "1:1"), "16:9");
    assert.equal(normalizeAmamVideoRatio("xinghe-2.0", "720x1280"), "9:16");
    assert.equal(normalizeAmamVideoRatio("zhiying-minimax-h3-2k", "1024x1024"), "1:1");
    assert.equal(supportsAmamVideoRatio("xinghe-2.0", "720x1280"), true);
    assert.equal(supportsAmamVideoRatio("xinghe-2.0", "1024x1024"), false);
});

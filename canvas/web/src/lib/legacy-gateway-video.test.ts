import assert from "node:assert/strict";
import test from "node:test";
import { isLegacyGatewayVideoModel, legacyGatewayVideoBody, legacyGatewayVideoOptions } from "./legacy-gateway-video";

const defaults = { model: "seedance2.5-9图", prompt: "video test", seconds: "6", size: "16:9", resolution: "1080", images: ["data:image/png;base64,YQ=="], baseUrl: "" };

test("every displayed duration and fixed resolution matches the request", () => {
    for (const model of ["seedance2.5-9图", "seedance2.0-900-720p", "minimax-h3-933-图文"]) {
        const options = legacyGatewayVideoOptions(model);
        for (const duration of options.durations) {
            const body = legacyGatewayVideoBody({ ...defaults, model, seconds: String(duration) });
            assert.equal(body.seconds, String(duration));
            if (options.resolution) assert.equal(body.resolution, options.resolution + "p");
        }
        assert.equal(legacyGatewayVideoBody({ ...defaults, model, seconds: "invalid" }).seconds, String(options.defaultDuration));
    }
    assert.throws(() => legacyGatewayVideoOptions("seedance-2.5"));
});

test("custom model handling is exact and restricted to OpenAI channels", () => {
    for (const model of ["seedance2.5-9图", "seedance2.0-900-720p", "minimax-h3-933-图文"]) {
        assert.equal(isLegacyGatewayVideoModel(model, "openai"), true);
        for (const protocol of ["ark", "kie", "88api", "metaso", "autodl"]) assert.equal(isLegacyGatewayVideoModel(model, protocol), false);
    }
    assert.equal(isLegacyGatewayVideoModel(" SEEDANCE2.0-900-720P ", "openai"), true);
    for (const model of ["seedance-2.5", "seedance2.5-9图-fast", "minimax-h3", "sora-2"]) assert.equal(isLegacyGatewayVideoModel(model, "openai"), false);
});

test("Seedance 2.5 retains 30 seconds and 720p with portrait pixel inputs", () => {
    const body = legacyGatewayVideoBody({ ...defaults, size: "720x1280" });
    assert.equal(body.seconds, "30");
    assert.equal(body.duration, 30);
    assert.equal(body.resolution, "720p");
    assert.equal(body.size, "720x1280");
    assert.equal(body.aspect_ratio, "9:16");
    assert.deepEqual(body.images, defaults.images);
    assert.equal(body.reference_images, undefined);
});

test("Seedance 2.0 retains 10 or 15 second selection", () => {
    for (const [seconds, expected] of [["6", "10"], ["10", "10"], ["15", "15"], ["30", "10"], ["bad", "10"]]) {
        assert.equal(legacyGatewayVideoBody({ ...defaults, model: "seedance2.0-900-720p", seconds }).seconds, expected);
    }
});

test("MiniMax custom model retains duration and 768p dimensions", () => {
    for (const [seconds, expected] of [["5", "5"], ["10", "10"], ["15", "15"], ["30", "10"]]) {
        const body = legacyGatewayVideoBody({ ...defaults, model: "minimax-h3-933-图文", seconds });
        assert.equal(body.seconds, expected);
        assert.equal(body.resolution, "768p");
        assert.equal(body.size, "1344x768");
    }
    assert.equal(legacyGatewayVideoBody({ ...defaults, model: "minimax-h3-933-图文", size: "9:16" }).size, "768x1365");
});

test("direct Vivid Seedance uses pure-base64 reference_images", () => {
    const body = legacyGatewayVideoBody({ ...defaults, baseUrl: "https://aigc.easysu.cn/v1" });
    assert.deepEqual(body.reference_images, ["YQ=="]);
    assert.equal(body.ratio, "16:9");
    assert.equal(body.images, undefined);
    assert.equal(body.duration, undefined);
});

test("other custom Vivid models preserve data URL input references", () => {
    const body = legacyGatewayVideoBody({ ...defaults, model: "minimax-h3-933-图文", baseUrl: "https://aigc.easysu.cn" });
    assert.deepEqual(body.input_reference, defaults.images);
    assert.equal(body.reference_images, undefined);
});

test("Vivid detection does not match a hostname in a path or suffix", () => {
    for (const baseUrl of ["https://example.invalid/aigc.easysu.cn", "https://aigc.easysu.cn.example.invalid", "invalid"]) {
        assert.deepEqual(legacyGatewayVideoBody({ ...defaults, baseUrl }).images, defaults.images);
    }
});

test("reference count and automatic size match the old contract", () => {
    const body = legacyGatewayVideoBody({ ...defaults, model: "seedance2.0-900-720p", size: "auto", resolution: "low", images: Array(12).fill(defaults.images[0]) });
    assert.equal((body.images as string[]).length, 8);
    assert.equal(body.size, undefined);
    assert.equal(body.resolution, "480p");
    assert.equal(legacyGatewayVideoBody({ ...defaults, images: [] }).images, undefined);
});

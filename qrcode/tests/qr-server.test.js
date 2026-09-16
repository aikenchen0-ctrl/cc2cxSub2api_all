import test from "node:test";
import assert from "node:assert/strict";

import { createQrApp } from "../src/qr-app.js";

test("POST /api/generate should accept one qr image and return generated image data", async () => {
  let capturedArgs;
  const app = createQrApp({
    generate: async (args) => {
      capturedArgs = args;
      return {
        imageDataUrl: "data:image/png;base64,AAA",
        revisedPrompt: "fixed prompt"
      };
    }
  });
  const server = app.listen(0);

  try {
    const form = new FormData();
    form.append(
      "qrImage",
      new Blob([Buffer.from("fake-image")], { type: "image/png" }),
      "qr.png"
    );
    form.append(
      "templateImage",
      new Blob([Buffer.from("template-image")], { type: "image/png" }),
      "template.png"
    );
    form.append(
      "referenceImages",
      new Blob([Buffer.from("ref-image")], { type: "image/png" }),
      "ref.png"
    );
    form.append("positivePrompt", "POSITIVE");
    form.append("negativePrompt", "NEGATIVE");
    form.append("maskShape", "circle");

    const address = server.address();
    const response = await fetch(`http://127.0.0.1:${address.port}/api/generate`, {
      method: "POST",
      body: form
    });
    const data = await response.json();

    assert.equal(response.status, 200);
    assert.equal(data.imageDataUrl, "data:image/png;base64,AAA");
    assert.equal(data.revisedPrompt, "fixed prompt");
    assert.equal(capturedArgs.templateImage.mimeType, "image/png");
    assert.equal(capturedArgs.referenceImages.length, 1);
    assert.equal(capturedArgs.positivePrompt, "POSITIVE");
    assert.equal(capturedArgs.negativePrompt, "NEGATIVE");
    assert.equal(capturedArgs.maskPlacement.shape, "circle");
  } finally {
    await new Promise((resolve, reject) => {
      server.close((error) => {
        if (error) reject(error);
        else resolve();
      });
    });
  }
});

test("POST /api/generate should create an async job and expose pollable result", async () => {
  let resolveGenerate;
  let generateResolved = false;
  const generateDone = new Promise((resolve) => {
    resolveGenerate = () => {
      generateResolved = true;
      resolve();
    };
  });
  const app = createQrApp({
    generate: async () => {
      await generateDone;
      return {
        imageDataUrl: "data:image/png;base64,ASYNC",
        revisedPrompt: "async prompt",
        pipeline: { phase: "async-complete" }
      };
    }
  });
  const server = app.listen(0);

  try {
    const form = new FormData();
    form.append(
      "qrImage",
      new Blob([Buffer.from("fake-image")], { type: "image/png" }),
      "qr.png"
    );
    form.append("async", "true");

    const address = server.address();
    const submitResponse = await fetch(`http://127.0.0.1:${address.port}/api/generate`, {
      method: "POST",
      body: form
    });
    const submitData = await submitResponse.json();

    assert.equal(submitResponse.status, 202);
    assert.equal(submitData.status, "queued");
    assert.match(submitData.jobId, /^gen_/);

    const runningResponse = await fetch(`http://127.0.0.1:${address.port}/api/generate/jobs/${submitData.jobId}`);
    const runningData = await runningResponse.json();

    assert.equal(runningResponse.status, 200);
    assert.ok(["queued", "running"].includes(runningData.status));
    assert.equal(runningData.result, undefined);

    resolveGenerate();

    let completedData = null;
    for (let attempt = 0; attempt < 20; attempt += 1) {
      const pollResponse = await fetch(`http://127.0.0.1:${address.port}/api/generate/jobs/${submitData.jobId}`);
      completedData = await pollResponse.json();
      if (completedData.status === "succeeded") {
        break;
      }
      await new Promise((resolve) => setTimeout(resolve, 10));
    }

    assert.equal(completedData.status, "succeeded");
    assert.equal(completedData.result.imageDataUrl, "data:image/png;base64,ASYNC");
    assert.equal(completedData.result.pipeline.phase, "async-complete");
  } finally {
    if (!generateResolved) {
      resolveGenerate();
    }
    await new Promise((resolve, reject) => {
      server.close((error) => {
        if (error) reject(error);
        else resolve();
      });
    });
  }
});

test("POST /api/generate async jobs should omit debug image data from production results", async () => {
  const app = createQrApp({
    includeDebugPipeline: false,
    generate: async () => ({
      imageDataUrl: "data:image/png;base64,ASYNC",
      revisedPrompt: "async prompt",
      pipeline: {
        phase: "async-complete",
        editMode: "image2-edits-mask",
        templateUsed: true,
        referenceImageCount: 1,
        requestImageCount: 1,
        uploadContextMode: "single-mask-preview",
        squarePreviewDataUrl: "data:image/png;base64,SQUARE",
        squarePreviewUploadDataUrl: "data:image/png;base64,UPLOAD",
        debugOverlayDataUrl: "data:image/png;base64,DEBUG",
        editMaskDataUrl: "data:image/png;base64,MASK",
        collagePreviewDataUrl: "data:image/png;base64,COLLAGE",
        selectedSubmitAttempt: {
          httpStatus: 200,
          ok: true,
          responsePreview: "large-response-preview"
        }
      }
    })
  });
  const server = app.listen(0);

  try {
    const form = new FormData();
    form.append(
      "qrImage",
      new Blob([Buffer.from("fake-image")], { type: "image/png" }),
      "qr.png"
    );
    form.append("async", "true");

    const address = server.address();
    const submitResponse = await fetch(`http://127.0.0.1:${address.port}/api/generate`, {
      method: "POST",
      body: form
    });
    const submitData = await submitResponse.json();

    let completedData = null;
    for (let attempt = 0; attempt < 20; attempt += 1) {
      const pollResponse = await fetch(`http://127.0.0.1:${address.port}/api/generate/jobs/${submitData.jobId}`);
      completedData = await pollResponse.json();
      if (completedData.status === "succeeded") {
        break;
      }
      await new Promise((resolve) => setTimeout(resolve, 10));
    }

    const pipeline = completedData.result.pipeline;
    assert.equal(completedData.result.imageDataUrl, "data:image/png;base64,ASYNC");
    assert.equal(pipeline.phase, "production-slimmed");
    assert.equal(pipeline.debugPayloadOmitted, true);
    assert.equal(pipeline.editMode, "image2-edits-mask");
    assert.equal(pipeline.selectedSubmitAttempt.httpStatus, 200);
    assert.equal(pipeline.selectedSubmitAttempt.ok, true);
    assert.equal(pipeline.squarePreviewDataUrl, undefined);
    assert.equal(pipeline.squarePreviewUploadDataUrl, undefined);
    assert.equal(pipeline.debugOverlayDataUrl, undefined);
    assert.equal(pipeline.editMaskDataUrl, undefined);
    assert.equal(pipeline.collagePreviewDataUrl, undefined);
    assert.equal(pipeline.selectedSubmitAttempt.responsePreview, undefined);
  } finally {
    await new Promise((resolve, reject) => {
      server.close((error) => {
        if (error) reject(error);
        else resolve();
      });
    });
  }
});

test("POST /api/generate async jobs should respect server concurrency limit", async () => {
  const resolvers = [];
  let startedCount = 0;
  const app = createQrApp({
    generateConcurrency: 1,
    generate: async () => {
      startedCount += 1;
      await new Promise((resolve) => {
        resolvers.push(resolve);
      });
      return {
        imageDataUrl: `data:image/png;base64,JOB${startedCount}`,
        pipeline: { startedCount }
      };
    }
  });
  const server = app.listen(0);

  async function submitAsyncGenerate(port) {
    const form = new FormData();
    form.append(
      "qrImage",
      new Blob([Buffer.from("fake-image")], { type: "image/png" }),
      "qr.png"
    );
    form.append("async", "true");

    const response = await fetch(`http://127.0.0.1:${port}/api/generate`, {
      method: "POST",
      body: form
    });
    return response.json();
  }

  try {
    const address = server.address();
    const first = await submitAsyncGenerate(address.port);
    const second = await submitAsyncGenerate(address.port);

    await new Promise((resolve) => setTimeout(resolve, 20));

    const firstPoll = await fetch(`http://127.0.0.1:${address.port}/api/generate/jobs/${first.jobId}`);
    const firstData = await firstPoll.json();
    const secondPoll = await fetch(`http://127.0.0.1:${address.port}/api/generate/jobs/${second.jobId}`);
    const secondData = await secondPoll.json();

    assert.equal(startedCount, 1);
    assert.equal(firstData.status, "running");
    assert.equal(secondData.status, "queued");

    resolvers.shift()();

    let finalSecondData = null;
    for (let attempt = 0; attempt < 20; attempt += 1) {
      if (startedCount > 1 && resolvers.length > 0) {
        resolvers.shift()();
      }
      const pollResponse = await fetch(`http://127.0.0.1:${address.port}/api/generate/jobs/${second.jobId}`);
      finalSecondData = await pollResponse.json();
      if (finalSecondData.status === "succeeded") {
        break;
      }
      await new Promise((resolve) => setTimeout(resolve, 10));
    }

    assert.equal(finalSecondData.status, "succeeded");
    assert.equal(startedCount, 2);
  } finally {
    while (resolvers.length > 0) {
      resolvers.shift()();
    }
    await new Promise((resolve, reject) => {
      server.close((error) => {
        if (error) reject(error);
        else resolve();
      });
    });
  }
});

test("POST /api/stylize-qr should accept one normal qr image and return artified image data", async () => {
  let capturedArgs;
  const app = createQrApp({
    stylize: async (args) => {
      capturedArgs = args;
      return {
        imageBuffer: Buffer.from("styled-image"),
        mimeType: "image/png",
        task: {
          imgUuid: "uuid-123",
          status: 1,
          duration: 12.3,
          cost: 1,
          url: "https://example.com/a.png"
        },
        input: {
          qrTrim: { enabled: true, mode: "manual" },
          jpegQuality: 95,
          jpegBytes: 100,
          model: "69",
          prompt: "water ink"
        }
      };
    }
  });
  const server = app.listen(0);

  try {
    const form = new FormData();
    form.append(
      "qrImage",
      new Blob([Buffer.from("fake-image")], { type: "image/png" }),
      "normal.png"
    );
    form.append("qrTrimEnabled", "true");
    form.append("qrTrimMode", "manual");
    form.append("qrCropX", "10");
    form.append("qrCropY", "20");
    form.append("qrCropWidth", "300");
    form.append("qrCropHeight", "300");

    const address = server.address();
    const response = await fetch(`http://127.0.0.1:${address.port}/api/stylize-qr`, {
      method: "POST",
      body: form
    });
    const data = await response.json();

    assert.equal(response.status, 200);
    assert.match(data.imageDataUrl, /^data:image\/png;base64,/);
    assert.equal(data.task.imgUuid, "uuid-123");
    assert.equal(capturedArgs.qrTrim.mode, "manual");
    assert.equal(capturedArgs.qrTrim.x, 10);
    assert.equal(capturedArgs.qrTrim.y, 20);
    assert.equal(capturedArgs.qrTrim.width, 300);
    assert.equal(capturedArgs.qrTrim.height, 300);
  } finally {
    await new Promise((resolve, reject) => {
      server.close((error) => {
        if (error) reject(error);
        else resolve();
      });
    });
  }
});

test("POST /api/wechat/token should fetch access token with appid and secret", async () => {
  let capturedUrl = "";
  const app = createQrApp({
    wechatFetch: async (url) => {
      capturedUrl = url;
      return {
        ok: true,
        status: 200,
        text: async () => JSON.stringify({
          access_token: "ACCESS_TOKEN",
          expires_in: 7200
        })
      };
    }
  });
  const server = app.listen(0);

  try {
    const address = server.address();
    const response = await fetch(`http://127.0.0.1:${address.port}/api/wechat/token`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ appid: "APPID", secret: "SECRET" })
    });
    const data = await response.json();

    assert.equal(response.status, 200);
    assert.equal(data.access_token, "ACCESS_TOKEN");
    assert.equal(data.expires_in, 7200);
    assert.match(capturedUrl, /appid=APPID/);
    assert.match(capturedUrl, /secret=SECRET/);
    assert.match(capturedUrl, /grant_type=client_credential/);
  } finally {
    await new Promise((resolve, reject) => {
      server.close((error) => {
        if (error) reject(error);
        else resolve();
      });
    });
  }
});

test("OPTIONS /api/wechat/token should allow configured cross-origin API calls", async () => {
  const app = createQrApp({
    corsOrigin: "https://www.example.com"
  });
  const server = app.listen(0);

  try {
    const address = server.address();
    const response = await fetch(`http://127.0.0.1:${address.port}/api/wechat/token`, {
      method: "OPTIONS",
      headers: {
        Origin: "https://www.example.com",
        "Access-Control-Request-Method": "POST",
        "Access-Control-Request-Headers": "content-type"
      }
    });

    assert.equal(response.status, 204);
    assert.equal(response.headers.get("access-control-allow-origin"), "https://www.example.com");
    assert.match(response.headers.get("access-control-allow-methods") || "", /POST/);
    assert.match(response.headers.get("access-control-allow-headers") || "", /Content-Type/);
  } finally {
    await new Promise((resolve, reject) => {
      server.close((error) => {
        if (error) reject(error);
        else resolve();
      });
    });
  }
});

test("POST /api/wechat/token should explain WeChat IP whitelist failures", async () => {
  const app = createQrApp({
    wechatFetch: async () => ({
      ok: true,
      status: 200,
      text: async () => JSON.stringify({
        errcode: 40164,
        errmsg: "invalid ip 203.0.113.10 ipv6 ::ffff:203.0.113.10, not in whitelist"
      })
    })
  });
  const server = app.listen(0);

  try {
    const address = server.address();
    const response = await fetch(`http://127.0.0.1:${address.port}/api/wechat/token`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ appid: "APPID", secret: "SECRET" })
    });
    const data = await response.json();

    assert.equal(response.status, 424);
    assert.equal(data.errcode, 40164);
    assert.match(data.error, /IP 白名单/);
    assert.match(data.diagnostic, /203\.0\.113\.10/);
    assert.ok(data.actions.some((item) => /服务器出口 IP/.test(item)));
  } finally {
    await new Promise((resolve, reject) => {
      server.close((error) => {
        if (error) reject(error);
        else resolve();
      });
    });
  }
});

test("POST /api/wechat/token should explain server network failures", async () => {
  const app = createQrApp({
    wechatFetch: async () => {
      throw new Error("getaddrinfo ENOTFOUND api.weixin.qq.com");
    }
  });
  const server = app.listen(0);

  try {
    const address = server.address();
    const response = await fetch(`http://127.0.0.1:${address.port}/api/wechat/token`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ appid: "APPID", secret: "SECRET" })
    });
    const data = await response.json();

    assert.equal(response.status, 424);
    assert.match(data.error, /服务器无法连接微信接口/);
    assert.match(data.diagnostic, /出站网络/);
  } finally {
    await new Promise((resolve, reject) => {
      server.close((error) => {
        if (error) reject(error);
        else resolve();
      });
    });
  }
});

test("POST /api/wechat/material/upload should upload the generated image with a required filename", async () => {
  let capturedUrl = "";
  let capturedBody;
  const app = createQrApp({
    wechatFetch: async (url, options = {}) => {
      capturedUrl = url;
      capturedBody = options.body;
      return {
        ok: true,
        status: 200,
        text: async () => JSON.stringify({
          media_id: "MEDIA_ID",
          url: "https://example.com/material.png"
        })
      };
    }
  });
  const server = app.listen(0);

  try {
    const form = new FormData();
    form.append("accessToken", "ACCESS_TOKEN");
    form.append("filename", "final-name.png");
    form.append(
      "media",
      new Blob([Buffer.from("image-bytes")], { type: "image/png" }),
      "ignored-client-name.png"
    );

    const address = server.address();
    const response = await fetch(`http://127.0.0.1:${address.port}/api/wechat/material/upload`, {
      method: "POST",
      body: form
    });
    const data = await response.json();

    assert.equal(response.status, 200);
    assert.equal(data.media_id, "MEDIA_ID");
    assert.match(capturedUrl, /\/cgi-bin\/material\/add_material/);
    assert.doesNotMatch(capturedUrl, /\?/);
    assert.equal(capturedBody.get("access_token"), "ACCESS_TOKEN");
    assert.equal(capturedBody.get("type"), "image");
    assert.equal(capturedBody.get("media").name, "final-name.png");
  } finally {
    await new Promise((resolve, reject) => {
      server.close((error) => {
        if (error) reject(error);
        else resolve();
      });
    });
  }
});

test("POST /api/wechat/material/upload should reject requests without a media file", async () => {
  const app = createQrApp({
    wechatFetch: async () => {
      throw new Error("wechat should not be called");
    }
  });
  const server = app.listen(0);

  try {
    const form = new FormData();
    form.append("accessToken", "ACCESS_TOKEN");
    form.append("filename", "final-name.png");

    const address = server.address();
    const response = await fetch(`http://127.0.0.1:${address.port}/api/wechat/material/upload`, {
      method: "POST",
      body: form
    });
    const data = await response.json();

    assert.equal(response.status, 400);
    assert.match(data.error, /没有收到 media 图片文件/);
  } finally {
    await new Promise((resolve, reject) => {
      server.close((error) => {
        if (error) reject(error);
        else resolve();
      });
    });
  }
});

test("POST /api/wechat/material/prepare should return image size metadata and a reusable image", async () => {
  const app = createQrApp();
  const server = app.listen(0);

  try {
    const form = new FormData();
    form.append(
      "media",
      new Blob([Buffer.from("image-bytes")], { type: "image/png" }),
      "generated.png"
    );

    const address = server.address();
    const response = await fetch(`http://127.0.0.1:${address.port}/api/wechat/material/prepare`, {
      method: "POST",
      body: form
    });
    const data = await response.json();

    assert.equal(response.status, 200);
    assert.equal(data.originalBytes, Buffer.from("image-bytes").length);
    assert.equal(data.preparedBytes, Buffer.from("image-bytes").length);
    assert.equal(data.withinLimit, true);
    assert.match(data.imageDataUrl, /^data:image\/png;base64,/);
  } finally {
    await new Promise((resolve, reject) => {
      server.close((error) => {
        if (error) reject(error);
        else resolve();
      });
    });
  }
});

test("POST /api/wechat/material/list should fetch image materials with paging", async () => {
  let capturedUrl = "";
  let capturedBody = null;
  const app = createQrApp({
    wechatFetch: async (url, options = {}) => {
      capturedUrl = url;
      capturedBody = JSON.parse(options.body);
      return {
        ok: true,
        status: 200,
        text: async () => JSON.stringify({
          item: [{ media_id: "MEDIA_ID", name: "final-name.png", url: "https://example.com/material.png" }]
        })
      };
    }
  });
  const server = app.listen(0);

  try {
    const address = server.address();
    const response = await fetch(`http://127.0.0.1:${address.port}/api/wechat/material/list`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ accessToken: "ACCESS_TOKEN", offset: 5, count: 20 })
    });
    const data = await response.json();

    assert.equal(response.status, 200);
    assert.equal(data.item[0].media_id, "MEDIA_ID");
    assert.match(capturedUrl, /\/cgi-bin\/material\/batchget_material\?access_token=ACCESS_TOKEN$/);
    assert.deepEqual(capturedBody, { type: "image", offset: 5, count: 20 });
  } finally {
    await new Promise((resolve, reject) => {
      server.close((error) => {
        if (error) reject(error);
        else resolve();
      });
    });
  }
});

test("POST /api/wechat/material/get should return image data for a media id", async () => {
  let capturedUrl = "";
  let capturedBody = null;
  const app = createQrApp({
    wechatFetch: async (url, options = {}) => {
      capturedUrl = url;
      capturedBody = JSON.parse(options.body);
      return {
        ok: true,
        status: 200,
        headers: new Headers({ "content-type": "image/png" }),
        arrayBuffer: async () => Buffer.from("image-bytes").buffer.slice(
          Buffer.from("image-bytes").byteOffset,
          Buffer.from("image-bytes").byteOffset + Buffer.from("image-bytes").byteLength
        )
      };
    }
  });
  const server = app.listen(0);

  try {
    const address = server.address();
    const response = await fetch(`http://127.0.0.1:${address.port}/api/wechat/material/get`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ accessToken: "ACCESS_TOKEN", mediaId: "MEDIA_ID" })
    });
    const data = await response.json();

    assert.equal(response.status, 200);
    assert.match(capturedUrl, /\/cgi-bin\/material\/get_material\?access_token=ACCESS_TOKEN$/);
    assert.deepEqual(capturedBody, { media_id: "MEDIA_ID" });
    assert.equal(data.mimeType, "image/png");
    assert.match(data.imageDataUrl, /^data:image\/png;base64,/);
  } finally {
    await new Promise((resolve, reject) => {
      server.close((error) => {
        if (error) reject(error);
        else resolve();
      });
    });
  }
});

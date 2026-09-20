import test from "node:test";
import assert from "node:assert/strict";

test("APP_CONFIG should read server port from PORT environment variable", async () => {
  const originalPort = process.env.PORT;
  process.env.PORT = "4567";

  try {
    const { APP_CONFIG } = await import(`../src/config.js?port-env-${Date.now()}`);
    assert.equal(APP_CONFIG.port, 4567);
  } finally {
    if (originalPort === undefined) {
      delete process.env.PORT;
    } else {
      process.env.PORT = originalPort;
    }
  }
});

test("APP_CONFIG should use port 5221 when PORT is not set", async () => {
  const originalPort = process.env.PORT;
  delete process.env.PORT;

  try {
    const { APP_CONFIG } = await import(`../src/config.js?default-port-${Date.now()}`);
    assert.equal(APP_CONFIG.port, 5221);
  } finally {
    if (originalPort === undefined) {
      delete process.env.PORT;
    } else {
      process.env.PORT = originalPort;
    }
  }
});

test("APP_CONFIG should read generate timeout and concurrency from environment", async () => {
  const originalTimeout = process.env.QR_GENERATE_TIMEOUT_MS;
  const originalConcurrency = process.env.QR_GENERATE_CONCURRENCY;
  process.env.QR_GENERATE_TIMEOUT_MS = "900000";
  process.env.QR_GENERATE_CONCURRENCY = "2";

  try {
    const { APP_CONFIG } = await import(`../src/config.js?generate-env-${Date.now()}`);
    assert.equal(APP_CONFIG.generateTimeoutMs, 900000);
    assert.equal(APP_CONFIG.generateConcurrency, 2);
  } finally {
    if (originalTimeout === undefined) {
      delete process.env.QR_GENERATE_TIMEOUT_MS;
    } else {
      process.env.QR_GENERATE_TIMEOUT_MS = originalTimeout;
    }

    if (originalConcurrency === undefined) {
      delete process.env.QR_GENERATE_CONCURRENCY;
    } else {
      process.env.QR_GENERATE_CONCURRENCY = originalConcurrency;
    }
  }
});

test("APP_CONFIG should omit debug pipeline by default in production", async () => {
  const originalNodeEnv = process.env.NODE_ENV;
  const originalDebugPipeline = process.env.QR_INCLUDE_DEBUG_PIPELINE;
  process.env.NODE_ENV = "production";
  delete process.env.QR_INCLUDE_DEBUG_PIPELINE;

  try {
    const { APP_CONFIG } = await import(`../src/config.js?debug-prod-${Date.now()}`);
    assert.equal(APP_CONFIG.includeDebugPipeline, false);
  } finally {
    if (originalNodeEnv === undefined) {
      delete process.env.NODE_ENV;
    } else {
      process.env.NODE_ENV = originalNodeEnv;
    }

    if (originalDebugPipeline === undefined) {
      delete process.env.QR_INCLUDE_DEBUG_PIPELINE;
    } else {
      process.env.QR_INCLUDE_DEBUG_PIPELINE = originalDebugPipeline;
    }
  }
});

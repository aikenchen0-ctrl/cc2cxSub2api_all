import test from "node:test";
import assert from "node:assert/strict";
import net from "node:net";
import express from "express";

import { startServer } from "../src/server-runtime.js";

async function listenOn(port) {
  const server = net.createServer();

  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(port, "127.0.0.1", resolve);
  });

  return server;
}

async function closeServer(server) {
  if (!server?.listening) {
    return;
  }

  await new Promise((resolve, reject) => {
    server.close((error) => {
      if (error) {
        reject(error);
        return;
      }

      resolve();
    });
  });
}

async function findFreePortPair() {
  for (let port = 41000; port < 42000; port += 2) {
    let first;
    let second;

    try {
      first = await listenOn(port);
      second = await listenOn(port + 1);
      return port;
    } catch {
      // 端口探测只用于测试隔离，失败后继续尝试下一组。
    } finally {
      await closeServer(second);
      await closeServer(first);
    }
  }

  throw new Error("未找到可用于测试的连续端口");
}

test("首选端口被占用时自动监听下一个可用端口", async () => {
  const preferredPort = await findFreePortPair();
  const blocker = await listenOn(preferredPort);
  const app = express();
  const messages = [];

  try {
    const server = await startServer({
      app,
      preferredPort,
      host: "127.0.0.1",
      maxAttempts: 2,
      logger: {
        log(message) {
          messages.push(message);
        },
        warn(message) {
          messages.push(message);
        }
      }
    });

    try {
      assert.equal(server.address().port, preferredPort + 1);
      assert.ok(messages.some((message) => message.includes(`http://127.0.0.1:${preferredPort + 1}`)));
    } finally {
      await closeServer(server);
    }
  } finally {
    await closeServer(blocker);
  }
});

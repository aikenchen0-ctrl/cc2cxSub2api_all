const DEFAULT_HOST = "0.0.0.0";
const DEFAULT_MAX_ATTEMPTS = 20;

function formatPublicHost(host) {
  return host === "0.0.0.0" || host === "::" ? "127.0.0.1" : host;
}

function listenOnce(app, port, host) {
  return new Promise((resolve, reject) => {
    const server = app.listen(port, host);

    function cleanup() {
      server.off("listening", onListening);
      server.off("error", onError);
    }

    function onListening() {
      cleanup();
      resolve(server);
    }

    function onError(error) {
      cleanup();
      reject(error);
    }

    server.once("listening", onListening);
    server.once("error", onError);
  });
}

export async function startServer({
  app,
  preferredPort,
  host = DEFAULT_HOST,
  maxAttempts = DEFAULT_MAX_ATTEMPTS,
  logger = console
}) {
  if (!app) {
    throw new Error("启动服务失败：缺少 Express 应用实例");
  }

  if (!Number.isInteger(preferredPort) || preferredPort <= 0) {
    throw new Error("启动服务失败：端口必须是正整数");
  }

  for (let offset = 0; offset < maxAttempts; offset += 1) {
    const port = preferredPort + offset;

    try {
      const server = await listenOnce(app, port, host);
      const publicHost = formatPublicHost(host);

      if (port !== preferredPort) {
        logger.warn(`端口 ${preferredPort} 已被占用，已自动切换到 ${port}`);
      }

      logger.log(`Server running at http://${publicHost}:${port}`);
      return server;
    } catch (error) {
      if (error.code === "EADDRINUSE") {
        continue;
      }

      throw error;
    }
  }

  const lastPort = preferredPort + maxAttempts - 1;
  throw new Error(`启动服务失败：端口 ${preferredPort} 到 ${lastPort} 均被占用`);
}

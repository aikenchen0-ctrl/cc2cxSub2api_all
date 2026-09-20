import { createQrApp } from "./qr-app.js";
import { APP_CONFIG } from "./config.js";
import { startServer } from "./server-runtime.js";

const app = createQrApp();

await startServer({
  app,
  preferredPort: APP_CONFIG.port
});

module.exports = {
  apps: [
    {
      name: 'openbidkit-yibiao-web',
      cwd: '/opt/openbidkit-yibiao-web/server',
      script: './node_modules/.bin/tsx',
      args: 'src/index.ts',
      interpreter: 'none',
      env: {
        NODE_ENV: 'production',
        HOST: '127.0.0.1',
        PORT: '3000',
        YIBIAO_DATA_DIR: '/opt/openbidkit-yibiao-web/data',
      },
      max_memory_restart: '2G',
      autorestart: true,
      time: true,
    },
    {
      name: 'yibiao-bidmonitor',
      cwd: '/opt/openbidkit-yibiao-web/bid-monitor',
      script: '/opt/openbidkit-yibiao-web/bid-monitor/.venv/bin/python',
      args: '-m uvicorn service.app:app --host 127.0.0.1 --port 8080',
      interpreter: 'none',
      env: {
        PYTHONUNBUFFERED: '1',
        BID_MONITOR_HOST: '127.0.0.1',
        BID_MONITOR_PORT: '8080',
        BID_MONITOR_DATA_ROOT: '/var/lib/yibiao-bidmonitor/users',
        BID_MONITOR_SERVICE_TOKEN: 'replace-with-a-random-internal-token',
      },
      max_memory_restart: '1G',
      autorestart: true,
      time: true,
    },
  ],
};

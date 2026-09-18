// 测试进程只使用临时密钥，避免依赖开发者本机环境文件。
if (!process.env.JWT_SECRET) process.env.JWT_SECRET = 'test-jwt-secret-with-at-least-32-characters-123';

import { urlToQrImage, isValidUrl } from './src/url-to-qr.js';
import fs from 'fs';
import path from 'path';

async function test() {
  console.log('测试URL转二维码功能...\n');
  
  // 测试URL验证
  console.log('1. 测试URL验证:');
  console.log('  https://example.com:', isValidUrl('https://example.com'));
  console.log('  http://test.com:', isValidUrl('http://test.com'));
  console.log('  invalid-url:', isValidUrl('invalid-url'));
  console.log('  empty:', isValidUrl(''));
  console.log();
  
  // 测试生成二维码
  console.log('2. 测试生成二维码:');
  try {
    const result = await urlToQrImage('https://example.com', {
      width: 512,
      margin: 4
    });
    
    console.log('  生成成功!');
    console.log('  文件大小:', result.buffer.length, 'bytes');
    console.log('  MIME类型:', result.mimeType);
    console.log('  文件名:', result.filename);
    
    // 保存测试文件
    const outputPath = path.join('output', 'test-url-qr.png');
    fs.writeFileSync(outputPath, result.buffer);
    console.log('  已保存到:', outputPath);
  } catch (error) {
    console.error('  生成失败:', error.message);
  }
}

test().catch(console.error);

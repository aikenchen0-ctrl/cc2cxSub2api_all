import fetch from 'node-fetch';

async function testApi() {
  console.log('测试URL转二维码API...\n');
  
  const testCases = [
    {
      name: '有效URL - HTTPS',
      url: 'https://www.example.com',
      shouldSuccess: true
    },
    {
      name: '有效URL - HTTP',
      url: 'http://test.com',
      shouldSuccess: true
    },
    {
      name: '无效URL - 缺少协议',
      url: 'www.example.com',
      shouldSuccess: false
    },
    {
      name: '空URL',
      url: '',
      shouldSuccess: false
    }
  ];
  
  for (const testCase of testCases) {
    console.log(`测试: ${testCase.name}`);
    console.log(`URL: ${testCase.url || '(空)'}`);
    
    try {
      const response = await fetch('http://127.0.0.1:3100/api/url-to-qr', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ url: testCase.url })
      });
      
      const data = await response.json();
      
      if (response.ok) {
        if (testCase.shouldSuccess) {
          console.log('✅ 成功');
          console.log(`   文件大小: ${data.imageDataUrl.length} bytes`);
          console.log(`   MIME类型: ${data.mimeType}`);
          console.log(`   文件名: ${data.filename}`);
        } else {
          console.log('❌ 意外成功（应该失败）');
        }
      } else {
        if (!testCase.shouldSuccess) {
          console.log('✅ 正确拒绝');
          console.log(`   错误信息: ${data.error}`);
        } else {
          console.log('❌ 意外失败');
          console.log(`   错误信息: ${data.error}`);
        }
      }
    } catch (error) {
      console.log('❌ 请求失败:', error.message);
    }
    
    console.log();
  }
}

testApi().catch(console.error);

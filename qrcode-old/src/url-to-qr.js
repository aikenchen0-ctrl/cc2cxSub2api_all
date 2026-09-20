import QRCode from 'qrcode';
import sharp from 'sharp';

/**
 * 将URL转换为二维码图片Buffer
 * @param {string} url - 要转换的URL
 * @param {object} options - 配置选项
 * @returns {Promise<Buffer>} 二维码图片Buffer
 */
export async function urlToQrImage(url, options = {}) {
  const {
    width = 512,
    margin = 4,
    color = {
      dark: '#000000',
      light: '#ffffff'
    }
  } = options;

  try {
    // 使用qrcode库生成DataURL
    const dataUrl = await QRCode.toDataURL(url, {
      width,
      margin,
      color: {
        dark: color.dark,
        light: color.light
      },
      errorCorrectionLevel: 'H' // 高容错率，适合艺术化处理
    });

    // 将DataURL转换为Buffer
    const base64Data = dataUrl.split(',')[1];
    const buffer = Buffer.from(base64Data, 'base64');

    // 使用sharp处理图片，确保格式统一为PNG
    const processedBuffer = await sharp(buffer)
      .png()
      .toBuffer();

    return {
      buffer: processedBuffer,
      mimeType: 'image/png',
      filename: `qr-${Date.now()}.png`
    };
  } catch (error) {
    throw new Error(`URL转二维码失败: ${error.message}`);
  }
}

/**
 * 验证URL格式
 * @param {string} url - 待验证的URL
 * @returns {boolean} 是否为有效URL
 */
export function isValidUrl(url) {
  if (!url || typeof url !== 'string') {
    return false;
  }

  try {
    new URL(url);
    return true;
  } catch {
    return false;
  }
}

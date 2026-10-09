import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import ts from 'typescript';

const source = readFileSync(new URL('../services/errorMessages.ts', import.meta.url), 'utf8');
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext } }).outputText;
const { getFailureDetails, createApiError } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`);

const cases = [
  ['上游 AI 接口错误：Upstream request failed（状态 502，type=upstream_error，requestId=eaaceea5-fd4e-4d89-b5e8-4db225479f22）', /可能是.*模型权限/, /请稍后重试/],
  ['Spring AI 上游错误：{"error":{"message":"Upstream error: 404","type":"not_found_error"}}（状态 404）', /找不到当前模型/, /服务地址/],
  ['上游 AI 接口错误：Insufficient account balance（状态 403）', /通道余额/, /不代表你的主站余额不足/],
  ['你的账户余额不足', /你的账户余额/, /前往主站/],
  ['上游错误：API Key 所属分组已删除（状态 403）', /删除或停用/, /管理员/],
  ['Model "gpt-image-2" is not available for this group', /没有开通当前模型/, /模型权限/],
  ['No available channel for model gpt-image-2', /可用通道/, /管理员/],
  ['Upstream access forbidden, please contact administrator（状态 502）', /拒绝了访问/, /权限/],
  ['上游 AI 接口错误（状态 401）', /服务账号可能已失效/, /密钥/],
  ['图片请求失败：401', /登录状态已失效/, /重新登录/],
  ['调用失败（状态 405）', /不支持这类操作/, /本次操作/],
  ['图片上传失败（状态 413）', /允许的大小/, /压缩/],
  ['调用失败（状态 429）', /限制了调用次数/, /等待几十秒/],
  ['Spring AI 请求超时（状态 504）', /超时/, /查看画布/],
  ['Failed to fetch', /暂时连不上/, /避免重复提交/],
  ['任务不存在或已过期', /记录已过期/, /是否已有结果/],
  ['图片无效（code=INVALID_IMAGE_DATA_URL）', /无法读取/, /重新上传/],
  ['文件不支持（code=UNSUPPORTED_FILE_TYPE）', /格式暂时不支持/, /PNG/],
  ['服务未配置（code=USER_API_CONFIG_REQUIRED）', /连接信息还没有配置好/, /使用主站服务/],
  ['未能从响应中获取图像数据', /结果不完整/, /重新提交/],
  ['context_length_exceeded', /超出了 AI/, /精简描述/],
  ['安全限制（code=POSSIBLE_CONTENT_POLICY_BLOCKED）', /内容安全限制/, /调整/],
  ['登录失败（code=INVALID_CREDENTIALS，状态 401）', /用户名或密码不正确/, /重新登录/],
  ['解析失败（code=EXTERNAL_IMAGES_EMPTY）', /链接获取到图片/, /直接上传/],
  ['下载失败（code=EXPORT_NOT_FOUND）', /文件已找不到/, /重新导出/],
  ['上传失败（code=MINIO_UNAVAILABLE，状态 500）', /服务暂时出了问题/, /管理员/],
  ['unexpected opaque failure', /无法确认具体原因/, /排查详情/],
  ['复制失败', /浏览器没有完成复制/, /剪贴板权限/]
];

for (const [raw, reason, action] of cases) {
  test(raw, () => {
    const failure = getFailureDetails(raw);
    assert.match(failure.message, reason);
    assert.match(failure.message, action);
    assert.doesNotMatch(failure.message, /requestId=|code=|type=|状态 \d{3}|Spring AI|Upstream/);
  });
}

test('preserves request ID only in expandable details', () => {
  const failure = getFailureDetails(cases[0][0]);
  assert.match(failure.metadata, /请求编号 eaaceea5-fd4e-4d89-b5e8-4db225479f22/);
  assert.match(failure.details, /eaaceea5-fd4e-4d89-b5e8-4db225479f22/);
});

test('API envelope keeps code for correct classification', () => {
  const error = createApiError({ error: { message: '验证失败', code: 'INVALID_CREDENTIALS' } }, 401, '操作失败');
  assert.match(getFailureDetails(error).message, /用户名或密码不正确/);
});

test('local input validation stays specific', () => {
  assert.equal(getFailureDetails('请先输入产品特点').message, '请先输入产品特点');
  assert.match(getFailureDetails(createApiError({ error: { message: '用户名只能包含字母和数字', code: 'VALIDATION_ERROR' } }, 400, '')).message, /用户名只能包含字母和数字/);
});

test('technical details redact credentials and internal URLs', () => {
  const failure = getFailureDetails('Upstream error: 502 https://private.internal/v1?api_key=secret sk-example-secret Bearer private-credential api_key=secret');
  assert.doesNotMatch(failure.details, /private\.internal|sk-example|private-credential|api_key=secret/);
});

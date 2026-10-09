type ErrorDetails = {
  status?: number;
  requestId?: string;
  code?: string;
  type?: string;
};

const getMessage = (error: unknown) => {
  if (error instanceof Error) return error.message;
  if (typeof error === 'string') return error;
  if (error && typeof error === 'object') {
    const value = error as Record<string, unknown>;
    if (typeof value.message === 'string') return value.message;
    if (value.error) return getMessage(value.error);
    if (typeof value.detail === 'string') return value.detail;
  }
  return '';
};

const getDetails = (message: string): ErrorDetails => {
  const statusMatch = message.match(/(?:状态|status(?:_code)?|upstream error|失败)\s*[=:：]?\s*(\d{3})\b/i);
  const requestMatch = message.match(/(?:requestId|request[_ -]?id)\s*[=:：]?\s*([\w-]+)/i);
  const codeMatch = message.match(/(?:code|错误代码)\s*[=:：]?\s*([\w.-]+)/i);
  const typeMatch = message.match(/(?:type|类型)\s*[=:：]?\s*([\w.-]+)/i);
  return {
    status: statusMatch ? Number(statusMatch[1]) : undefined,
    requestId: requestMatch?.[1],
    code: codeMatch?.[1],
    type: typeMatch?.[1]
  };
};

export const createApiError = (payload: unknown, status: number, fallback: string) => {
  const envelope = payload && typeof payload === 'object' ? payload as Record<string, unknown> : {};
  const error = envelope.error && typeof envelope.error === 'object'
    ? envelope.error as Record<string, unknown> : envelope;
  const message = getMessage(payload) || fallback;
  const fields = [
    `状态 ${status}`,
    typeof error.code === 'string' ? `code=${error.code}` : '',
    typeof error.type === 'string' ? `type=${error.type}` : '',
    typeof envelope.requestId === 'string' ? `requestId=${envelope.requestId}` : ''
  ].filter(Boolean);
  return new Error(`${message}（${fields.join('，')}）`);
};

const redactTechnicalMessage = (message: string) => message
  .replace(/\bsk-[\w-]+/gi, '[已隐藏密钥]')
  .replace(/Bearer\s+[^\s"，）]+/gi, 'Bearer [已隐藏凭据]')
  .replace(/https?:\/\/[^\s"<>，）]+/gi, '[已隐藏服务地址]')
  .replace(/(?:api[_-]?key|access[_-]?token|refresh[_-]?token|password|secret|authorization)\s*[=:：]\s*[^\s，）]+/gi, '[已隐藏凭据]')
  .replace(/eyJ[\w-]+\.[\w-]+\.[\w-]+/g, '[已隐藏凭据]')
  .slice(0, 1600);

export const getFailureDetails = (error: unknown, context?: string) => {
  const raw = getMessage(error).trim();
  const formatted = formatUserFacingError(raw, undefined, context);
  const [message, metadata] = formatted.split('\n\n技术信息：');
  return {
    message,
    details: (metadata || formatted !== raw) ? redactTechnicalMessage(raw) : '',
    metadata
  };
};

const technicalSuffix = (details: ErrorDetails) => {
  const parts = [
    details.status ? `状态 ${details.status}` : '',
    details.code ? `代码 ${details.code}` : '',
    details.type ? `类型 ${details.type}` : '',
    details.requestId ? `请求编号 ${details.requestId}` : ''
  ].filter(Boolean);
  return parts.length > 0 ? `\n\n技术信息：${parts.join('，')}` : '';
};

/** Convert gateway/provider errors into actionable Chinese text for end users. */
export const formatUserFacingError = (
  error: unknown,
  fallback = '操作没有完成，请稍后再试。',
  context = '这项操作'
) => {
  const raw = getMessage(error).trim();
  if (!raw) return fallback;
  const details = getDetails(raw);
  const status = details.status;
  const upstream = /上游|upstream|spring ai|AI 接口/i.test(raw);
  const explain = (reason: string, action: string) => `${reason}\n${action}${technicalSuffix(details)}`;

  if (/invalid_credentials|用户名或密码.*(错误|不正确)/i.test(raw)) {
    return explain('用户名或密码不正确。', '请检查输入后重新登录。');
  }
  if (/username_exists|用户名.*已存在/i.test(raw)) {
    return explain('这个用户名已经被注册了。', '请换一个用户名，或使用已有账号登录。');
  }
  if (/sso_ticket|sso_not_configured/i.test(raw)) {
    return explain('登录链接已失效，或登录服务尚未配置好。', '请从主站菜单重新进入超级改图；仍无法进入时请联系管理员。');
  }
  if (/image_job_not_found|agent_run_not_found|图片任务不存在|任务不存在或已过期/i.test(raw)) {
    return explain('这项任务的记录已过期，或在服务重启后无法找回。', '请检查画布是否已有结果；没有结果时可以重新提交。');
  }
  if (/asset_not_found|canvas_not_found|export_not_found/i.test(raw)) {
    return explain('需要的图片、项目或下载文件已找不到。', '请刷新项目列表，重新选择图片或重新导出文件。');
  }
  if (/too large|太大|超过.*大小|upload_too_large|asset_too_large|external_image_too_large/i.test(raw) || status === 413) {
    return explain('图片或请求内容超过了允许的大小。', '请压缩图片、减少参考图数量，或降低图片分辨率后重试。');
  }
  if (/unsupported_file_type|asset_rotate_unsupported|文件.*不支持|格式.*不支持/i.test(raw)) {
    return explain('这份文件的格式暂时不支持。', '请转成 PNG、JPG 或 WebP 图片后重新上传。');
  }
  if (/agent_.*(?:image.*required|image.*missing|mask_required)|product_poster_image_missing|empty_file/i.test(raw)) {
    return explain('还缺少这次操作需要的图片或涂抹区域。', '请先选中或上传参考图；局部编辑时先涂抹要修改的位置。');
  }
  if (/invalid_base_url|user_api_config_required|invalid_user_api_config|server_default_api_config_required/i.test(raw)) {
    return explain('AI 连接信息还没有配置好，暂时无法调用。', '使用主站服务时请联系管理员；使用个人服务时请检查连接设置里的服务地址、密钥和模型。');
  }
  if (/external_images_api_key_missing|knowledge_embedding_config_missing|知识库.*配置缺失/i.test(raw)) {
    return explain('这项功能需要的服务还没有配置好。', '请联系管理员开通或检查配置后再试。');
  }
  if (/invalid_url|asset_url_|external_image_(?:private_url|host_invalid|redirect)|链接.*无效/i.test(raw)) {
    return explain('这个图片或分享链接无法访问。', '请换一个公开可访问的完整链接，或下载图片后直接上传。');
  }
  if (/invalid_image_data|agent_image_asset_invalid|invalid.*image|图片.*损坏/i.test(raw)) {
    return explain('图片无法读取，可能已损坏或上传未完成。', '请重新上传图片，上传完成后再试。');
  }
  if (/context[_ ](?:length|window)|上下文.*超|maximum context/i.test(raw)) {
    return explain('这次对话或参考内容太多，超出了 AI 一次能处理的范围。', '请精简描述、减少参考图，或新建项目后再试。');
  }
  if (/empty_text|未能从响应中获取图像|invalid_json|parse_failed|plan_empty|response_invalid/i.test(raw)) {
    return explain('AI 返回了结果，但结果不完整，暂时无法使用。', '请重新提交一次；持续失败时请联系管理员检查模型是否支持这项功能。');
  }

  if (/possible_content_policy_blocked|content[_ -]?policy|safety|敏感|内容政策|不符合.*规范/i.test(raw)) {
    return explain('AI 服务因内容安全限制没有完成这次请求。', '请调整可能敏感的描述，或换一张参考图片后重试。');
  }
  if (/insufficient account balance|余额不足|insufficient.*balance|insufficient_quota/i.test(raw)) {
    return upstream || /insufficient account balance/i.test(raw)
      ? explain('提供 AI 服务的通道余额或额度不足，暂时无法处理。', '请稍后重试；仍失败时请联系管理员补充通道额度。此提示不代表你的主站余额不足。')
      : explain('你的账户余额或可用额度不足，暂时无法继续使用 AI。', '请前往主站检查余额和套餐，充值或补充额度后重试。');
  }
  if (/所属分组已删除|group.*deleted|分组.*删除/i.test(raw)) {
    return explain('提供 AI 服务的账号所属通道已被删除或停用。', '请联系管理员重新配置可用通道后再试。');
  }
  if (/model .*not (?:available|found)|model_not_found|unsupported_model|没有模型|no available (?:channel|accounts)|没有可用.*渠道|model_match=0|模型.*不存在/i.test(raw)) {
    return explain('提供 AI 服务的账号没有开通当前模型，或这个模型没有可用通道。', '请稍后重试；持续失败时请联系管理员检查模型权限和通道配置。');
  }
  if (/upstream access forbidden|access forbidden|访问被禁止/i.test(raw)) {
    return explain('提供 AI 服务的通道拒绝了访问，可能已失效或没有权限。', '请联系管理员检查通道的权限和可用状态。');
  }
  if ((status === 401 && !upstream) || /未登录|登录已过期|auth_required|invalid_token|jwt_expired/i.test(raw)) {
    return explain('登录状态已失效，暂时无法继续操作。', '请从主站菜单重新进入超级改图，或重新登录后再试。');
  }
  if ((status === 401 || status === 403) && upstream) {
    return explain('AI 服务拒绝了访问，服务账号可能已失效或没有调用权限。', '请联系管理员检查 AI 通道；使用个人服务时请检查连接设置中的密钥。');
  }
  if (status === 403) {
    return explain('当前账号没有权限执行这项操作。', '请确认进入的是自己的项目；仍失败时请联系管理员检查账号权限。');
  }
  if (status === 404 && upstream) {
    return explain('AI 服务找不到当前模型或调用入口。', '请联系管理员检查模型名称、服务地址和接口类型；使用个人服务时请检查连接设置。');
  }
  if (status === 404) {
    return explain('需要的任务、图片或页面已找不到，可能已过期或被删除。', '请刷新页面后重新选择；任务没有结果时可以重新提交。');
  }
  if (status === 405 || /method not allowed|不支持.*(接口|方法|端点)|endpoint.*not supported/i.test(raw)) {
    return explain('当前服务不支持这类操作，例如通道支持生图但不支持编辑。', '请联系管理员检查通道是否支持本次操作；使用个人服务时请检查连接设置。');
  }
  if (status === 429 || /rate.?limit|too many|频繁|限流/i.test(raw)) {
    return explain('服务当前请求太多，暂时限制了调用次数。', '请等待几十秒后再试，连续点击会延长等待时间。');
  }
  if (status === 504 || status === 408 || /timeout|timed out|超时/i.test(raw)) {
    return explain('等待服务回复的时间太长，这次操作超时了。', '请先查看画布是否已有结果；没有结果时稍后重试，或降低图片分辨率。');
  }
  if (/failed to fetch|networkerror|load failed|网络|连接.*失败|连接.*断开/i.test(raw)) {
    return explain('暂时连不上服务，可能是网络中断或服务正在重启。', '请检查网络并稍后刷新页面；先查看是否已有结果，避免重复提交。');
  }
  if (status === 502 || status === 503 || /upstream|bad gateway|service unavailable|上游.*错误/i.test(raw)) {
    return explain('AI 服务没有完成这次请求。可能是服务暂时不可用，或服务账号缺少模型权限、额度不足、连接配置有问题。', '请稍后重试；反复失败时请联系管理员检查 AI 通道。服务未提供具体原因，不能据此判断是你操作有误或个人余额不足。');
  }
  if (status === 500 || /internal_error|minio_unavailable/i.test(raw)) {
    return explain('超级改图的服务暂时出了问题，这次操作没有完成。', '请稍后重试；反复失败时请联系管理员，并提供下方请求编号或出错时间。');
  }
  if (/复制.*失败|clipboard|notallowederror/i.test(raw)) {
    return explain('浏览器没有完成复制，可能未允许剪贴板访问。', '请检查浏览器的剪贴板权限，或手动选择文字复制。');
  }
  if (/external.*(?:failed|empty)|没有从.*解析到图片/i.test(raw)) {
    return explain('没有从这个链接获取到图片，链接可能已失效或限制访问。', '请换一个公开链接，或下载图片后直接上传。');
  }
  if (/asset_(?:upload|download|read|rotate)_failed|export_create_failed|导出.*失败|下载.*失败|图片加载失败|裁剪失败|旋转图片失败/i.test(raw)) {
    return explain('图片读取、保存或导出没有完成，文件或存储服务可能暂时不可用。', '请稍后重试；图片无法打开时请重新上传，持续失败时请联系管理员。');
  }
  if (/empty_export/i.test(raw)) {
    return explain('当前导出范围里还没有可导出的内容。', '请先添加图片或文字，或调整导出范围后再试。');
  }
  if ((status === 400 || status === 409 || details.code === 'VALIDATION_ERROR') && /[\u4e00-\u9fff]/.test(raw) && !/[{}]|exception|java\./i.test(raw)) {
    const localMessage = raw.replace(/（状态[\s\S]*$/, '').replace(/Base URL/g, '服务地址').replace(/API Key/g, '服务密钥');
    return `${localMessage}${technicalSuffix(details)}`;
  }
  if (details.status || details.requestId || details.code || details.type) {
    return explain('这次操作没有完成，服务没有提供可以确认的原因。', '请稍后重试；反复失败时请联系管理员，并提供下方请求编号或出错时间。');
  }
  // Preserve useful local validation messages; never display an unknown English stack trace as the explanation.
  return /[\u4e00-\u9fff]/.test(raw) && raw.length <= 240 && !/[{}]|exception|java\.|code=|type=/i.test(raw)
    ? redactTechnicalMessage(raw).replace(/AI Agent/g, 'AI 助手').replace(/Base URL/g, '服务地址').replace(/API Key/g, '服务密钥').replace(/WebSocket/g, '实时连接')
    : explain('这次操作没有完成，暂时无法确认具体原因。', '请稍后重试；如果反复出现，请联系管理员并提供排查详情和出错时间。');
};


import json
import logging


class SatelliteConfigurationError(RuntimeError):
    """The managed relay cannot be called with the current configuration."""


def _has_configured_secret(value):
    normalized = str(value or '').strip().lower()
    if not normalized:
        return False
    return not (
        normalized in {'changeme', 'change-me', 'replace-me', 'your-key', 'your_api_key', 'your-api-key', 'xxx', 'sk-xxx'}
        or normalized.startswith(('your_', 'your-', 'replace-with-', 'example-', 'sk-super-'))
    )

class AIGuard:
    def __init__(self, config=None, log_callback=None):
        self.logger = logging.getLogger("AIGuard")
        self.log_callback = log_callback  # GUI日志回调
        self.update_config(config)

    def log(self, message):
        """输出日志到GUI和logger"""
        if self.log_callback:
            self.log_callback(message)
        self.logger.info(message)

    def update_config(self, config):
        if not config:
            self.enabled = False
            return
            
        # Managed satellite calls always terminate at Sub2API.  Never accept a
        # browser-supplied/user-configured key as a fallback in this mode.
        import os
        # Match the server's fail-closed mode detection.  A placeholder in an
        # example env file must never re-enable the old local-key fallback.
        self.managed = bool(
            str(os.getenv('SUB2API_APP_CREDENTIAL') or '').strip()
            or str(os.getenv('SUB2API_SSO_SECRET') or '').strip()
        )
        if self.managed:
            relay = (os.getenv('SUB2API_RELAY_BASE_URL') or os.getenv('LINK') or '').strip()
            if relay and '://' not in relay:
                relay = 'http://' + relay
            relay = relay.rstrip('/')
            if relay and not relay.endswith('/v1'):
                relay += '/v1'
            self.api_key = os.getenv('SUB2API_APP_CREDENTIAL', '').strip()
            # A managed satellite must fail closed when the relay address is
            # missing.  Falling back to loopback can accidentally call an
            # unrelated local service and violates the shared relay contract.
            self.base_url = f'{relay}/chat/completions' if relay else ''
            self.model = os.getenv('SUB2API_RELAY_MODEL', 'gpt-5.5').strip() or 'gpt-5.5'
        else:
            self.api_key = config.get('api_key', '')
            self.base_url = config.get('base_url', 'https://cc.honoursoft.cn/').rstrip('/')
            self.model = config.get('model', 'gpt-5.5')
        self.enabled = config.get('enable', False)
        self.custom_prompt = config.get('prompt', '')

    def check_relevance(self, title, content="", raise_on_error=False):
        """
        检查项目是否与无人机巡检相关
        返回: (is_relevant: bool, reason: str)
        """
        if not self.enabled:
            return True, "AI未启用"

        if self.managed:
            from auth_sso import current_subject
            if (not _has_configured_secret(self.api_key) or not self.base_url or not current_subject().strip()):
                # This is an integration/configuration failure, not a model
                # relevance failure.  Never convert it into the historical
                # "keep the bid" fallback, otherwise a managed request would
                # silently continue without the required relay identity.
                raise SatelliteConfigurationError('Sub2API satellite credential, relay URL, or session identity is not configured')
        if not self.api_key:
            return True, "AI未配置Key"

        self.log(f"🤖 [AI分析] 开始分析: {title[:40]}...")

        system_prompt = (
            "你是一个专业的招投标项目筛选专家。我们公司是做【光伏巡检无人机】和【风电巡检无人机】的，"
            "产品主要用于光伏发电板巡检（含红外热斑检测）和风力发电设施巡检（含叶片检测）。\n\n"
            "请判断该项目是否适合我们公司投标。\n\n"
            "【符合条件】：\n"
            "- 光伏电站/光伏发电项目的无人机巡检服务采购\n"
            "- 风电场/风力发电项目的无人机巡检服务采购\n"
            "- 光伏组件红外检测、热斑检测服务\n"
            "- 风机叶片无人机检测服务\n"
            "- 新能源电站无人机运维服务\n\n"
            "【排除条件】：\n"
            "- 单纯采购无人机设备（非服务）\n"
            "- 测绘、航拍、农业植保、消防等其他领域无人机\n"
            "- 光伏/风电的工程建设、设备安装（无巡检需求）\n"
            "- 清洗、清洁、运输等非巡检服务\n"
            "- 监理、咨询、设计类服务\n\n"
            "返回JSON: {\"relevant\": true/false, \"reason\": \"50字以内的判断理由\"}"
        )
        
        if self.custom_prompt:
            system_prompt = self.custom_prompt

        user_content = f"项目标题: {title}\n项目内容: {content[:800]}"

        # 判断是否使用 Claude 原生格式（基于模型名称和URL）
        is_claude_native = (
            'claude' in self.model.lower() and 
            'honoursoft' in self.base_url.lower()
        )
        
        # 构造请求payload（自动兼容 Claude 和 OpenAI/DeepSeek 格式）
        if is_claude_native:
            # Claude 原生格式：system 作为顶级参数
            payload = {
                "model": self.model,
                "system": system_prompt,
                "messages": [
                    {"role": "user", "content": user_content}
                ],
                "temperature": 0.1,
                "max_tokens": 300
            }
        else:
            # OpenAI/DeepSeek 兼容格式：system 在 messages 数组中
            payload = {
                "model": self.model,
                "messages": [
                    {"role": "system", "content": system_prompt},
                    {"role": "user", "content": user_content}
                ],
                "temperature": 0.1,
                "max_tokens": 300
            }

        # 直接使用用户提供的URL，不添加任何后缀
        url = self.base_url.rstrip('/')

        self.log(f"🔗 [AI分析] 请求API: {self.base_url}")
        self.log(f"📦 [AI分析] 使用模型: {self.model}")

        try:
            import requests
            import time
            
            headers = {"Content-Type": "application/json", "Authorization": f"Bearer {self.api_key}"}
            if self.managed:
                from auth_sso import satellite_headers
                headers.update(satellite_headers(required=True))
            
            max_retries = 3
            retry_delay = 2  # 秒
            
            for attempt in range(max_retries):
                try:
                    self.log(f"⏳ [AI分析] 正在等待AI响应...")
                    resp = requests.post(url, headers=headers, json=payload, timeout=120)
                    
                    if resp.status_code != 200:
                        error_detail = resp.text[:200]
                        self.log(f"❌ [AI分析] API返回错误: HTTP {resp.status_code}")
                        raise Exception(f"HTTP {resp.status_code}: {error_detail}")
                    
                    result = resp.json()
                    ai_content = result['choices'][0]['message']['content']
                    
                    self.log(f"✅ [AI分析] 收到AI响应")
                    
                    # 解析AI返回的JSON
                    try:
                        # 尝试从响应中提取JSON
                        if '```json' in ai_content:
                            json_str = ai_content.split('```json')[1].split('```')[0].strip()
                        elif '```' in ai_content:
                            json_str = ai_content.split('```')[1].split('```')[0].strip()
                        elif '{' in ai_content and '}' in ai_content:
                            start = ai_content.find('{')
                            end = ai_content.rfind('}') + 1
                            json_str = ai_content[start:end]
                        else:
                            json_str = ai_content
                            
                        analysis = json.loads(json_str)
                        is_relevant = analysis.get('relevant', False)
                        reason = analysis.get('reason', 'AI未提供理由')
                        
                        if is_relevant:
                            self.log(f"✅ [AI判定] 相关 - {reason}")
                        else:
                            self.log(f"🚫 [AI判定] 不相关 - {reason}")
                            
                        return is_relevant, reason
                        
                    except json.JSONDecodeError:
                        # 如果无法解析JSON，尝试从文本判断
                        self.log(f"⚠️ [AI分析] 返回非标准JSON，尝试文本分析")
                        is_relevant = "true" in ai_content.lower() or "相关" in ai_content or "是" in ai_content[:20]
                        return is_relevant, ai_content[:80]
                        
                except (requests.exceptions.ConnectionError, requests.exceptions.Timeout) as e:
                    # 网络连接错误或超时，可以重试
                    if attempt < max_retries - 1:
                        self.log(f"⚠️ [AI分析] 网络异常，{retry_delay}秒后重试 ({attempt + 1}/{max_retries})")
                        time.sleep(retry_delay)
                    else:
                        self.log(f"❌ [AI分析] 网络异常，已重试{max_retries}次仍失败")
                        if raise_on_error:
                            raise
                        return True, f"AI网络异常（已重试{max_retries}次）"

        except ImportError:
            self.log(f"❌ [AI分析] 缺少requests库")
            return True, "请安装 requests 库: pip install requests"
        except Exception as e:
            error_msg = str(e)
            self.log(f"❌ [AI分析] 请求失败: {error_msg[:100]}")
            self.logger.error(f"AI请求失败: {error_msg}")
            if raise_on_error:
                raise
            return True, f"AI请求异常: {error_msg[:50]}"

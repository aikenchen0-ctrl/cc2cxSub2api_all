# Sub2API 生图 Skill

## 用户安装

1. 解压 `genimg-skill.zip`，把其中的 `sub2api-image` 文件夹交给 Codex 安装到
   当前用户的 skills 目录（例如 `~/.codex/skills/sub2api-image`）。
   也可以把压缩包拖入 Codex，说“安装这个 skill，保留我的文字服务配置”。
2. 由用户自行在 Sub2API 的“API 密钥”页面创建一把密钥，选择已开通图片生成的分组。
   登录邮箱密码、文字专用分组密钥、上游账号密钥都不是这里需要的凭证。
3. 运行下列命令，按提示输入图片密钥，输入不会显示：

   ```text
   python ~/.codex/skills/sub2api-image/scripts/image_api.py configure
   ```

   Windows 可以使用完整路径或 `$HOME/.codex/skills/sub2api-image/scripts/image_api.py`。
   Python 版本需为 3.10 或以上，不需要 pip 安装第三方库。
   未配置时会提示“未配置生图密钥，请先配置生图密钥。”
4. 在 Codex 中说“用 Sub2API 生图，生成……”或提供参考图并要求编辑。

默认图片地址为 `https://api.cc2.cx`，可通过 `configure --base-url` 指定另一地址。
默认图片模型为 `gpt-image-2.6-flare`。
图片密钥与地址独立保存，原有文字供应商、模型、登录配置均保持原样。
一次性调用也可仅设置 `SUB2API_IMAGE_API_KEY`、`SUB2API_IMAGE_BASE_URL` 环境变量。
不要把密钥放进 Git、截图、网站说明或请求提示词。

可直接运行 `status` 检查本机是否已配置。这只是本地检查，不代表余额或上游可用。
输出默认放在任务目录的 `outputs` 下。同名文件已存在时，请选择新名称。
当前版本提供文字生图、参考图编辑、多张图片与遮罩编辑；暂未提供流式预览和
Responses 多轮图片工具。通过再次提交上一张图片，可继续进行编辑。

## 管理员上线准备

现有后台即可管理图片上游和售价，这个安装包不创建或修改服务器分组。

1. 管理员账号管理：添加支持 OpenAI Images 协议的上游账号，配置它的 API 地址
   和上游密钥。上游密钥仅保存在后台，绝不放入 skill。
2. 分组管理：新建例如“生图”的 OpenAI 分组，开启“允许图片生成”，绑定上述账号。
   只使用已经验证支持的图片模型名称。
3. 根据实际采购成本，在现有分组图片定价或渠道模型定价中配置售价并核对账单。
   `允许图片生成` 只是图片权限，不能据此认为该组自动禁止所有文字请求。
4. 创建测试用户的图片分组密钥，用本 skill 验证：
   生图成功、参考图编辑、多图张数、权限不足、余额不足、上游失败和超时。
   对照使用记录核对成功和失败场景的计费，确认后再对用户开放。
5. 发布时照常构建前端，安装包会随静态资源和嵌入式前端一起发布。
   本功能没有数据库结构迁移。

网络超时可能发生在上游已经生成之后。本工具不会自动重试付费 POST；
请先检查使用记录，确认后再提交。安装包不包含任何共享密钥或赠送余额。

## 维护安装包

源文件在仓库 `skills/sub2api-image`，下载包是 `frontend/public/downloads/genimg-skill.zip`，
两者必须同步提交。在仓库根目录执行：

```text
python skills/package-sub2api-image.py           # 重新生成下载包
python skills/package-sub2api-image.py --check   # 校验下载包与源文件一致
```

打包只包含 `SKILL.md`、`INSTALL.md`、`agents/openai.yaml`、`scripts/image_api.py`，
不包含 `__pycache__`、测试、输出图片或任何本地凭据。压缩包使用固定时间戳，
内容相同即字节相同。发布工作流在构建前端前执行 `--check`，源文件改动后忘记重新打包
会直接失败，避免下载到旧版本。

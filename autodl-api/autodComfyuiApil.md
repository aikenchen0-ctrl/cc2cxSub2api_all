Token：<7pmva6xeyRMSZh9iM3zTKENgRXDYp2xDH0nP5GMkIAW2F7WC>
https://autodl.art/large-model/comfyui随时可访问了解
H3六图三音频生视频（高质量音画融合）
工作流ID：  minimax_h3_z0903 
工作流描述：  MiniMax H3六图三音频生视频（高质量音画融合），使用 6 张参考图片和最多 3 段参考音频，生成最长 15 秒的高质量音画融合视频。
API端点： 
提交任务：
/api/v1/comfyui/comfyui_workflow/minimax_h3_z0903 
查询任务：
/api/v1/comfyui/comfyui_workflow/result/{task_id} 
价格    
480p
￥0.04 / 秒
￥0.03 / 秒
768p
￥0.07 / 秒
￥0.05 / 秒
1088p
￥0.07 / 秒
￥0.05 / 秒
1440p
￥0.08 / 秒
￥0.06 / 秒
详情API
输入
{
    "seed": "随机种子（选填，整数，用于控制生成结果的随机性，相同参数和相同Seed通常可获得相近的生成结果）",
    "prompt": "视频生成提示词（必填，1-10000个字符，建议描述主体、动作、场景、镜头运动等）",
    "duration": "视频时长（选填，整数，1-15秒，默认5秒）",
    "resolution": "输出分辨率（选填，可选值：480p竖(480*864)、480p横(864*480)、768p竖(768*1376)、768p横(1376*768)、1088p竖(1088*1920)、1088p横(1920*1088)、1440p竖(1440*2560)、1440p横(2560*1440)，默认768p竖(768*1376)）",
    "ref_audio_0": "参考音频URL（必填，支持MP3/WAV/MP4/FLAC）",
    "ref_audio_1": "参考音频URL（选填，支持MP3/WAV/MP4/FLAC）",
    "ref_audio_2": "参考音频URL（选填，支持MP3/WAV/MP4/FLAC）",
    "ref_image_0": "参考图片URL（必填，支持JPG/PNG/WebP）",
    "ref_image_1": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_2": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_3": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_4": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_5": "参考图片URL（选填，支持JPG/PNG/WebP）"
}
输出
{
    "msg": "",
    "code": "Success",
    "data": {
        "status": "completed",
        "results": [
            {
                "url": "https://",
                "type": "video",
                "file_type": "mp4",
                "output_type": "output"
            }
        ],
        "task_id": "671ce5ca-80b3-4a18-8b5c-af6013f0f03d",
        "client_id": "158a0bba46b6a42f9b175ecdeaea2ac4"
    },
    "request_id": "0782accb6b05116890256ffaa01cb283"
}H3六图生视频（多图一致性创作）
工作流ID：  minimax_h3_z0902 
工作流描述：  MiniMax H3 六图生视频（多图一致性创作），使用 6 张参考图片和文字提示生成最长 15 秒的高质量 AI 视频。
API端点： 
提交任务：
/api/v1/comfyui/comfyui_workflow/minimax_h3_z0902 
查询任务：
/api/v1/comfyui/comfyui_workflow/result/{task_id} 
价格    
480p
￥0.04 / 秒
￥0.03 / 秒
768p
￥0.07 / 秒
￥0.05 / 秒
1088p
￥0.07 / 秒
￥0.05 / 秒
1440p
￥0.08 / 秒
￥0.06 / 秒
详情API
输入
{
    "seed": "随机种子（选填，整数，用于控制生成结果的随机性，相同参数和相同Seed通常可获得相近的生成结果）",
    "prompt": "视频生成提示词（必填，1-10000个字符，建议描述主体、动作、场景、镜头运动等）",
    "duration": "视频时长（选填，整数，1-15秒，默认5秒）",
    "resolution": "输出分辨率（选填，可选值：480p竖(480*864)、480p横(864*480)、768p竖(768*1376)、768p横(1376*768)、1088p竖(1088*1920)、1088p横(1920*1088)、1440p竖(1440*2560)、1440p横(2560*1440)，默认768p竖(768*1376)）",
    "ref_image_0": "参考图片URL（必填，支持JPG/PNG/WebP）",
    "ref_image_1": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_2": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_3": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_4": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_5": "参考图片URL（选填，支持JPG/PNG/WebP）"
}
输出
{
    "msg": "",
    "code": "Success",
    "data": {
        "status": "completed",
        "results": [
            {
                "url": "https://",
                "type": "video",
                "file_type": "mp4",
                "output_type": "output"
            }
        ],
        "task_id": "671ce5ca-80b3-4a18-8b5c-af6013f0f03d",
        "client_id": "158a0bba46b6a42f9b175ecdeaea2ac4"
    },
    "request_id": "0782accb6b05116890256ffaa01cb283"
}
H3文生视频（高质量创意直出）
工作流ID：  minimax_h3_z0901 
工作流描述：  MiniMax H3 文生视频（高质量创意直出），通过文字描述直接生成最长 15 秒的高质量 AI 视频，无需上传参考图片或音频。
API端点： 
提交任务：
/api/v1/comfyui/comfyui_workflow/minimax_h3_z0901 
查询任务：
/api/v1/comfyui/comfyui_workflow/result/{task_id} 
价格    
480p
￥0.04 / 秒
￥0.03 / 秒
768p
￥0.07 / 秒
￥0.05 / 秒
1088p
￥0.07 / 秒
￥0.05 / 秒
1440p
￥0.08 / 秒
￥0.06 / 秒
详情API
输入
{
    "seed": "随机种子（选填，整数，用于控制生成结果的随机性，相同参数和相同Seed通常可获得相近的生成结果）",
    "prompt": "视频生成提示词（必填，1-10000个字符，建议描述主体、动作、场景、镜头运动等）",
    "duration": "视频时长（选填，整数，1-15秒，默认5秒）",
    "resolution": "输出分辨率（选填，可选值：480p竖(480*864)、480p横(864*480)、768p竖(768*1376)、768p横(1376*768)、1088p竖(1088*1920)、1088p横(1920*1088)、1440p竖(1440*2560)、1440p横(2560*1440)，默认768p竖(768*1376)）"
}
输出
{
    "msg": "",
    "code": "Success",
    "data": {
        "status": "completed",
        "results": [
            {
                "url": "https://",
                "type": "video",
                "file_type": "mp4",
                "output_type": "output"
            }
        ],
        "task_id": "671ce5ca-80b3-4a18-8b5c-af6013f0f03d",
        "client_id": "158a0bba46b6a42f9b175ecdeaea2ac4"
    },
    "request_id": "0782accb6b05116890256ffaa01cb283"
}

H3多图多音频生视频(升级画质)
工作流ID：  minimax_h3_zm_u24 
工作流描述：  MiniMax H3多图参考生成视频，最长支持15秒视频
API端点： 
提交任务：
/api/v1/comfyui/comfyui_workflow/minimax_h3_zm_u24 
查询任务：
/api/v1/comfyui/comfyui_workflow/result/{task_id} 
价格    
480p
￥0.03 / 秒
￥0.02 / 秒
768p
￥0.04 / 秒
￥0.03 / 秒
详情API
输入
{
    "seed": "随机种子（选填，整数，用于控制生成结果的随机性，相同参数和相同Seed通常可获得相近的生成结果）",
    "prompt": "视频生成提示词（必填，1-10000个字符，建议描述主体、动作、场景、镜头运动等）",
    "duration": "视频时长（选填，整数，1-15秒，默认5秒）",
    "resolution": "输出分辨率（选填，可选值：480p竖、768p竖、480p横、768p横、480p(1:1)、768p(1:1)，默认768p竖）",
    "ref_audio_0": "参考音频URL（选填，支持MP3/WAV/MP4/FLAC）",
    "ref_audio_1": "参考音频URL（选填，支持MP3/WAV/MP4/FLAC）",
    "ref_audio_2": "参考音频URL（选填，支持MP3/WAV/MP4/FLAC）",
    "ref_image_0": "参考图片URL（必填，支持JPG/PNG/WebP）",
    "ref_image_1": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_2": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_3": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_4": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_5": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_6": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_7": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_8": "参考图片URL（选填，支持JPG/PNG/WebP）"
}
输出
{
    "msg": "",
    "code": "Success",
    "data": {
        "status": "completed",
        "results": [
            {
                "url": "https://",
                "type": "video",
                "file_type": "mp4",
                "output_type": "output"
            }
        ],
        "task_id": "671ce5ca-80b3-4a18-8b5c-af6013f0f03d",
        "client_id": "158a0bba46b6a42f9b175ecdeaea2ac4"
    },
    "request_id": "0782accb6b05116890256ffaa01cb283"
}
H3多图多音频生视频(高速版)
工作流ID：  minimax_h3_zm_u08 
工作流描述：  MiniMax H3多图参考生成视频，最长支持15秒视频
API端点： 
提交任务：
/api/v1/comfyui/comfyui_workflow/minimax_h3_zm_u08 
查询任务：
/api/v1/comfyui/comfyui_workflow/result/{task_id} 
价格    
480p
￥0.03 / 秒
￥0.02 / 秒
768p
￥0.04 / 秒
￥0.03 / 秒
详情API
输入
{
    "seed": "随机种子（选填，整数，用于控制生成结果的随机性，相同参数和相同Seed通常可获得相近的生成结果）",
    "prompt": "视频生成提示词（必填，1-10000个字符，建议描述主体、动作、场景、镜头运动等）",
    "duration": "视频时长（选填，整数，1-15秒，默认5秒）",
    "resolution": "输出分辨率（选填，可选值：480p竖、768p竖、480p横、768p横、480p(1:1)、768p(1:1)，默认768p竖）",
    "ref_audio_0": "参考音频URL（选填，支持MP3/WAV/MP4/FLAC）",
    "ref_audio_1": "参考音频URL（选填，支持MP3/WAV/MP4/FLAC）",
    "ref_audio_2": "参考音频URL（选填，支持MP3/WAV/MP4/FLAC）",
    "ref_image_0": "参考图片URL（必填，支持JPG/PNG/WebP）",
    "ref_image_1": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_2": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_3": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_4": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_5": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_6": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_7": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_8": "参考图片URL（选填，支持JPG/PNG/WebP）"
}
输出
{
    "msg": "",
    "code": "Success",
    "data": {
        "status": "completed",
        "results": [
            {
                "url": "https://",
                "type": "video",
                "file_type": "mp4",
                "output_type": "output"
            }
        ],
        "task_id": "671ce5ca-80b3-4a18-8b5c-af6013f0f03d",
        "client_id": "158a0bba46b6a42f9b175ecdeaea2ac4"
    },
    "request_id": "0782accb6b05116890256ffaa01cb283"
}
H3首尾帧生成视频
工作流ID：  minimax_h3_b99_002 
工作流描述：  MiniMax H3首尾帧生视频
API端点： 
提交任务：
/api/v1/comfyui/comfyui_workflow/minimax_h3_b99_002 
查询任务：
/api/v1/comfyui/comfyui_workflow/result/{task_id} 
价格    
736p
￥0.05 / 秒
￥0.03 / 秒
详情API
输入
{
    "seed": "随机种子（选填，整数，用于控制生成结果的随机性，相同参数和相同Seed通常可获得相近的生成结果）",
    "prompt": "视频生成提示词（必填，建议描述主体、动作、场景、镜头等）",
    "duration": "视频时长（选填，整数，1-15 秒，默认5秒）",
    "last_frame": "尾帧图片 URL（必填，支持 JPG/PNG/WebP）",
    "resolution": "输出分辨率（选填，可选值：736p竖、736p横、736p(1:1)，默认值736p竖）",
    "first_frame": "首帧图片 URL（必填，支持 JPG/PNG/WebP）"
}
输出
{
    "msg": "",
    "code": "Success",
    "data": {
        "status": "completed",
        "results": [
            {
                "url": "https://",
                "type": "video",
                "file_type": "mp4",
                "output_type": "output"
            }
        ],
        "task_id": "671ce5ca-80b3-4a18-8b5c-af6013f0f03d",
        "client_id": "158a0bba46b6a42f9b175ecdeaea2ac4"
    },
    "request_id": "0782accb6b05116890256ffaa01cb283"
}

H3文生视频
工作流ID：  minimax_h3_b99_001 
工作流描述：  MiniMax H3文生视频
API端点： 
提交任务：
/api/v1/comfyui/comfyui_workflow/minimax_h3_b99_001 
查询任务：
/api/v1/comfyui/comfyui_workflow/result/{task_id} 
价格    
736p
￥0.05 / 秒
￥0.03 / 秒
详情API
输入
{
    "seed": "随机种子（选填，整数，用于控制生成结果的随机性，相同参数和相同Seed通常可获得相近的生成结果）",
    "prompt": "视频生成提示词（必填，建议描述主体、动作、场景、镜头等）",
    "duration": "视频时长（选填，整数，1-15 秒，默认5秒）",
    "resolution": "输出分辨率（选填，可选值：736p竖、736p横、736p(1:1)，默认值736p竖）"
}
输出
{
    "msg": "",
    "code": "Success",
    "data": {
        "status": "completed",
        "results": [
            {
                "url": "https://",
                "type": "video",
                "file_type": "mp4",
                "output_type": "output"
            }
        ],
        "task_id": "671ce5ca-80b3-4a18-8b5c-af6013f0f03d",
        "client_id": "158a0bba46b6a42f9b175ecdeaea2ac4"
    },
    "request_id": "0782accb6b05116890256ffaa01cb283"
}

H3多图生视频12秒
工作流ID：  minimax_h3_b99_003_12s 
工作流描述：  MiniMax H3多图参考生成视频，最长支持12秒视频
API端点： 
提交任务：
/api/v1/comfyui/comfyui_workflow/minimax_h3_b99_003_12s 
查询任务：
/api/v1/comfyui/comfyui_workflow/result/{task_id} 
价格    
736p
￥0.05 / 秒
￥0.03 / 秒
详情API
输入
{
    "seed": "随机种子（选填，整数，用于控制生成结果的随机性，相同参数和相同Seed通常可获得相近的生成结果）",
    "prompt": "视频生成提示词（必填，1-10000个字符，建议描述主体、动作、场景、镜头运动等）",
    "duration": "视频时长（选填，整数，1-12秒，默认5秒）",
    "resolution": "输出分辨率（选填，可选值：736p竖、736p横、736p(1:1)，默认736p竖）",
    "ref_image_0": "参考图片URL（必填，支持JPG/PNG/WebP）",
    "ref_image_1": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_2": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_3": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_4": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_5": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_6": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_7": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_8": "参考图片URL（选填，支持JPG/PNG/WebP）"
}
输出
{
    "msg": "",
    "code": "Success",
    "data": {
        "status": "completed",
        "results": [
            {
                "url": "https://",
                "type": "video",
                "file_type": "mp4",
                "output_type": "output"
            }
        ],
        "task_id": "671ce5ca-80b3-4a18-8b5c-af6013f0f03d",
        "client_id": "158a0bba46b6a42f9b175ecdeaea2ac4"
    },
    "request_id": "0782accb6b05116890256ffaa01cb283"
}

动作迁移
工作流ID：  wan2.2animate-v4-motion_retargeting 
工作流描述：  动作迁移
API端点： 
提交任务：
/api/v1/comfyui/comfyui_workflow/wan2.2animate-v4-motion_retargeting 
查询任务：
/api/v1/comfyui/comfyui_workflow/result/{task_id} 
价格    
根据生成视频的实际时长进行计费
 高峰时段价格(08:00~24:00)：￥0.04 / 秒
 空闲时段价格(00:00~08:00)：￥0.03 / 秒
详情API
输入
{
    "seed": "随机种子(选填，整数，用于控制生成结果的随机性，相同参数和相同Seed通常可获得相近的生成结果)",
    "ref_image": "参考图片URL(必填，支持 JPG/PNG/WebP)",
    "ref_video": "参考视频URL(必填，支持 MP4/WebM)",
    "resolution": "输出分辨率(选填，可选值：464*832px(竖版)、832*464px(横版)，默认464*832px(竖版))"
}
输出
{
    "msg": "",
    "code": "Success",
    "data": {
        "status": "completed",
        "results": [
            {
                "url": "https://",
                "type": "video",
                "file_type": "mp4",
                "output_type": "output"
            }
        ],
        "task_id": "671ce5ca-80b3-4a18-8b5c-af6013f0f03d",
        "client_id": "158a0bba46b6a42f9b175ecdeaea2ac4"
    },
    "request_id": "0782accb6b05116890256ffaa01cb283"
}

H3多图多音频生视频15秒
工作流ID：  minimax_h3_image_audio_to_video_v2_15s 
工作流描述：  MiniMax H3多图多音频参考生视频，最长支持15秒视频，该工作流未做任何包装门槛较高，需精确控制提示词来使用
API端点： 
提交任务：
/api/v1/comfyui/comfyui_workflow/minimax_h3_image_audio_to_video_v2_15s 
查询任务：
/api/v1/comfyui/comfyui_workflow/result/{task_id} 
价格    
480p
￥0.03 / 秒
￥0.02 / 秒
768p
￥0.04 / 秒
￥0.03 / 秒
详情API
输入
{
    "seed": "随机种子（选填，整数，用于控制生成结果的随机性，相同参数和相同Seed通常可获得相近的生成结果）",
    "prompt": "视频生成提示词（必填，1-10000个字符，建议描述主体、动作、场景、镜头运动等）",
    "duration": "视频时长（选填，整数，1-15秒，默认5秒）",
    "resolution": "输出分辨率（选填，可选值：480p竖、768p竖、480p横、768p横，默认768p竖）",
    "ref_audio_0": "参考音频URL（选填，支持MP3/WAV/MP4/FLAC）",
    "ref_audio_1": "参考音频URL（选填，支持MP3/WAV/MP4/FLAC）",
    "ref_audio_2": "参考音频URL（选填，支持MP3/WAV/MP4/FLAC）",
    "ref_image_0": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_1": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_2": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_3": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_4": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_5": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_6": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_7": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_8": "参考图片URL（选填，支持JPG/PNG/WebP）"
}
输出
{
    "msg": "",
    "code": "Success",
    "data": {
        "status": "completed",
        "results": [
            {
                "url": "https://",
                "type": "video",
                "file_type": "mp4",
                "output_type": "output"
            }
        ],
        "task_id": "671ce5ca-80b3-4a18-8b5c-af6013f0f03d",
        "client_id": "158a0bba46b6a42f9b175ecdeaea2ac4"
    },
    "request_id": "0782accb6b05116890256ffaa01cb283"
}

H3多图生视频15秒
工作流ID：  minimax_h3_lightx2v_v5_15s 
工作流描述：  MiniMax H3多图参考生成视频，最长支持15秒视频
API端点： 
提交任务：
/api/v1/comfyui/comfyui_workflow/minimax_h3_lightx2v_v5_15s 
查询任务：
/api/v1/comfyui/comfyui_workflow/result/{task_id} 
价格    
480p
￥0.03 / 秒
￥0.02 / 秒
768p
￥0.04 / 秒
￥0.03 / 秒
详情API
输入
{
    "seed": "随机种子（选填，整数，用于控制生成结果的随机性，相同参数和相同Seed通常可获得相近的生成结果）",
    "prompt": "视频生成提示词（必填，建议描述主体、动作、场景、镜头等）",
    "duration": "视频时长（选填，整数，1-15 秒，默认5秒）",
    "resolution": "输出分辨率（选填，可选值：480p竖、480p横、768p竖、768p横、480p(1:1)、768p(1:1)，默认值768p竖）",
    "ref_image_0": "图片 URL（必填，支持 JPG/PNG/WebP）",
    "ref_image_1": "图片 URL（选填，支持 JPG/PNG/WebP）",
    "ref_image_2": "图片 URL（选填，支持 JPG/PNG/WebP）",
    "ref_image_3": "图片 URL（选填，支持 JPG/PNG/WebP）",
    "ref_image_4": "图片 URL（选填，支持 JPG/PNG/WebP）",
    "ref_image_5": "图片 URL（选填，支持 JPG/PNG/WebP）",
    "ref_image_6": "图片 URL（选填，支持 JPG/PNG/WebP）",
    "ref_image_7": "图片 URL（选填，支持 JPG/PNG/WebP）",
    "ref_image_8": "图片 URL（选填，支持 JPG/PNG/WebP）"
}
输出
{
    "msg": "",
    "code": "Success",
    "data": {
        "status": "completed",
        "results": [
            {
                "url": "https://",
                "type": "video",
                "file_type": "mp4",
                "output_type": "output"
            }
        ],
        "task_id": "671ce5ca-80b3-4a18-8b5c-af6013f0f03d",
        "client_id": "158a0bba46b6a42f9b175ecdeaea2ac4"
    },
    "request_id": "0782accb6b05116890256ffaa01cb283"
}

H3多图多音频生视频
工作流ID：  minimax_h3_image_audio_to_video_v2 
工作流描述：  MiniMax H3多图多音频参考生视频，该工作流未做任何包装门槛较高，需精确控制提示词来使用
API端点： 
提交任务：
/api/v1/comfyui/comfyui_workflow/minimax_h3_image_audio_to_video_v2 
查询任务：
/api/v1/comfyui/comfyui_workflow/result/{task_id} 
价格    
480p
￥0.03 / 秒
￥0.02 / 秒
768p
￥0.04 / 秒
￥0.03 / 秒
1080p
￥0.10 / 秒
￥0.06 / 秒
详情API
输入
{
    "seed": "随机种子（选填，整数，用于控制生成结果的随机性，相同参数和相同Seed通常可获得相近的生成结果）",
    "prompt": "视频生成提示词（必填，1-10000个字符，建议描述主体、动作、场景、镜头运动等）",
    "duration": "视频时长（选填，整数，1-10秒，默认5秒）",
    "resolution": "输出分辨率（选填，可选值：480p竖、768p竖、1080p竖、480p横、768p横、1080p横，默认768p竖）",
    "ref_audio_0": "参考音频URL（选填，支持MP3/WAV/MP4/FLAC）",
    "ref_audio_1": "参考音频URL（选填，支持MP3/WAV/MP4/FLAC）",
    "ref_audio_2": "参考音频URL（选填，支持MP3/WAV/MP4/FLAC）",
    "ref_image_0": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_1": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_2": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_3": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_4": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_5": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_6": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_7": "参考图片URL（选填，支持JPG/PNG/WebP）",
    "ref_image_8": "参考图片URL（选填，支持JPG/PNG/WebP）"
}
输出
{
    "msg": "",
    "code": "Success",
    "data": {
        "status": "completed",
        "results": [
            {
                "url": "https://",
                "type": "video",
                "file_type": "mp4",
                "output_type": "output"
            }
        ],
        "task_id": "671ce5ca-80b3-4a18-8b5c-af6013f0f03d",
        "client_id": "158a0bba46b6a42f9b175ecdeaea2ac4"
    },
    "request_id": "0782accb6b05116890256ffaa01cb283"
}

H3图生视频-音频同步(自动对口型)
工作流ID：  minimax_h3_image_audio_to_video 
工作流描述：  MiniMax H3图生视频-音频同步
API端点： 
提交任务：
/api/v1/comfyui/comfyui_workflow/minimax_h3_image_audio_to_video 
查询任务：
/api/v1/comfyui/comfyui_workflow/result/{task_id} 
价格    
480p
￥0.03 / 秒
￥0.02 / 秒
768p
￥0.04 / 秒
￥0.03 / 秒
1080p
￥0.09 / 秒
￥0.05 / 秒
详情API
输入
{
    "resolution": "输出视频分辨率（选填，可选值：480p竖、768p竖、1080p竖、480p横、768p横、1080p横，默认值：768p竖）",
    "ref_audio_0": "参考音频 URL（必填，支持 MP3、WAV、MP4、FLAC）",
    "ref_image_0": "参考图片 URL（必填，支持 JPG、PNG、WebP）",
    "audio_duration": "音频截取时长，单位为秒（选填，1-15秒，默认值：5秒）"
}
输出
{
    "msg": "",
    "code": "Success",
    "data": {
        "status": "completed",
        "results": [
            {
                "url": "https://",
                "type": "video",
                "file_type": "mp4",
                "output_type": "output"
            }
        ],
        "task_id": "671ce5ca-80b3-4a18-8b5c-af6013f0f03d",
        "client_id": "158a0bba46b6a42f9b175ecdeaea2ac4"
    },
    "request_id": "0782accb6b05116890256ffaa01cb283"
}

H3多图参考生视频
工作流ID：  minimax_h3_lightx2v_v5 
工作流描述：  MiniMax H3多图参考生成视频
API端点： 
提交任务：
/api/v1/comfyui/comfyui_workflow/minimax_h3_lightx2v_v5 
查询任务：
/api/v1/comfyui/comfyui_workflow/result/{task_id} 
价格    
480p
￥0.03 / 秒
￥0.02 / 秒
768p
￥0.04 / 秒
￥0.03 / 秒
1080p
￥0.09 / 秒
￥0.05 / 秒
详情API
输入
{
    "seed": "随机种子（选填，整数，用于控制生成结果的随机性，相同参数和相同Seed通常可获得相近的生成结果）",
    "prompt": "视频生成提示词（必填，建议描述主体、动作、场景、镜头等）",
    "duration": "视频时长（选填，整数，1-10 秒，默认5秒）",
    "resolution": "输出分辨率（选填，可选值：480p竖、480p横、1080p横、768p竖、768p横、1080p竖、480p(1:1)、768p(1:1)、1080p(1:1)，默认值768p竖）",
    "ref_image_0": "图片 URL（必填，支持 JPG/PNG/WebP）",
    "ref_image_1": "图片 URL（选填，支持 JPG/PNG/WebP）",
    "ref_image_2": "图片 URL（选填，支持 JPG/PNG/WebP）",
    "ref_image_3": "图片 URL（选填，支持 JPG/PNG/WebP）",
    "ref_image_4": "图片 URL（选填，支持 JPG/PNG/WebP）",
    "ref_image_5": "图片 URL（选填，支持 JPG/PNG/WebP）",
    "ref_image_6": "图片 URL（选填，支持 JPG/PNG/WebP）",
    "ref_image_7": "图片 URL（选填，支持 JPG/PNG/WebP）",
    "ref_image_8": "图片 URL（选填，支持 JPG/PNG/WebP）"
}
输出
{
    "msg": "",
    "code": "Success",
    "data": {
        "status": "completed",
        "results": [
            {
                "url": "https://",
                "type": "video",
                "file_type": "mp4",
                "output_type": "output"
            }
        ],
        "task_id": "671ce5ca-80b3-4a18-8b5c-af6013f0f03d",
        "client_id": "158a0bba46b6a42f9b175ecdeaea2ac4"
    },
    "request_id": "0782accb6b05116890256ffaa01cb283"
}

H3文生视频
工作流ID：  minimax_h3_lightx2v_no_pic 
工作流描述：  MiniMax H3文生视频
API端点： 
提交任务：
/api/v1/comfyui/comfyui_workflow/minimax_h3_lightx2v_no_pic 
查询任务：
/api/v1/comfyui/comfyui_workflow/result/{task_id} 
价格    
480p
￥0.03 / 秒
￥0.02 / 秒
768p
￥0.04 / 秒
￥0.03 / 秒
详情API
输入
{
    "prompt": "视频生成提示词（必填，建议描述主体、动作、场景、镜头等）",
    "duration": "视频时长（选填，整数，1-15 秒，默认5秒）",
    "resolution": "输出分辨率（选填，可选值：480p竖、480p横、768p竖、768p横、480p(1:1)、768p(1:1)，默认值768竖）"
}
输出
{
    "msg": "",
    "code": "Success",
    "data": {
        "status": "completed",
        "results": [
            {
                "url": "https://",
                "type": "video",
                "file_type": "mp4",
                "output_type": "output"
            }
        ],
        "task_id": "671ce5ca-80b3-4a18-8b5c-af6013f0f03d",
        "client_id": "158a0bba46b6a42f9b175ecdeaea2ac4"
    },
    "request_id": "0782accb6b05116890256ffaa01cb283"
}

H3首尾帧生成视频
工作流ID：  minimax_h3_lightx2v 
工作流描述：  MiniMax H3首尾帧生视频
API端点： 
提交任务：
/api/v1/comfyui/comfyui_workflow/minimax_h3_lightx2v 
查询任务：
/api/v1/comfyui/comfyui_workflow/result/{task_id} 
价格    
480p
￥0.03 / 秒
￥0.02 / 秒
768p
￥0.04 / 秒
￥0.03 / 秒
详情API
输入
{
    "seed": "随机种子（选填，整数，用于控制生成结果的随机性，相同参数和相同Seed通常可获得相近的生成结果）",
    "prompt": "视频生成提示词（必填，建议描述主体、动作、场景、镜头等）",
    "duration": "视频时长（选填，整数，1-10 秒，默认5秒）",
    "last_frame": "尾帧图片 URL（必填，支持 JPG/PNG/WebP）",
    "resolution": "输出分辨率（选填，可选值：480p竖、480p横、768p竖、768p横、480p(1:1)、768p(1:1)，默认值768竖）",
    "first_frame": "首帧图片 URL（必填，支持 JPG/PNG/WebP）"
}
输出
{
    "msg": "",
    "code": "Success",
    "data": {
        "status": "completed",
        "results": [
            {
                "url": "https://",
                "type": "video",
                "file_type": "mp4",
                "output_type": "output"
            }
        ],
        "task_id": "671ce5ca-80b3-4a18-8b5c-af6013f0f03d",
        "client_id": "158a0bba46b6a42f9b175ecdeaea2ac4"
    },
    "request_id": "0782accb6b05116890256ffaa01cb283"
}

indextts2
工作流ID：  indextts2-v1 
工作流描述：  indextts2
API端点： 
提交任务：
/api/v1/comfyui/comfyui_workflow/indextts2-v1 
查询任务：
/api/v1/comfyui/comfyui_workflow/result/{task_id} 
价格    
输入：￥0.02 / 次
详情API
输入
{
    "emo_sad": 0,
    "emo_calm": 0.3,
    "emo_angry": 0,
    "emo_happy": 0.5,
    "emo_afraid": 0,
    "emo_random": false,
    "prompt_text": "你好，这是一段测试文本",
    "emo_disgusted": 0,
    "emo_ref_audio": "https://www.soundhelix.com/examples/mp3/SoundHelix-Song-1.mp3",
    "emo_surprised": 0,
    "prompt_simple": "https://www.soundhelix.com/examples/mp3/SoundHelix-Song-2.mp3",
    "emo_melancholic": 0,
    "emo_control_method": "与音色参考音频相同"
}
输出
{
    "msg": "",
    "code": "Success",
    "data": {
        "status": "completed",
        "results": [
            {
                "url": "https://codewithgpu-test-1310972338.cos.ap-beijing.myqcloud.com/comfyui/outputs/3/2026/04/16/671ce5ca-80b3-4a18-8b5c-af6013f0f03d/ComfyUI_00010_.wav",
                "type": "audio",
                "node_id": "9",
                "file_type": "wav",
                "output_type": "output"
            }
        ],
        "task_id": "671ce5ca-80b3-4a18-8b5c-af6013f0f03d",
        "client_id": "158a0bba46b6a42f9b175ecdeaea2ac4"
    },
    "request_id": "0782accb6b05116890256ffaa01cb283"
}

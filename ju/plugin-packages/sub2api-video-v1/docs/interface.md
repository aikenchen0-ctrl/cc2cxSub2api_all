# Sub2API 视频接口

- Base URL：Sub2API 网关 `/v1`，不是管理接口 `/api/v1`。
- 创建：`POST /v1/videos`，JSON 字段 `model/prompt/duration/resolution/aspect_ratio/images`。
- 状态：`GET /v1/videos/{taskId}`，接受标准对象或 AutoDL `code/data/msg` 信封。
- 下载：`GET /v1/videos/{taskId}/content`，三个阶段均使用后端 Bearer Key。
- 不消费状态响应中的外部 URL。视频完成后通过已鉴权的 content 端点下载，再由影策持久化素材并提供播放与 Range 访问。
- 支持 `minimax_h3_b99_001`（文生视频）、`minimax_h3_b99_002`（首尾两帧）、`minimax_h3_b99_003_12s`（多图）。`minimax/minimax-h3/minimax_h3` 为文生视频别名。
- 尚未核实的工作流、音频模型、视频或音频参考输入明确拒绝，不按模型名字猜测协议。
- 图像按显式 `first_frame/last_frame` 排序，其余参考图按 `order` 排序；AutoDL 的具体字段转换由 Sub2API 负责。
- 参考素材必须能从上游访问；宿主负责资源归属检查、签名 URL 和 SSRF 限制。部署只有内网地址时可验证文生视频，图生视频仍需配置可公开访问的素材服务。
- 未声明取消端点：Sub2API 当前不支持取消上游视频，取消影策等待不保证撤销已发生的上游费用。

<!-- YINGCE_MANIFEST_CONTRACT_START -->
## Manifest 完整接口定义

以下 JSON 与插件包内实际 `manifest.json` 逐字段一致，覆盖插件身份、权限、配置、鉴权、参数、校验、创建、Agent、查询、取消、结果下载、响应和 Agent 响应映射。`documentation` 字段的值就是当前完整文档；为避免文档在自身内部无限递归，JSON 中仅用等义占位文本表示正文。

```json
{
  "apiVersion": "yingce.plugin/v2",
  "id": "sub2api-video-v1",
  "name": "Sub2API Video",
  "version": "1.0.0",
  "author": "cc2cx",
  "description": "Sub2API JSON video gateway with authenticated result downloads.",
  "documentation": "<当前插件的完整 documentation，由 README.md 与 docs/interface.md 拼接而成；为避免 JSON 递归，此处不重复展开正文。>",
  "permissions": [
    "generation.run",
    "media.read"
  ],
  "configuration": {
    "fields": [
      {
        "name": "apiKey",
        "type": "secret",
        "label": "Super Key",
        "required": true
      }
    ]
  },
  "contributes": {
    "providers": [
      {
        "id": "sub2api-video-v1",
        "label": "Sub2API Video",
        "capabilities": [
          "video"
        ],
        "scopes": [
          "admin.system-channel",
          "canvas",
          "creation"
        ],
        "requiresPublicMediaUrls": true,
        "auth": {
          "type": "bearer",
          "field": "apiKey"
        },
        "parameters": [
          {
            "name": "model",
            "type": "string",
            "required": true,
            "mapping": "model",
            "description": "Verified Sub2API video model or alias."
          },
          {
            "name": "prompt",
            "type": "string",
            "required": true,
            "mapping": "prompt",
            "description": "Video generation prompt."
          },
          {
            "name": "duration",
            "type": "integer",
            "mapping": "duration",
            "description": "Integer duration in seconds, constrained by the model capability."
          },
          {
            "name": "resolution",
            "type": "string",
            "mapping": "resolution",
            "description": "Model resolution tier; Sub2API maps it to the workflow enum."
          },
          {
            "name": "aspectRatio",
            "type": "string",
            "mapping": "aspect_ratio",
            "description": "16:9 or 9:16; square output is not enabled."
          },
          {
            "name": "images",
            "type": "media[]",
            "mapping": "images",
            "description": "Public reference URLs, first and last frame roles ordered before other references."
          }
        ],
        "validations": [
          {
            "assert": {
              "$in": [
                {
                  "$ref": "request.model"
                },
                [
                  "minimax_h3_b99_001",
                  "minimax_h3_b99_002",
                  "minimax_h3_b99_003_12s",
                  "minimax",
                  "minimax-h3",
                  "minimax_h3"
                ]
              ]
            },
            "message": "Sub2API video model has no verified capability profile"
          },
          {
            "assert": {
              "$and": [
                {
                  "$eq": [
                    {
                      "$len": {
                        "$ref": "request.videos"
                      }
                    },
                    0
                  ]
                },
                {
                  "$eq": [
                    {
                      "$len": {
                        "$ref": "request.audios"
                      }
                    },
                    0
                  ]
                }
              ]
            },
            "message": "This Sub2API video profile does not accept video or audio references"
          }
        ],
        "create": {
          "method": "POST",
          "path": "/v1/videos",
          "contentType": "application/json",
          "body": {
            "model": {
              "$ref": "request.model"
            },
            "prompt": {
              "$ref": "request.prompt"
            },
            "duration": {
              "$ref": "request.duration"
            },
            "resolution": {
              "$ref": "request.resolution"
            },
            "aspect_ratio": {
              "$ref": "request.aspectRatio"
            },
            "images": {
              "$omitEmpty": {
                "$map": {
                  "from": {
                    "$concatArrays": [
                      {
                        "$filter": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.images"
                            }
                          },
                          "as": "media",
                          "where": {
                            "$eq": [
                              {
                                "$ref": "media.role"
                              },
                              "first_frame"
                            ]
                          }
                        }
                      },
                      {
                        "$filter": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.images"
                            }
                          },
                          "as": "media",
                          "where": {
                            "$eq": [
                              {
                                "$ref": "media.role"
                              },
                              "last_frame"
                            ]
                          }
                        }
                      },
                      {
                        "$filter": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.images"
                            }
                          },
                          "as": "media",
                          "where": {
                            "$and": [
                              {
                                "$ne": [
                                  {
                                    "$ref": "media.role"
                                  },
                                  "first_frame"
                                ]
                              },
                              {
                                "$ne": [
                                  {
                                    "$ref": "media.role"
                                  },
                                  "last_frame"
                                ]
                              }
                            ]
                          }
                        }
                      }
                    ]
                  },
                  "as": "media",
                  "in": {
                    "$ref": "media.value"
                  }
                }
              }
            }
          }
        },
        "poll": {
          "method": "GET",
          "path": "/v1/videos/{{taskId}}"
        },
        "result": {
          "method": "GET",
          "path": "/v1/videos/{{taskId}}/content",
          "headers": {
            "Accept": "video/mp4, video/webm"
          }
        },
        "response": {
          "taskIdPaths": [
            "data.task_id",
            "data.id",
            "task_id",
            "id"
          ],
          "status": {
            "$if": {
              "condition": {
                "$in": [
                  {
                    "$coalesce": [
                      {
                        "$ref": "response.code"
                      },
                      "Success"
                    ]
                  },
                  [
                    0,
                    "0",
                    200,
                    "200",
                    201,
                    "201",
                    "Success",
                    "success"
                  ]
                ]
              },
              "then": {
                "$coalesce": [
                  {
                    "$ref": "response.data.status"
                  },
                  {
                    "$ref": "response.status"
                  },
                  {
                    "$ref": "response.data.state"
                  },
                  {
                    "$ref": "response.state"
                  },
                  "pending"
                ]
              },
              "else": "failed"
            }
          },
          "errorPaths": [
            "error"
          ],
          "messagePaths": [
            "data.message",
            "error.message",
            "msg",
            "message",
            "fail_reason"
          ]
        }
      }
    ]
  }
}
```
<!-- YINGCE_MANIFEST_CONTRACT_END -->

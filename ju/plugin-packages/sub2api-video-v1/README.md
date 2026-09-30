# Sub2API Video

影策后台使用卫星应用凭证和当前 SSO 用户身份调用 Sub2API 视频网关；Sub2API 在服务端使用该用户的 SuperKey 跨分组调度。浏览器不直接调用上游，不接收任何 Key 或临时视频地址。

源码在本目录，运行时读取同级 `sub2api-video-v1.yingce-plugin`。接口合同见 [docs/interface.md](docs/interface.md)。

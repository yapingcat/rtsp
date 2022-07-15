# rtspclient
- Rtspclient(rfc2326)

- Rtsp/Rtsps

- H264/H265

- digest/basic


随便写了个rtsp客户端 支持rtsp和rtsps, 利用 Goroutine，可以作为客户端测试服务端的并发能力

如果需要用户名密码做鉴权，先把用户名密码写入到url中，类似这种 `rtsp://<username>:<passwd>@host:port/live/xxxx`


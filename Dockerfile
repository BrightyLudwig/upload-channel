# 作品上传通道 —— 极简分组上传站
#
# 构建（在 /home/claude/upload-channel 下）：
#   sudo docker run --rm -v $PWD:/src -w /src -e GOPROXY=off golang:1.25-alpine \
#     sh -c 'CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o upload-channel .'
#   sudo docker build -t upload-channel:latest .
#
# 运行：-p 27107:3000 选手端；-p <管理端口>:3001 主办方端（不外告，安全组单独放行）
FROM alpine:latest
RUN mkdir -p /data/uploads
COPY upload-channel /usr/local/bin/upload-channel
EXPOSE 3000 3001
# 时区：容器内不装 tzdata，只读挂载宿主机 /usr/share/zoneinfo，配合 -e TZ=Asia/Shanghai。
CMD ["/usr/local/bin/upload-channel", "-addr", ":3000", "-admin-addr", ":3001", "-data", "/data/uploads", "-config", "/data/config.json", "-groups", "15"]

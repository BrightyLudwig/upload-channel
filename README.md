# 作品上传通道 —— 极简分组上传站

部署日期：2026-09-18。自建的单文件 Go 程序，替代 Zipline / Cloudreve / PsiTransfer 等一众方案。

## 需求与结论

需求只有四条：**一个密钥登录、能上传文件、15 个组每组累计最多 1GB、中文界面。**

Zipline 三条满足、中文界面这条满足不了——Zipline v4 没有做国际化（源码里没有 i18next/next-intl，
也没有 locales 目录），界面文案硬编码在编译好的前端包里，改不干净。因此自建。
本程序只依赖 Go 标准库，编译出一个 8.3MB 静态二进制，容器内存占用约 5MB。

## 基本信息

| 项目 | 值 |
|---|---|
| 选手端 | http://121.43.195.133:27107 → 容器 3000（只有上传路由，`/admin` 是 404） |
| 主办方端 | http://121.43.195.133:29615 → 容器 3001（只有管理路由，独立密钥登录） |
| 容器 | `upload-channel`（alpine + 静态二进制，镜像 `upload-channel:latest`） |
| 数据 | `/home/claude/upload-channel/data/uploads/<组号>/`（15 个目录，文件名即原始文件名） |
| 配置 | `/home/claude/upload-channel/data/config.json`（600 权限：密钥 + 会话签名密钥 + 配额 + `admin_key`） |
| 密钥表 | `/home/claude/upload-channel/credentials.txt`（600 权限，分发给各组） |
| 配额 | 每组 1 GiB，按目录实际大小累计 |

## 主办方管理端（2026-10-17 新增）

- **入口与选手端分开端口**：选手端 27107 上不存在任何管理路由（返回 404），
  管理端只挂在 29615 上，选手即便知道地址也看不到管理登录页
- 管理密钥首次启动自动生成并写回 `config.json` 的 `admin_key` 字段（旧配置自动补）
- 登录后可见：**各队按首次上传时间排出的路演顺序**（先交先排，未上传的列最后）、
  每队文件清单（点击即下载，含每个文件的上传时间）、用量汇总
- 页面每 30 秒自动刷新；也显示各队密钥（方便主办方补发）
- **多终端自适应**（2026-10-17）：卡片式行布局替代宽表格，手机/平板/桌面都能用；
  文件清单默认折叠（`<details>`，触屏点按展开），刷新后保持展开状态；
  未上传的队压成标签区不占行；时间显示省略年份，手机上更紧凑（≤560px 有单独的紧凑断点）
- 管理下载走 `/admin/f?group=组号&name=文件名`，需管理会话，未登录 401
- **一键下载**（2026-10-19）：管理页概览卡上的「一键下载全部作品」按钮 → `GET /admin/archive`，
  流式打包全部已交作品为一个 zip（内存恒定，不落盘）。zip 内按路演顺序建目录 `01_第 15 组/…`，
  附 `路演顺序清单.txt`（含各队排名/密钥/首次上传时间/文件明细/未交名单）。
  文件用 Store 不二次压缩（作品多为 zip/视频，再压白费 CPU）。有已交队伍时按钮才显示
- 会话与选手端独立（Cookie 名 `admin`，同一签名算法、固定 gid `__admin__`）

## 功能边界（刻意做窄）

**有**：密钥登录（登录态 30 天）、拖拽/点选批量上传、上传进度条、本组文件列表（含大小）、
下载、删除、跨组隔离、登录限速（每 IP 每 2 分钟 12 次**失败**尝试）。

界面只显示「已用多少」，**不显示剩余空间、不显示总量上限**——只在真的超了的时候提示
「超过本组容量上限」，文案里同样不带数字。这是刻意的：容量上限不对外告知，
不给学生一个可以自己算的余量数字。

配套地，`credentials.txt` 里给各组看的部分也**不写容量**，容量数字只出现在文件末尾
「仅供主办方留存」那一段——分发前把这一段删掉再发。

**没有**（需求之外一律不做）：注册、邀请、邮箱、短链、相册、标签、统计、管理后台、
OAuth、Webhook、缩略图、分享链接、多语言切换。

## 运维

```bash
sudo docker logs -f upload-channel          # 看上传/删除/登录日志
sudo docker restart upload-channel          # 重启（数据在宿主机目录，随便重启）

# 查看各组用量（最常用来盯比赛进度）
du -sh /home/claude/upload-channel/data/uploads/*

# 清空某一组
sudo rm -rf /home/claude/upload-channel/data/uploads/group03/*
```

### 改配额 / 重置某组密钥

编辑 `data/config.json`，修改 `quota`（字节）或 `groups.<组号>.key`，然后 `sudo docker restart upload-channel`。
文件顶层还有 `secret`，是登录 cookie 的签名密钥，**改动会让所有人退出登录**。

### 改配置后重建（改了代码才需要）

```bash
cd /home/claude/upload-channel
sudo docker run --rm -v $PWD:/src -w /src -e GOPROXY=off golang:1.25-alpine \
  sh -c 'CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o upload-channel .'
sudo docker build -t upload-channel:latest .
sudo docker rm -f upload-channel
sudo docker run -d --name upload-channel --restart always \
  -p 27107:3000 -p 29615:3001 \
  -v /home/claude/upload-channel/data:/data \
  -v /usr/share/zoneinfo:/usr/share/zoneinfo:ro \
  -e TZ=Asia/Shanghai \
  upload-channel:latest
```

时区靠只读挂载宿主机的 `/usr/share/zoneinfo`——alpine 内 `apk add tzdata` 走默认源在国内会超时，别加。

## 安全设计

- 密钥 16 字符（字母表去掉 I/L/O/0/1 等易混字符），展示为 `XXXX-XXXX-XXXX-XXXX`，输入时忽略大小写和横线
- 登录态是 HMAC-SHA256 签名的 cookie，服务端无状态；伪造 cookie 无法通过验签（已实测）
- 所有文件操作强制限定在 `uploads/<本组>/` 内；文件名取 basename，挡路径穿越；隐藏文件（`.` 开头）不可上传
- 各组目录互不可见，跨组删除返回「文件不存在」（已实测）
- 上传先预检 Content-Length，超配额直接 413 拒绝，不浪费学生流量；流式写入时再校验一次，
  超配额即删除半截临时文件。写入全程流式，500MB 文件内存占用不涨（已实测）

## 已知取舍

- **HTTP 明文**：没有 HTTPS，密钥在链路上是明文。比赛收集作品的场景可接受；若在意，
  需上域名+证书，另配 nginx 反代。
- **无管理界面**：加组、改配额靠编辑 `config.json`。15 个组固定，比赛期间基本不动。
- **删除不可恢复**：学生可以删自己组的文件，没有回收站。日志里能看到谁删了什么。
- 已下载的密钥表在浏览器里是明文输入框（`type=password` 只是不回显，不提供更强的保护）。
- **0 字节文件的配额**：文件列表里会显示 0 B，这类文件不占用配额，删掉即可。不影响正常上传。
- **界面语言写死中文**：没有多语言切换，也不需要。

## 修复记录

### 2026-09-18 删除报「文件名不合法」

**症状**：上传能成功，点删除却提示「文件名不合法」，文件还在。

**根因**：`r.ParseForm()` **不解析 multipart/form-data 请求体**——它只处理 urlencoded，
遇到 multipart 会把请求体原样留着不管。浏览器删除文件时提交的是 `FormData`（即 multipart），
于是服务端 `r.FormValue("name")` 取到空字符串，被 `safeName` 判为非法文件名。

**为什么部署前的测试没抓到**：测试全用 `curl -d`，而 `curl -d` 发的是 urlencoded，
恰好走了 `ParseForm` 能处理的那条路。**测试工具和真实浏览器用了不同的 Content-Type，
这是这次漏测的根本原因。**

**修复**：新增 `formValue()` helper，改用 `r.ParseMultipartForm()`（它对 urlencoded 会内部转调
`ParseForm`，两种格式都能解析），并加 `http.MaxBytesReader` 限制表单体积 1MB 防滥用。
登录和删除两处都改用它。

**教训**：验证接口时要按浏览器真实的 Content-Type 测（表单接口用 `curl -F`，不是 `-d`）。

### 2026-09-18 登录被自己锁住

旧版限速按**总请求数**限（每分钟 10 次），正常登录、登出、刷新都消耗额度，多试两次就进不去。
改为只统计**失败**次数（12 次 / 2 分钟），登录成功即清零。日志中 `[登录失败] ip=...` 是密钥输错，
`[登录限速] ip=...` 才是触发封禁（防爆破，按 IP 计）。

## 部署时踩过的坑

- `apk add tzdata` 在容器构建时超时（alpine 默认源），改用挂载宿主机 zoneinfo
- Go 构建走本地 `golang:1.25-alpine` 镜像 + `GOPROXY=off`，全程不联网
- 外网访问依赖阿里云安全组放行 TCP **27107**（选手端）和 **29615**（主办方端，只发给主办方）
  （服务器本机 curl 公网 IP 不经过安全组，测不出真实可达性）

## 历史方案（均已停用，数据保留）

- Zipline（28483）：容器 `zipline` + `zipline-db`，界面英文，本方案已取代
- PsiTransfer（28481）：容器 `psitransfer`
- Cloudreve（28482）：容器已删，数据在 `/home/claude/cloudreve/data`

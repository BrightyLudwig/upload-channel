// 作品上传通道 —— 极简分组上传站
//
// 设计目标：15 个参赛组，每组一个密钥，登录后只能上传/查看/删除自己组的文件，
// 每组有独立的容量上限（具体数值只在 config.json 里，不告知各参赛组）。
// 后台零管理界面，用量直接看磁盘目录。
//
// 仅使用 Go 标准库，无外部依赖，编译为静态二进制后跑在 alpine 容器里。
package main

import (
	"archive/zip"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------- 配置 ----------

type Group struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type Config struct {
	Secret   string            `json:"secret"`
	Quota   int64             `json:"quota"`
	AdminKey string            `json:"admin_key"` // 主办方管理密钥，首次启动自动生成
	Groups  map[string]*Group `json:"groups"`
}

const adminGid = "__admin__"

const (
	defaultQuota   = 1 << 30 // 1 GiB
	sessionTTL     = 30 * 24 * time.Hour
	keyAlphabet    = "ABCDEFGHJKMNPQRSTUVWXYZ23456789" // 去掉了 I L O 0 1，避免念错
	keyGroups      = 4
	keyGroupLen    = 4
	maxNameRunes   = 120
	multipartSlack = 1 << 20 // 多部分表单边界/头部的冗余，预检时放宽 1MB
	maxFormBytes   = 1 << 20 // 表单（非文件）请求体上限，防止有人拿超大 body 打登录接口
)

// ---------- 服务 ----------

type server struct {
	cfg     *Config
	dataDir string
	locks   map[string]*sync.Mutex
	mu      sync.Mutex
	limiter *rateLimiter
}

func (s *server) groupLock(gid string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if mu, ok := s.locks[gid]; ok {
		return mu
	}
	mu := &sync.Mutex{}
	s.locks[gid] = mu
	return mu
}

func (s *server) groupDir(gid string) string { return filepath.Join(s.dataDir, gid) }

// used 返回该组已占用的字节数和文件列表。
func (s *server) used(gid string) (int64, []fileInfo, error) {
	entries, err := os.ReadDir(s.groupDir(gid))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil, nil
		}
		return 0, nil, err
	}
	var total int64
	files := make([]fileInfo, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") { // 跳过临时文件
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		total += info.Size()
		files = append(files, fileInfo{Name: name, Size: info.Size(), ModTime: info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ModTime.After(files[j].ModTime) })
	return total, files, nil
}

type fileInfo struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"-"`
}

// ---------- 会话 ----------

func (s *server) sign(gid string, exp int64) string {
	msg := fmt.Sprintf("%s|%d", gid, exp)
	mac := hmac.New(sha256.New, []byte(s.cfg.Secret))
	mac.Write([]byte(msg))
	return msg + "|" + hex.EncodeToString(mac.Sum(nil))
}

func (s *server) session(r *http.Request) (string, bool) {
	c, err := r.Cookie("session")
	if err != nil {
		return "", false
	}
	parts := strings.Split(c.Value, "|")
	if len(parts) != 3 {
		return "", false
	}
	gid, expStr := parts[0], parts[1]
	var exp int64
	parsed, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return "", false
	}
	exp = parsed
	want := s.sign(gid, exp)
	if subtle.ConstantTimeCompare([]byte(want), []byte(c.Value)) != 1 {
		return "", false
	}
	if time.Now().Unix() > exp {
		return "", false
	}
	if _, ok := s.cfg.Groups[gid]; !ok {
		return "", false
	}
	return gid, true
}

// formValue 读取表单字段，同时兼容 urlencoded 和 multipart/form-data。
//
// 注意：r.ParseForm() 只解析 urlencoded 请求体，对 multipart 是把请求体原样留着不解析的。
// 浏览器用 FormData 提交（删除文件就是），所以必须用 ParseMultipartForm，
// 它内部对 urlencoded 会转调 ParseForm，两种都能覆盖。
func formValue(w http.ResponseWriter, r *http.Request, field string) (string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	// 128KB 内存额度足够放若干个文件名，超出部分落临时文件（这里不会有）
	if err := r.ParseMultipartForm(128 << 10); err != nil {
		if err := r.ParseForm(); err != nil {
			return "", false
		}
	}
	return r.FormValue(field), true
}

// ---------- 登录限速 ----------

// rateLimiter 只统计"失败"次数：正常登录、登出、上传都不消耗额度，
// 避免学生多试两次就把自己锁在门外（旧版按总请求数限速，一分钟 10 次太紧）。
type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{hits: map[string][]time.Time{}, limit: limit, window: window}
}

// allow 判断该 IP 是否还能再试一次。
func (rl *rateLimiter) allow(ip string) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	kept := make([]time.Time, 0, len(rl.hits[ip]))
	for _, t := range rl.hits[ip] {
		if now.Sub(t) < rl.window {
			kept = append(kept, t)
		}
	}
	rl.hits[ip] = kept
	return len(kept) < rl.limit
}

// fail 记录一次失败尝试。
func (rl *rateLimiter) fail(ip string) {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.hits[ip] = append(rl.hits[ip], now)
}

// reset 登录成功后清空该 IP 的失败记录。
func (rl *rateLimiter) reset(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.hits, ip)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ---------- 工具 ----------

func normalizeKey(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	r := strings.NewReplacer("-", "", " ", "", "　", "", "_", "")
	return r.Replace(s)
}

// safeName 只保留纯文件名，挡掉路径穿越和隐藏文件。
func safeName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, ".") {
		return ""
	}
	runes := []rune(name)
	if len(runes) > maxNameRunes {
		ext := filepath.Ext(name)
		extRunes := []rune(ext)
		if len(extRunes) > 16 {
			ext, extRunes = "", nil
		}
		name = string(runes[:maxNameRunes-len(extRunes)]) + ext
	}
	return name
}

// uniqueName 处理重名：文件名 → 文件名 (2) → 文件名 (3)
func uniqueName(dir, name string) string {
	if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 2; i < 1000; i++ {
		cand := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(filepath.Join(dir, cand)); os.IsNotExist(err) {
			return cand
		}
	}
	return fmt.Sprintf("%s (%d)%s", base, time.Now().UnixNano(), ext)
}

func humanSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1<<20:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	case n < 1<<30:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	default:
		return fmt.Sprintf("%.2f GB", float64(n)/(1<<30))
	}
}

// 注意：面向学生的提示语里不出现具体配额数字（不告知各组有 1GB 限制），
// 只说"超过本组容量上限"。要看配额改 config.json 的 quota 字段。

// quotaText 仅用于服务端日志，不进入任何返回给前端的文案。
func quotaText(quota int64) string {
	if quota%(1<<30) == 0 {
		return fmt.Sprintf("%dGB", quota/(1<<30))
	}
	return humanSize(quota)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"ok": false, "error": msg})
}

func randKey() string {
	var b strings.Builder
	for i := 0; i < keyGroups; i++ {
		if i > 0 {
			b.WriteByte('-')
		}
		for j := 0; j < keyGroupLen; j++ {
			n, err := rand.Int(rand.Reader, big.NewInt(int64(len(keyAlphabet))))
			if err != nil {
				panic(err)
			}
			b.WriteByte(keyAlphabet[n.Int64()])
		}
	}
	return b.String()
}

// ---------- HTTP ----------

// routes 选手端口：只有上传相关路由，不存在任何管理入口。
func (s *server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("POST /login", s.handleLogin)
	mux.HandleFunc("GET /logout", s.handleLogout)
	mux.HandleFunc("GET /api/files", s.handleList)
	mux.HandleFunc("POST /upload", s.handleUpload)
	mux.HandleFunc("POST /api/delete", s.handleDelete)
	mux.HandleFunc("GET /f/{name}", s.handleDownload)
	return mux
}

// adminRoutes 管理端口：只挂管理端路由，根路径直接跳到管理登录页。
func (s *server) adminRoutes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
	})
	mux.HandleFunc("GET /admin", s.handleAdminIndex)
	mux.HandleFunc("POST /admin/login", s.handleAdminLogin)
	mux.HandleFunc("GET /admin/logout", s.handleAdminLogout)
	mux.HandleFunc("GET /api/admin/groups", s.handleAdminData)
	mux.HandleFunc("GET /admin/f", s.handleAdminDownload)
	mux.HandleFunc("GET /admin/archive", s.handleAdminArchive)
	return mux
}

// ---------- 主办方管理端 ----------

// adminSession 校验主办方会话 Cookie（与小组会话同算法，但走独立的 Cookie 名和固定 gid）。
func (s *server) adminSession(r *http.Request) bool {
	c, err := r.Cookie("admin")
	if err != nil {
		return false
	}
	parts := strings.Split(c.Value, "|")
	if len(parts) != 3 {
		return false
	}
	gid, expStr := parts[0], parts[1]
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || gid != adminGid {
		return false
	}
	want := s.sign(adminGid, exp)
	if subtle.ConstantTimeCompare([]byte(want), []byte(c.Value)) != 1 {
		return false
	}
	return time.Now().Unix() <= exp
}

func (s *server) handleAdminIndex(w http.ResponseWriter, r *http.Request) {
	if s.adminSession(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = adminTmpl.Execute(w, map[string]any{})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = adminLoginTmpl.Execute(w, map[string]any{"Error": ""})
}

func (s *server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.limiter.allow(ip) {
		log.Printf("[管理登录限速] ip=%s", ip)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = adminLoginTmpl.Execute(w, map[string]any{"Error": "密钥错误次数过多，请稍等片刻再试"})
		return
	}
	key, ok := formValue(w, r, "key")
	if !ok {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = adminLoginTmpl.Execute(w, map[string]any{"Error": "提交内容无法解析，请重试"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(normalizeKey(s.cfg.AdminKey)), []byte(normalizeKey(key))) != 1 {
		s.limiter.fail(ip)
		log.Printf("[管理登录失败] ip=%s", ip)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = adminLoginTmpl.Execute(w, map[string]any{"Error": "密钥无效，请核对后重试"})
		return
	}
	s.limiter.reset(ip)
	exp := time.Now().Add(sessionTTL).Unix()
	http.SetCookie(w, &http.Cookie{
		Name:     "admin",
		Value:    s.sign(adminGid, exp),
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	log.Printf("[管理登录成功] ip=%s", ip)
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "admin", Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// adminFile / adminGroup 是 /api/admin/groups 的返回结构。
// Time 用本地时区格式化成字符串，前端直接展示。
type adminFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Time string `json:"time"`
}

type adminGroup struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Key         string      `json:"key"`
	Uploaded    bool        `json:"uploaded"`
	FirstUpload string      `json:"firstUpload,omitempty"`
	LastUpload  string      `json:"lastUpload,omitempty"`
	FirstSort   int64       `json:"-"`
	FileCount   int         `json:"fileCount"`
	Total       int64       `json:"total"`
	Files       []adminFile `json:"files"`
}

// handleAdminData 返回全部小组的上传情况，已上传的按首次上传时间升序（即路演顺序），
// 未上传的排在后面。
func (s *server) handleAdminData(w http.ResponseWriter, r *http.Request) {
	if !s.adminSession(r) {
		writeErr(w, http.StatusUnauthorized, "登录已过期，请重新登录")
		return
	}
	groups := make([]adminGroup, 0, len(s.cfg.Groups))
	for gid, g := range s.cfg.Groups {
		ag := adminGroup{ID: gid, Name: g.Name, Key: g.Key, Files: []adminFile{}}
		mu := s.groupLock(gid)
		mu.Lock()
		_, files, err := s.used(gid)
		mu.Unlock()
		if err != nil {
			log.Printf("[管理端读目录失败] %s: %v", gid, err)
		}
		// used() 返回的是按时间倒序，这里要按时间正序（上传顺序）
		for i := len(files) - 1; i >= 0; i-- {
			f := files[i]
			ag.Files = append(ag.Files, adminFile{
				Name: f.Name, Size: f.Size,
				Time: f.ModTime.Format("2006-01-02 15:04:05"),
			})
			ag.Total += f.Size
			if ag.FirstSort == 0 || f.ModTime.Unix() < ag.FirstSort {
				ag.FirstSort = f.ModTime.Unix()
				ag.FirstUpload = f.ModTime.Format("2006-01-02 15:04:05")
			}
			ag.LastUpload = f.ModTime.Format("2006-01-02 15:04:05")
		}
		ag.FileCount = len(ag.Files)
		ag.Uploaded = ag.FileCount > 0
		groups = append(groups, ag)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		// 已上传的在前，按首次上传时间排；都未上传则按组号排
		if groups[i].Uploaded != groups[j].Uploaded {
			return groups[i].Uploaded
		}
		if groups[i].Uploaded && groups[j].Uploaded {
			return groups[i].FirstSort < groups[j].FirstSort
		}
		return groups[i].ID < groups[j].ID
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"groups": groups,
		"quota":  s.cfg.Quota,
	})
}

// handleAdminDownload 主办方下载任意小组的文件。
func (s *server) handleAdminDownload(w http.ResponseWriter, r *http.Request) {
	if !s.adminSession(r) {
		http.Error(w, "登录已过期，请重新登录", http.StatusUnauthorized)
		return
	}
	gid := r.URL.Query().Get("group")
	if _, ok := s.cfg.Groups[gid]; !ok {
		http.NotFound(w, r)
		return
	}
	name := safeName(r.URL.Query().Get("name"))
	if name == "" {
		http.NotFound(w, r)
		return
	}
	mu := s.groupLock(gid)
	mu.Lock()
	path := filepath.Join(s.groupDir(gid), name)
	info, err := os.Stat(path)
	mu.Unlock()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	log.Printf("[管理端下载] %s 文件=%q ip=%s", s.cfg.Groups[gid].Name, name, clientIP(r))
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+urlEncode(name))
	http.ServeFile(w, r, path)
}

// handleAdminArchive 一键打包下载全部已交作品（zip，按路演顺序建组目录，附清单文件）。
// 流式写出，不落盘、内存占用恒定；文件用 Store（不二次压缩）——作品多为 zip/视频等
// 已压缩格式，再压一遍白费 CPU 还慢。
func (s *server) handleAdminArchive(w http.ResponseWriter, r *http.Request) {
	if !s.adminSession(r) {
		http.Error(w, "登录已过期，请重新登录", http.StatusUnauthorized)
		return
	}

	// 先按路演顺序收集各组文件（复用 handleAdminData 的排序规则）
	type grpFiles struct {
		id      string
		name    string
		files   []fileInfo
		first   time.Time
		hasFirst bool
	}
	groups := make([]grpFiles, 0, len(s.cfg.Groups))
	for gid, g := range s.cfg.Groups {
		gf := grpFiles{id: gid, name: g.Name}
		mu := s.groupLock(gid)
		mu.Lock()
		_, files, err := s.used(gid)
		mu.Unlock()
		if err != nil {
			log.Printf("[打包] 读目录失败 %s: %v", gid, err)
			continue
		}
		// used() 返回倒序，翻成正序（上传顺序）
		for i := len(files) - 1; i >= 0; i-- {
			if !gf.hasFirst || files[i].ModTime.Before(gf.first) {
				gf.first = files[i].ModTime
				gf.hasFirst = true
			}
		}
		gf.files = files
		groups = append(groups, gf)
	}
	var uploaded, pending []grpFiles
	for _, gf := range groups {
		if len(gf.files) > 0 {
			uploaded = append(uploaded, gf)
		} else {
			pending = append(pending, gf)
		}
	}
	sort.SliceStable(uploaded, func(i, j int) bool {
		if !uploaded[i].hasFirst || !uploaded[j].hasFirst {
			return uploaded[i].id < uploaded[j].id
		}
		return uploaded[i].first.Before(uploaded[j].first)
	})
	if len(uploaded) == 0 {
		http.Error(w, "还没有任何队伍上传作品", http.StatusNotFound)
		return
	}

	log.Printf("[打包下载] %d 队 %d 文件 ip=%s", len(uploaded), func() int {
		n := 0
		for _, g := range uploaded {
			n += len(g.files)
		}
		return n
	}(), clientIP(r))

	zipName := "作品提交-" + time.Now().Format("20060102-150405") + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+urlEncode(zipName))

	zw := zip.NewWriter(w)
	// 清单：路演顺序 + 各队情况
	manifest := &strings.Builder{}
	fmt.Fprintf(manifest, "作品提交清单（生成于 %s）\r\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(manifest, "路演顺序按各组首次上传时间排定，先交先排。\r\n\r\n")
	for i, gf := range uploaded {
		fmt.Fprintf(manifest, "第 %d 名：%s（密钥 %s）\r\n", i+1, gf.name, s.cfg.Groups[gf.id].Key)
		if gf.hasFirst {
			fmt.Fprintf(manifest, "  首次上传：%s\r\n", gf.first.Format("2006-01-02 15:04:05"))
		}
		for _, f := range gf.files {
			fmt.Fprintf(manifest, "  - %s（%s，%s）\r\n", f.Name, humanSize(f.Size), f.ModTime.Format("2006-01-02 15:04:05"))
		}
	}
	if len(pending) > 0 {
		fmt.Fprintf(manifest, "\r\n尚未上传的队伍：\r\n")
		for _, gf := range pending {
			fmt.Fprintf(manifest, "  - %s（密钥 %s）\r\n", gf.name, s.cfg.Groups[gf.id].Key)
		}
	}
	if mw, err := zw.CreateHeader(&zip.FileHeader{
		Name:     "路演顺序清单.txt",
		Method:   zip.Deflate,
		Modified: time.Now(),
	}); err == nil {
		_, _ = io.Copy(mw, strings.NewReader(manifest.String()))
	}

	for i, gf := range uploaded {
		dir := fmt.Sprintf("%02d_%s/", i+1, gf.name)
		for _, f := range gf.files {
			// 只在 open 时持锁；unix 下已打开的 fd 不受后续删除影响，避免大文件复制期间阻塞该组上传
			mu := s.groupLock(gf.id)
			mu.Lock()
			src, err := os.Open(filepath.Join(s.groupDir(gf.id), f.Name))
			mu.Unlock()
			if err != nil {
				log.Printf("[打包] 跳过 %s/%s: %v", gf.name, f.Name, err)
				continue
			}
			hdr := &zip.FileHeader{
				Name:     dir + f.Name,
				Method:   zip.Store,
				Modified: f.ModTime,
			}
			hw, err := zw.CreateHeader(hdr)
			if err != nil {
				_ = src.Close()
				continue
			}
			_, _ = io.Copy(hw, src)
			_ = src.Close()
		}
	}
	_ = zw.Close()
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if gid, ok := s.session(r); ok {
		s.renderMain(w, gid)
		return
	}
	s.renderLogin(w, "")
}

func (s *server) renderLogin(w http.ResponseWriter, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = loginTmpl.Execute(w, map[string]any{"Error": errMsg})
}

func (s *server) renderMain(w http.ResponseWriter, gid string) {
	g := s.cfg.Groups[gid]
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = mainTmpl.Execute(w, map[string]any{
		"GroupID":   gid,
		"GroupName": g.Name,
		"Quota":     s.cfg.Quota,
	})
}

func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.limiter.allow(ip) {
		log.Printf("[登录限速] ip=%s", ip)
		s.renderLogin(w, "密钥错误次数过多，请稍等片刻再试")
		return
	}
	key, ok := formValue(w, r, "key")
	if !ok {
		s.renderLogin(w, "提交内容无法解析，请重试")
		return
	}
	key = normalizeKey(key)
	if key == "" {
		s.renderLogin(w, "请输入小组密钥")
		return
	}
	var gid string
	for id, g := range s.cfg.Groups {
		if subtle.ConstantTimeCompare([]byte(normalizeKey(g.Key)), []byte(key)) == 1 {
			gid = id
			break
		}
	}
	if gid == "" {
		s.limiter.fail(ip)
		log.Printf("[登录失败] ip=%s", ip)
		s.renderLogin(w, "密钥无效，请核对后重试")
		return
	}
	s.limiter.reset(ip)
	exp := time.Now().Add(sessionTTL).Unix()
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    s.sign(gid, exp),
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	log.Printf("[登录成功] %s ip=%s", s.cfg.Groups[gid].Name, ip)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *server) handleList(w http.ResponseWriter, r *http.Request) {
	gid, ok := s.session(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "登录已过期，请重新登录")
		return
	}
	mu := s.groupLock(gid)
	mu.Lock()
	used, files, err := s.used(gid)
	mu.Unlock()
	if err != nil {
		log.Printf("[列目录失败] %s: %v", gid, err)
		writeErr(w, http.StatusInternalServerError, "读取文件列表失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"used":  used,
		"quota": s.cfg.Quota,
		"files": files,
	})
}

var errQuota = errors.New("超过配额")

func (s *server) handleUpload(w http.ResponseWriter, r *http.Request) {
	gid, ok := s.session(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "登录已过期，请重新登录")
		return
	}
	mu := s.groupLock(gid)
	mu.Lock()
	defer mu.Unlock()

	used, _, err := s.used(gid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取用量失败")
		return
	}
	remaining := s.cfg.Quota - used

	// 提前预检：Content-Length 明显超出剩余配额时直接拒绝，省得白传一趟。
	// 提示只说"超过容量上限"，既不透露还剩多少，也不透露上限是多少。
	if r.ContentLength > 0 && r.ContentLength > remaining+multipartSlack {
		log.Printf("[上传拒绝-超配额] %s 配额=%s 请求=%s ip=%s",
			s.cfg.Groups[gid].Name, quotaText(s.cfg.Quota), humanSize(r.ContentLength), clientIP(r))
		writeErr(w, http.StatusRequestEntityTooLarge, "上传失败：超过本组容量上限")
		return
	}

	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "上传格式不正确")
		return
	}

	dir := s.groupDir(gid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, "创建目录失败")
		return
	}

	var (
		uploaded []map[string]any
		problems []string
		written  int64
	)
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			problems = append(problems, "上传中断："+err.Error())
			break
		}
		if part.FormName() != "files" || part.FileName() == "" {
			_ = part.Close()
			continue
		}
		name := safeName(part.FileName())
		if name == "" {
			problems = append(problems, "文件名不合法，已跳过")
			_ = part.Close()
			continue
		}
		if remaining-written <= 0 {
			problems = append(problems, fmt.Sprintf("「%s」未上传：本组容量已用满，请联系主办方", name))
			_ = part.Close()
			continue
		}
		final := uniqueName(dir, name)
		tmp := filepath.Join(dir, fmt.Sprintf(".tmp-%d-%s", time.Now().UnixNano(), randHex(6)))
		f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
		if err != nil {
			problems = append(problems, fmt.Sprintf("「%s」未上传：%v", name, err))
			_ = part.Close()
			continue
		}
		// 留出余量，避免写满后才发现超配额
		n, cErr := io.Copy(f, io.LimitReader(part, remaining-written+1))
		closeErr := f.Close()
		_ = part.Close()
		if cErr != nil || closeErr != nil {
			_ = os.Remove(tmp)
			problems = append(problems, fmt.Sprintf("「%s」上传失败：写入中断", name))
			continue
		}
		if written+n > remaining {
			_ = os.Remove(tmp)
			problems = append(problems, fmt.Sprintf("「%s」未上传：超过本组容量上限", name))
			continue
		}
		if err := os.Rename(tmp, filepath.Join(dir, final)); err != nil {
			_ = os.Remove(tmp)
			problems = append(problems, fmt.Sprintf("「%s」保存失败", name))
			continue
		}
		written += n
		uploaded = append(uploaded, map[string]any{"name": final, "size": n})
		log.Printf("[上传] %s 文件=%q 大小=%s ip=%s", s.cfg.Groups[gid].Name, final, humanSize(n), clientIP(r))
	}

	if len(uploaded) == 0 && len(problems) == 0 {
		writeErr(w, http.StatusBadRequest, "没有收到文件")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"uploaded": uploaded,
		"problems": problems,
	})
}

func (s *server) handleDelete(w http.ResponseWriter, r *http.Request) {
	gid, ok := s.session(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "登录已过期，请重新登录")
		return
	}
	name, ok := formValue(w, r, "name")
	if !ok {
		writeErr(w, http.StatusBadRequest, "提交内容无法解析")
		return
	}
	name = safeName(name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "文件名不合法")
		return
	}
	mu := s.groupLock(gid)
	mu.Lock()
	defer mu.Unlock()
	target := filepath.Join(s.groupDir(gid), name)
	info, err := os.Stat(target)
	if err != nil || info.IsDir() {
		writeErr(w, http.StatusNotFound, "文件不存在")
		return
	}
	if err := os.Remove(target); err != nil {
		writeErr(w, http.StatusInternalServerError, "删除失败")
		return
	}
	log.Printf("[删除] %s 文件=%q ip=%s", s.cfg.Groups[gid].Name, name, clientIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *server) handleDownload(w http.ResponseWriter, r *http.Request) {
	gid, ok := s.session(r)
	if !ok {
		http.Error(w, "登录已过期，请重新登录", http.StatusUnauthorized)
		return
	}
	name := safeName(r.PathValue("name"))
	if name == "" {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.groupDir(gid), name)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+urlEncode(name))
	http.ServeFile(w, r, path)
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func urlEncode(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// ---------- 配置加载 ----------

func loadConfig(path string, count int) (*Config, error) {
	if raw, err := os.ReadFile(path); err == nil {
		var cfg Config
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, fmt.Errorf("解析 %s 失败: %w", path, err)
		}
		if cfg.Secret == "" || len(cfg.Groups) == 0 {
			return nil, fmt.Errorf("%s 内容不完整", path)
		}
		if cfg.Quota <= 0 {
			cfg.Quota = defaultQuota
		}
		// 旧版本生成的配置没有管理密钥，补生成并写回，这样不用手工改 config.json
		if cfg.AdminKey == "" {
			cfg.AdminKey = randKey()
			if raw, err := json.MarshalIndent(cfg, "", "  "); err == nil {
				if err := os.WriteFile(path, raw, 0o600); err != nil {
					return nil, fmt.Errorf("写入管理密钥失败: %w", err)
				}
				log.Printf("已生成主办方管理密钥并写回 %s", path)
			} else {
				return nil, err
			}
		}
		return &cfg, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	cfg := &Config{
		Secret: randHex(32),
		Quota:  defaultQuota,
		Groups: map[string]*Group{},
	}
	for i := 1; i <= count; i++ {
		gid := fmt.Sprintf("group%02d", i)
		cfg.Groups[gid] = &Group{Name: fmt.Sprintf("第 %d 组", i), Key: randKey()}
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return nil, err
	}
	log.Printf("已生成新配置 %s（%d 个小组）", path, count)
	return cfg, nil
}

func main() {
	addr := flag.String("addr", ":3000", "选手端监听地址")
	adminAddr := flag.String("admin-addr", "", "主办方管理端监听地址（留空则不启动管理端）")
	dataDir := flag.String("data", "/data/uploads", "上传文件存放目录")
	cfgPath := flag.String("config", "/data/config.json", "配置文件路径")
	groups := flag.Int("groups", 15, "小组数量（仅在首次生成配置时生效）")
	flag.Parse()

	cfg, err := loadConfig(*cfgPath, *groups)
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}
	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("无法创建数据目录: %v", err)
	}
	for gid := range cfg.Groups {
		if err := os.MkdirAll(filepath.Join(*dataDir, gid), 0o755); err != nil {
			log.Fatalf("无法创建 %s 目录: %v", gid, err)
		}
	}

	s := &server{
		cfg:     cfg,
		dataDir: *dataDir,
		locks:   map[string]*sync.Mutex{},
		limiter: newRateLimiter(12, 2*time.Minute),
	}

	log.Printf("作品上传通道启动于 %s，数据目录 %s，%d 个小组，每组配额 %s",
		*addr, *dataDir, len(cfg.Groups), humanSize(cfg.Quota))

	srv := &http.Server{
		Addr:              *addr,
		Handler:           securityHeaders(s.routes()),
		ReadHeaderTimeout: 15 * time.Second,
		// 不设 ReadTimeout / WriteTimeout：大文件上传会超过默认时限
		IdleTimeout: 120 * time.Second,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Fatalf("服务退出: %v", err)
		}
	}()

	if *adminAddr != "" {
		adminSrv := &http.Server{
			Addr:              *adminAddr,
			Handler:           securityHeaders(s.adminRoutes()),
			ReadHeaderTimeout: 15 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
		log.Printf("主办方管理端启动于 %s（管理密钥见 config.json 的 admin_key 字段）", *adminAddr)
		if err := adminSrv.ListenAndServe(); err != nil {
			log.Fatalf("管理端退出: %v", err)
		}
	} else {
		select {} // 只跑选手端时阻塞住
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy",
			"default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}

// ---------- 页面 ----------

var loginTmpl = template.Must(template.New("login").Parse(loginHTML))
var mainTmpl = template.Must(template.New("main").Parse(mainHTML))
var adminLoginTmpl = template.Must(template.New("adminLogin").Parse(adminLoginHTML))
var adminTmpl = template.Must(template.New("admin").Parse(adminHTML))

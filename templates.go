package main

// 页面模板。全中文、无外链资源（不引 CDN，离线可用）。

const pageStyle = `
:root {
  --bg: #f5f6f8; --card: #fff; --fg: #1c1f23; --muted: #6b7280;
  --line: #e3e6ea; --accent: #2f6feb; --accent-hover: #245cc7;
  --warn: #c2410c; --warn-bg: #fff4ed; --ok: #17803d; --danger: #c02626;
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #16181d; --card: #1f2229; --fg: #e8eaed; --muted: #9aa2ae;
    --line: #31353d; --accent: #5b8def; --accent-hover: #7aa3f2;
    --warn: #f0a06a; --warn-bg: #35281f; --ok: #5cc98a; --danger: #f07777;
  }
}
* { box-sizing: border-box; }
body {
  margin: 0; background: var(--bg); color: var(--fg);
  font-family: system-ui, -apple-system, "PingFang SC", "Microsoft YaHei", "Noto Sans CJK SC", sans-serif;
  font-size: 15px; line-height: 1.6;
}
.wrap { max-width: 720px; margin: 0 auto; padding: 28px 20px 60px; }
.card { background: var(--card); border: 1px solid var(--line); border-radius: 12px; padding: 22px; }
h1 { font-size: 20px; margin: 0 0 4px; }
.sub { color: var(--muted); font-size: 13px; margin: 0 0 20px; }
label { display: block; font-size: 13px; color: var(--muted); margin-bottom: 6px; }
input[type=text], input[type=password] {
  width: 100%; padding: 11px 13px; font-size: 16px; font-family: inherit;
  color: var(--fg); background: var(--bg);
  border: 1px solid var(--line); border-radius: 8px; outline: none;
}
input:focus { border-color: var(--accent); }
button {
  font-family: inherit; font-size: 15px; border: 0; border-radius: 8px;
  padding: 11px 20px; cursor: pointer; background: var(--accent); color: #fff;
}
button:hover { background: var(--accent-hover); }
button:disabled { opacity: .5; cursor: default; }
button.ghost { background: transparent; color: var(--muted); border: 1px solid var(--line); padding: 7px 13px; font-size: 13px; }
button.ghost:hover { color: var(--fg); border-color: var(--muted); background: transparent; }
.err { background: var(--warn-bg); color: var(--warn); border-radius: 8px; padding: 10px 13px; font-size: 14px; margin-bottom: 16px; }
.top { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; margin-bottom: 18px; }
.usage { font-size: 14px; color: var(--muted); margin-bottom: 8px; }
.usage b { color: var(--fg); font-weight: 600; }
.bar { height: 8px; background: var(--line); border-radius: 99px; overflow: hidden; margin-bottom: 22px; }
.bar > i { display: block; height: 100%; background: var(--accent); border-radius: 99px; transition: width .3s; }
.bar > i.full { background: var(--danger); }
.drop {
  border: 2px dashed var(--line); border-radius: 12px; padding: 34px 20px;
  text-align: center; cursor: pointer; transition: .15s; background: var(--bg);
}
.drop:hover, .drop.over { border-color: var(--accent); background: rgba(47,111,235,.06); }
.drop .big { font-size: 15px; margin-bottom: 4px; }
.drop .hint { font-size: 13px; color: var(--muted); }
.drop.dim { opacity: .5; cursor: not-allowed; }
.progress { margin-top: 14px; display: none; }
.progress.on { display: block; }
.progress .bar { margin: 6px 0 0; }
.progress .txt { font-size: 13px; color: var(--muted); display: flex; justify-content: space-between; }
.msg { margin-top: 14px; font-size: 14px; border-radius: 8px; padding: 10px 13px; display: none; }
.msg.on { display: block; }
.msg.ok { background: rgba(23,128,61,.1); color: var(--ok); }
.msg.bad { background: var(--warn-bg); color: var(--warn); }
h2 { font-size: 15px; margin: 26px 0 10px; font-weight: 600; }
h2 span { color: var(--muted); font-weight: 400; font-size: 13px; }
ul.files { list-style: none; margin: 0; padding: 0; border: 1px solid var(--line); border-radius: 10px; overflow: hidden; }
ul.files li { display: flex; align-items: center; gap: 12px; padding: 11px 14px; border-top: 1px solid var(--line); font-size: 14px; }
ul.files li:first-child { border-top: 0; }
ul.files .nm { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
ul.files .nm a { color: var(--fg); text-decoration: none; }
ul.files .nm a:hover { color: var(--accent); text-decoration: underline; }
ul.files .sz { color: var(--muted); font-size: 13px; white-space: nowrap; }
ul.files .del { background: none; border: 0; color: var(--muted); padding: 4px 6px; font-size: 13px; border-radius: 6px; }
ul.files .del:hover { color: var(--danger); background: rgba(192,38,38,.08); }
.empty { color: var(--muted); font-size: 14px; padding: 20px; text-align: center; border: 1px solid var(--line); border-radius: 10px; }
`

const loginHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>作品上传通道</title>
<style>` + pageStyle + `</style>
</head>
<body>
<div class="wrap">
  <div class="card">
    <h1>作品上传通道</h1>
    <p class="sub">请输入本组密钥登录，登录后可上传和查看本组文件。</p>
    {{if .Error}}<div class="err">{{.Error}}</div>{{end}}
    <form method="post" action="/login">
      <label for="key">小组密钥</label>
      <input id="key" name="key" type="password" autocomplete="off" autofocus
             placeholder="例如：ABCD-EFGH-JKLM-NPQR" spellcheck="false">
      <div style="margin-top:16px">
        <button type="submit" style="width:100%">进 入</button>
      </div>
    </form>
  </div>
  <p class="sub" style="text-align:center;margin-top:18px">密钥由主办方发放，遗失请联系主办方重置。</p>
</div>
</body>
</html>`

const mainHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.GroupName}} · 作品上传通道</title>
<style>` + pageStyle + `</style>
</head>
<body>
<div class="wrap" id="app" data-group="{{.GroupName}}" data-quota="{{.Quota}}">
  <div class="top">
    <div>
      <h1>{{.GroupName}}</h1>
      <p class="sub" style="margin:0">作品上传通道</p>
    </div>
    <a href="/logout"><button class="ghost" type="button">退出登录</button></a>
  </div>

  <div class="card">
    <div class="usage" id="usage">已用 <b>—</b></div>
    <div class="bar"><i id="barfill" style="width:0"></i></div>

    <div class="drop" id="drop">
      <div class="big">把文件拖到这里</div>
      <div class="hint">或点击选择文件 · 可一次选多个</div>
      <input type="file" id="picker" multiple hidden>
    </div>

    <div class="progress" id="prog">
      <div class="txt"><span id="progname">正在上传…</span><span id="progpc">0%</span></div>
      <div class="bar"><i id="progbar" style="width:0"></i></div>
    </div>

    <div class="msg" id="msg"></div>
  </div>

  <h2>本组文件 <span id="fcount"></span></h2>
  <div id="flist"></div>
</div>

<script>
(function () {
  var app = document.getElementById('app');
  var quota = parseInt(app.dataset.quota, 10) || 0;
  var $drop = document.getElementById('drop'), $picker = document.getElementById('picker');
  var $usage = document.getElementById('usage'), $bar = document.getElementById('barfill');
  var $prog = document.getElementById('prog'), $progbar = document.getElementById('progbar');
  var $progpc = document.getElementById('progpc'), $progname = document.getElementById('progname');
  var $msg = document.getElementById('msg'), $flist = document.getElementById('flist');
  var $fcount = document.getElementById('fcount');
  var busy = false, used = 0;

  function fmt(n) {
    if (n < 1024) return n + ' B';
    if (n < 1048576) return (n / 1024).toFixed(1) + ' KB';
    if (n < 1073741824) return (n / 1048576).toFixed(1) + ' MB';
    return (n / 1073741824).toFixed(2) + ' GB';
  }
  function say(text, ok) {
    $msg.textContent = text;
    $msg.className = 'msg on ' + (ok ? 'ok' : 'bad');
    if (ok) setTimeout(function () { $msg.className = 'msg'; }, 4000);
  }

  function refresh() {
    fetch('/api/files', { credentials: 'same-origin' }).then(function (r) {
      if (r.status === 401) { location.reload(); return; }
      return r.json();
    }).then(function (d) {
      if (!d || !d.ok) return;
      used = d.used;
      var pct = quota ? Math.min(100, used / quota * 100) : 0;
      // 只显示"已用"，不显示"剩余"，也不显示总量
      $usage.innerHTML = '已用 <b>' + fmt(used) + '</b>';
      $bar.style.width = pct + '%';
      $bar.className = pct >= 99 ? 'full' : '';
      $drop.className = 'drop' + (used >= quota ? ' dim' : '');
      $fcount.textContent = d.files.length ? '（' + d.files.length + ' 个）' : '';
      if (!d.files.length) {
        $flist.innerHTML = '<div class="empty">还没有上传任何文件</div>';
        return;
      }
      var ul = document.createElement('ul');
      ul.className = 'files';
      d.files.forEach(function (f) {
        var li = document.createElement('li');
        var nm = document.createElement('div');
        nm.className = 'nm';
        var a = document.createElement('a');
        a.href = '/f/' + encodeURIComponent(f.name);
        a.textContent = f.name;
        nm.appendChild(a);
        var sz = document.createElement('div');
        sz.className = 'sz';
        sz.textContent = fmt(f.size);
        var del = document.createElement('button');
        del.className = 'del';
        del.type = 'button';
        del.textContent = '删除';
        del.onclick = function () {
          if (!confirm('确定删除「' + f.name + '」？删除后无法恢复。')) return;
          var fd = new FormData();
          fd.append('name', f.name);
          fetch('/api/delete', { method: 'POST', body: fd, credentials: 'same-origin' })
            .then(function (r) { return r.json(); })
            .then(function (res) {
              if (res.ok) { say('已删除「' + f.name + '」', true); refresh(); }
              else { say(res.error || '删除失败', false); }
            })
            .catch(function () { say('网络异常，删除失败', false); });
        };
        li.appendChild(nm); li.appendChild(sz); li.appendChild(del);
        ul.appendChild(li);
      });
      $flist.innerHTML = '';
      $flist.appendChild(ul);
    }).catch(function () {});
  }

  function upload(files) {
    if (busy || !files.length) return;
    // 不在前端算剩余空间（那等于把剩余量泄露给学生）。只做"已经满了"的短路，
    // 真正的配额判断交给服务端，超了由服务端返回提示。
    if (quota && used >= quota) {
      say('本组空间已用满，无法继续上传，请联系主办方', false);
      return;
    }
    var fd = new FormData();
    for (var j = 0; j < files.length; j++) fd.append('files', files[j], files[j].name);

    busy = true;
    $picker.disabled = true;
    $prog.className = 'progress on';
    $progname.textContent = files.length > 1 ? ('正在上传 ' + files.length + ' 个文件…') : ('正在上传 ' + files[0].name);
    $progbar.style.width = '0%';
    $progpc.textContent = '0%';

    var xhr = new XMLHttpRequest();
    xhr.open('POST', '/upload');
    xhr.withCredentials = true;
    xhr.upload.onprogress = function (e) {
      if (!e.lengthComputable) return;
      var p = Math.round(e.loaded / e.total * 100);
      $progbar.style.width = p + '%';
      $progpc.textContent = p + '%';
    };
    xhr.onload = function () {
      busy = false;
      $picker.disabled = false;
      $prog.className = 'progress';
      var res = {};
      try { res = JSON.parse(xhr.responseText); } catch (e) {}
      if (xhr.status === 401) { location.reload(); return; }
      if (res.ok) {
        var n = res.uploaded.length;
        var text = n ? ('上传成功：' + n + ' 个文件，共 ' + fmt(res.uploaded.reduce(function (a, b) { return a + b.size; }, 0))) : '';
        if (res.problems && res.problems.length) {
          say((text ? text + '；' : '') + res.problems.join('；'), false);
        } else {
          say(text || '上传完成', true);
        }
        refresh();
      } else {
        say(res.error || ('上传失败（' + xhr.status + '）'), false);
      }
    };
    xhr.onerror = function () {
      busy = false;
      $picker.disabled = false;
      $prog.className = 'progress';
      say('网络中断，上传失败。请检查网络后重试。', false);
    };
    xhr.send(fd);
  }

  $drop.onclick = function () { if (!busy && used < quota) $picker.click(); };
  $picker.onchange = function () { upload($picker.files); $picker.value = ''; };

  ['dragenter', 'dragover'].forEach(function (ev) {
    $drop.addEventListener(ev, function (e) { e.preventDefault(); if (!busy) $drop.classList.add('over'); });
  });
  ['dragleave', 'drop'].forEach(function (ev) {
    $drop.addEventListener(ev, function (e) { e.preventDefault(); $drop.classList.remove('over'); });
  });
  $drop.addEventListener('drop', function (e) {
    if (busy || used >= quota) return;
    if (e.dataTransfer && e.dataTransfer.files.length) upload(e.dataTransfer.files);
  });
  window.addEventListener('dragover', function (e) { e.preventDefault(); });
  window.addEventListener('drop', function (e) { e.preventDefault(); });

  refresh();
})();
</script>
</body>
</html>`

// ---------- 主办方管理端页面 ----------

// 多终端自适应：手机上堆叠、桌面宽松；文件清单默认折叠（触屏好点按）。
const adminStyle = `
.meta-row { display: flex; align-items: center; gap: 16px; flex-wrap: wrap; font-size: 14px; color: var(--muted); }
.meta-row b { color: var(--fg); font-weight: 600; }
.meta-row .dot { opacity: .5; }
.updated { font-size: 12px; color: var(--muted); }
h2.sec { margin: 22px 0 10px; font-size: 15px; font-weight: 600; }
h2.sec span { color: var(--muted); font-weight: 400; font-size: 13px; }

.team { display: flex; gap: 14px; padding: 14px 16px; border-bottom: 1px solid var(--line); }
.team:last-child { border-bottom: 0; }
.rank {
  flex: 0 0 46px; width: 46px; height: 46px; border-radius: 12px;
  background: rgba(47,111,235,.1); color: var(--accent);
  font-size: 20px; font-weight: 700; display: flex; align-items: center; justify-content: center;
}
.tmain { flex: 1; min-width: 0; }
.thead { display: flex; align-items: baseline; gap: 10px; flex-wrap: wrap; }
.gname { font-weight: 600; font-size: 15px; }
.gkey { font-size: 12px; color: var(--muted); font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
.tmeta { display: flex; gap: 14px; flex-wrap: wrap; font-size: 13px; color: var(--muted); margin-top: 6px; }
.tmeta b { color: var(--fg); font-weight: 500; }

details.files { margin-top: 8px; }
details.files summary {
  cursor: pointer; font-size: 13px; color: var(--accent); user-select: none;
  display: inline-flex; align-items: center; gap: 4px; padding: 5px 10px 5px 0;
}
details.files summary:hover { text-decoration: underline; }
.flist { margin: 4px 0 0; padding: 0; list-style: none; }
.flist li {
  display: flex; gap: 12px; flex-wrap: wrap; align-items: baseline;
  padding: 7px 0; border-top: 1px dashed var(--line); font-size: 13px;
}
.flist li:first-child { border-top: 0; }
.flist a { color: var(--accent); text-decoration: none; word-break: break-all; min-width: 0; flex: 1; }
.flist a:hover { text-decoration: underline; }
.flist .fs { color: var(--muted); white-space: nowrap; }
.flist .ft { color: var(--muted); white-space: nowrap; }

.chips { display: flex; flex-wrap: wrap; gap: 8px; }
.chip { border: 1px solid var(--line); border-radius: 8px; padding: 6px 11px; font-size: 13px; }
.chip i { color: var(--muted); font-style: normal; font-family: ui-monospace, monospace; font-size: 11px; margin-left: 7px; }
.empty-ok { color: var(--muted); font-size: 13px; }
a { color: var(--accent); }

@media (max-width: 560px) {
  .wrap { padding: 16px 12px 40px; }
  .card { padding: 14px; border-radius: 10px; }
  .team { padding: 12px 6px; gap: 10px; }
  .rank { flex-basis: 36px; width: 36px; height: 36px; font-size: 16px; border-radius: 9px; }
  .gname { font-size: 14px; }
  .tmeta { gap: 10px; }
  .flist li { gap: 8px; }
}
`

const adminLoginHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>主办方管理 · 作品上传通道</title>
<style>` + pageStyle + `</style>
</head>
<body>
<div class="wrap">
  <div class="card">
    <h1>主办方管理</h1>
    <p class="sub">输入管理密钥，查看各队上传情况与路演顺序。</p>
    {{if .Error}}<div class="err">{{.Error}}</div>{{end}}
    <form method="post" action="/admin/login">
      <label for="key">管理密钥</label>
      <input id="key" name="key" type="password" autocomplete="off" autofocus
             placeholder="由部署方提供" spellcheck="false">
      <div style="margin-top:16px">
        <button type="submit" style="width:100%">进 入</button>
      </div>
    </form>
  </div>
  <p class="sub" style="text-align:center;margin-top:18px">参赛队请从首页用小组密钥进入。</p>
</div>
</body>
</html>`

const adminHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>主办方管理 · 作品上传通道</title>
<style>` + pageStyle + adminStyle + `</style>
</head>
<body>
<div class="wrap">
  <div class="top">
    <div>
      <h1>主办方管理</h1>
      <p class="sub" style="margin:0">路演顺序 = 首次上传时间，先交先排</p>
    </div>
    <a href="/admin/logout"><button class="ghost" type="button">退出</button></a>
  </div>

  <div class="card" style="display:flex;align-items:center;gap:14px;flex-wrap:wrap">
    <div class="meta-row" id="summary" style="flex:1;min-width:0">加载中…</div>
    <a href="/admin/archive" id="dlAll" style="display:none">
      <button type="button">一键下载全部作品</button>
    </a>
  </div>

  <h2 class="sec">路演顺序 <span id="uploadedCount"></span></h2>
  <div class="card" style="padding:0" id="rankList"></div>

  <h2 class="sec">尚未上传 <span id="pendingCount"></span></h2>
  <div class="card"><div class="chips" id="pendingList"></div></div>
</div>

<script>
(function () {
  var $sum = document.getElementById('summary');
  var $rank = document.getElementById('rankList');
  var $pending = document.getElementById('pendingList');
  var $upCount = document.getElementById('uploadedCount');
  var $pdCount = document.getElementById('pendingCount');
  var expanded = {}; // 刷新后保留各队文件清单的展开状态

  function fmt(n) {
    if (n < 1024) return n + ' B';
    if (n < 1048576) return (n / 1024).toFixed(1) + ' KB';
    if (n < 1073741824) return (n / 1048576).toFixed(1) + ' MB';
    return (n / 1073741824).toFixed(2) + ' GB';
  }

  function esc(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  function shortTime(t) { return t ? t.slice(5) : ''; } // 去掉年份，手机上更省地方

  function render(d) {
    var uploaded = [], pending = [];
    d.groups.forEach(function (g) { (g.uploaded ? uploaded : pending).push(g); });
    var totalFiles = 0, totalSize = 0;
    d.groups.forEach(function (g) { totalFiles += g.fileCount; totalSize += g.total; });

    var now = new Date();
    $sum.innerHTML =
      '<span>已交 <b>' + uploaded.length + '</b> / ' + d.groups.length + ' 队</span><span class="dot">·</span>' +
      '<span>文件 <b>' + totalFiles + '</b> 个</span><span class="dot">·</span>' +
      '<span>占用 <b>' + fmt(totalSize) + '</b></span><span class="dot">·</span>' +
      '<span class="updated">更新于 ' + now.toTimeString().slice(0, 8) + '，30 秒自动刷新</span>';

    document.getElementById('dlAll').style.display = uploaded.length ? '' : 'none';
    $upCount.textContent = uploaded.length ? '（' + uploaded.length + ' 队）' : '';
    $pdCount.textContent = pending.length ? '（' + pending.length + ' 队）' : '';

    var html = '';
    uploaded.forEach(function (g, i) {
      var files = g.files.map(function (f) {
        var url = '/admin/f?group=' + encodeURIComponent(g.id) + '&name=' + encodeURIComponent(f.name);
        return '<li><a href="' + url + '" download>' + esc(f.name) + '</a>' +
          '<span class="fs">' + fmt(f.size) + '</span><span class="ft">' + shortTime(f.time) + '</span></li>';
      }).join('');
      html += '<div class="team">' +
        '<div class="rank">' + (i + 1) + '</div>' +
        '<div class="tmain">' +
        '<div class="thead"><span class="gname">' + esc(g.name) + '</span><span class="gkey">' + esc(g.key) + '</span></div>' +
        '<div class="tmeta">' +
        '<span>首次上传 <b>' + shortTime(g.firstUpload) + '</b></span>' +
        (g.lastUpload !== g.firstUpload ? '<span>最近 <b>' + shortTime(g.lastUpload) + '</b></span>' : '') +
        '<span><b>' + g.fileCount + '</b> 个文件 · <b>' + fmt(g.total) + '</b></span>' +
        '</div>' +
        '<details class="files" data-g="' + esc(g.id) + '"' + (expanded[g.id] ? ' open' : '') + '>' +
        '<summary>文件清单</summary><ul class="flist">' + files + '</ul></details>' +
        '</div></div>';
    });
    $rank.innerHTML = html || '<div class="empty-ok" style="padding:18px;text-align:center">还没有队伍上传作品</div>';

    if (pending.length) {
      $pending.innerHTML = pending.map(function (g) {
        return '<span class="chip">' + esc(g.name) + '<i>' + esc(g.key) + '</i></span>';
      }).join('');
    } else {
      $pending.innerHTML = '<span class="empty-ok">全部队伍都已提交 ✓</span>';
    }
  }

  function load() {
    fetch('/api/admin/groups', { credentials: 'same-origin' }).then(function (r) {
      if (r.status === 401) { location.reload(); return; }
      return r.json();
    }).then(function (d) {
      if (!d || !d.ok) return;
      // 记下当前展开了哪些队的文件清单，重渲染后恢复
      document.querySelectorAll('details.files').forEach(function (el) {
        expanded[el.getAttribute('data-g')] = el.open;
      });
      render(d);
    }).catch(function () {});
  }

  load();
  setInterval(load, 30000);
})();
</script>
</body>
</html>`

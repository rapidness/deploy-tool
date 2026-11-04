// logs.js 日志页面前端逻辑（原生 JS，无外部依赖）

var BASE = window.__BP__ || '';
var allLogs = [];
var currentNode = ''; // 当前查看的节点，空表示本机

// 退出登录
document.getElementById('btn-logout').addEventListener('click', function() {
  fetch(BASE + '/logout', { method: 'POST' }).then(function() { window.location.href = BASE + '/login'; });
});

document.getElementById('btn-refresh-logs').addEventListener('click', loadLogs);

document.getElementById('log-search').addEventListener('input', renderLogs);

document.getElementById('node-select').addEventListener('change', function() {
  currentNode = this.value;
  allLogs = [];
  renderLogs();
  loadLogs();
});

loadNodes().then(loadLogs);
setInterval(loadLogs, 15000);

// 加载节点列表并渲染节点下拉框；单节点时隐藏
function loadNodes() {
  return fetch(BASE + '/api/nodes')
    .then(function(r) { return r.json(); })
    .then(function(list) {
      var nodes = list || [];
      var wrap = document.getElementById('node-select-wrap');
      var sel = document.getElementById('node-select');
      if (nodes.length <= 1) {
        wrap.style.display = 'none';
        currentNode = nodes.length === 1 ? nodes[0].name : '';
        return;
      }
      wrap.style.display = '';
      sel.innerHTML = '';
      nodes.forEach(function(n) {
        var opt = document.createElement('option');
        opt.value = n.name;
        opt.textContent = n.name + (n.is_self ? '（本机）' : '');
        sel.appendChild(opt);
      });
      // 默认查看本机日志
      var self = nodes.filter(function(n) { return n.is_self; })[0];
      if (self) sel.value = self.name;
      currentNode = sel.value;
    })
    .catch(function() {
      // 节点加载失败时退回本机日志
      document.getElementById('node-select-wrap').style.display = 'none';
      currentNode = '';
    });
}

// logsURL 构造日志接口地址（action 为空表示列表）。
function logsURL(action, name) {
  var params = [];
  if (currentNode) params.push('node=' + encodeURIComponent(currentNode));
  if (name) params.push('name=' + encodeURIComponent(name));
  return BASE + '/api/logs' + (action ? '/' + action : '') + (params.length ? '?' + params.join('&') : '');
}

function loadLogs() {
  fetch(logsURL('', ''))
    .then(function(r) { return r.json(); })
    .then(function(list) {
      var div = document.getElementById('logs');
      if (list && list.error) {
        div.innerHTML = '<p class="error">' + escapeHtml(list.error) + '</p>';
        return;
      }
      allLogs = list || [];
      renderLogs();
    })
    .catch(function(err) {
      document.getElementById('logs').innerHTML = '<p class="error">加载失败: ' + err.message + '</p>';
    });
}

function renderLogs() {
  var div = document.getElementById('logs');
  var kw = (document.getElementById('log-search').value || '').trim().toLowerCase();
  var list = allLogs;
  if (kw) {
    list = list.filter(function(f) { return (f.name || '').toLowerCase().indexOf(kw) >= 0; });
  }
  if (list.length === 0) {
    div.innerHTML = '<p class="muted">' + (kw ? '无匹配文件' : '暂无日志文件') + '</p>';
    return;
  }
  var html = '<table><thead><tr><th>文件名</th><th>大小</th><th>最后更新</th><th>操作</th></tr></thead><tbody>';
  list.forEach(function(f) {
    var t = f.mod_time ? new Date(f.mod_time).toLocaleString('zh-CN') : '';
    html += '<tr>'
      + '<td>' + escapeHtml(f.name) + '</td>'
      + '<td>' + formatSize(f.size) + '</td>'
      + '<td>' + t + '</td>'
      + '<td>'
      + '<button class="btn-sm" onclick="viewLog(\'' + escapeAttr(f.name) + '\')">查看</button> '
      + '<button class="btn-sm" onclick="viewLogNewTab(\'' + escapeAttr(f.name) + '\')">新窗口查看</button> '
      + '<button class="btn-sm" onclick="downloadLog(\'' + escapeAttr(f.name) + '\')">下载</button>'
      + '</td>'
      + '</tr>';
  });
  html += '</tbody></table>';
  div.innerHTML = html;
}

function formatSize(bytes) {
  if (bytes == null) return '';
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  if (bytes < 1024 * 1024 * 1024) return (bytes / 1024 / 1024).toFixed(1) + ' MB';
  return (bytes / 1024 / 1024 / 1024).toFixed(2) + ' GB';
}

function downloadLog(name) {
  window.location.href = logsURL('download', name);
}

function viewLogNewTab(name) {
  window.open(logsURL('view', name), '_blank');
}

var currentLogName = '';

function viewLog(name) {
  currentLogName = name;
  var viewer = document.getElementById('log-viewer');
  var content = document.getElementById('log-viewer-content');
  var title = document.getElementById('log-viewer-title');
  title.textContent = '日志查看: ' + name + (currentNode ? '（' + currentNode + '）' : '');
  content.textContent = '加载中...';
  viewer.style.display = '';
  viewer.classList.remove('maximized');
  document.getElementById('btn-log-maximize').textContent = '最大化';
  fetch(logsURL('view', name))
    .then(function(r) {
      if (!r.ok) { throw new Error('HTTP ' + r.status); }
      return r.text();
    })
    .then(function(text) {
      content.textContent = text;
    })
    .catch(function(err) {
      content.textContent = '加载失败: ' + err.message;
    });
}

document.getElementById('btn-close-log-viewer').addEventListener('click', function() {
  document.getElementById('log-viewer').style.display = 'none';
  document.getElementById('log-viewer-content').textContent = '';
  currentLogName = '';
});

document.getElementById('btn-log-maximize').addEventListener('click', function() {
  var viewer = document.getElementById('log-viewer');
  var btn = this;
  if (viewer.classList.toggle('maximized')) {
    btn.textContent = '还原';
  } else {
    btn.textContent = '最大化';
  }
});

document.getElementById('btn-log-newtab').addEventListener('click', function() {
  if (!currentLogName) return;
  window.open(logsURL('view', currentLogName), '_blank');
});

// 工具函数
function escapeHtml(s) {
  if (s == null) return '';
  return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}
function escapeAttr(s) {
  return String(s).replace(/'/g, "\\'").replace(/"/g, '&quot;');
}

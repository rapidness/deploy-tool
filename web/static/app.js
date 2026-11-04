// app.js 服务更新工具前端逻辑（原生 JS，无外部依赖）

var BASE = window.__BP__ || '';
var currentService = '';
var currentTargets = []; // [{path, desc}]
var targetUploads = {};  // path -> {uploadID, files}

// 退出登录
document.getElementById('btn-logout').addEventListener('click', function() {
  fetch(BASE + '/logout', { method: 'POST' }).then(function() { window.location.href = BASE + '/login'; });
});

// 加载服务列表并填充下拉框与状态表格
function loadServices() {
  fetch(BASE + '/api/services')
    .then(function(r) { return r.json(); })
    .then(function(services) {
      var html = '<table><thead><tr><th>服务名称</th><th>显示名</th><th>状态</th><th>操作</th></tr></thead><tbody>';
      services.forEach(function(s) {
        var statusClass = 'status-' + s.status;
        var statusText = { running: '运行中', stopped: '已停止', unknown: '未知' }[s.status] || s.status;
        html += '<tr>'
          + '<td>' + escapeHtml(s.name) + '</td>'
          + '<td>' + escapeHtml(s.display_name || '') + '</td>'
          + '<td class="' + statusClass + '">' + statusText + '</td>'
          + '<td>'
          + '<button class="btn-sm" onclick="controlService(\'' + escapeAttr(s.name) + '\', \'start\')">启动</button> '
          + '<button class="btn-sm" onclick="controlService(\'' + escapeAttr(s.name) + '\', \'stop\')">停止</button> '
          + '<button class="btn-sm" onclick="controlService(\'' + escapeAttr(s.name) + '\', \'restart\')">重启</button>'
          + '</td>'
          + '</tr>';
      });
      html += '</tbody></table>';
      document.getElementById('services').innerHTML = html;

      var select = document.getElementById('service-select');
      var current = select.value;
      select.innerHTML = '<option value="">请选择服务</option>';
      services.forEach(function(s) {
        var opt = document.createElement('option');
        opt.value = s.name;
        opt.textContent = s.display_name ? s.display_name + ' (' + s.name + ')' : s.name;
        select.appendChild(opt);
      });
      if (current) select.value = current;
      // 只有一个服务时自动选中
      if (!select.value && services.length === 1) {
        select.value = services[0].name;
        select.dispatchEvent(new Event('change'));
      }
    })
    .catch(function(err) {
      document.getElementById('services').innerHTML = '<p class="error">加载失败: ' + err.message + '</p>';
    });
}

// 控制服务启停
function controlService(name, action) {
  var form = new FormData();
  form.append('service', name);
  form.append('action', action);
  fetch(BASE + '/api/service/control', { method: 'POST', body: form })
    .then(function(r) { return r.json(); })
    .then(function(res) {
      if (res.error) { alert('操作失败: ' + res.error); }
      loadServices();
    })
    .catch(function(err) { alert('请求失败: ' + err.message); });
}

// === 服务选择 → 加载目标路径 ===

document.getElementById('service-select').addEventListener('change', function() {
  currentService = this.value;
  targetUploads = {};
  if (currentService) {
    document.getElementById('targets-section').style.display = '';
    loadTargets();
  } else {
    document.getElementById('targets-section').style.display = 'none';
  }
});

function loadTargets() {
  fetch(BASE + '/api/targets?service=' + encodeURIComponent(currentService))
    .then(function(r) { return r.json(); })
    .then(function(list) {
      currentTargets = list || [];
      renderTargets();
    })
    .catch(function(err) {
      document.getElementById('targets-status').innerHTML = '<p class="error">加载目标路径失败: ' + err.message + '</p>';
    });
}

function renderTargets() {
  var container = document.getElementById('targets-list');
  if (currentTargets.length === 0) {
    container.innerHTML = '<p class="muted">暂无目标路径，点击"添加路径"创建</p>';
    return;
  }
  var html = '';
  currentTargets.forEach(function(t, i) {
    var path = t.path || '';
    var desc = t.desc || '';
    var uploadInfo = targetUploads[path];
    var uploadHtml = uploadInfo
      ? '<p class="step-success">已上传: ' + escapeHtml(uploadInfo.files.map(function(f){return f.name;}).join(', ')) + '</p>'
      : '<p class="muted">未上传</p>';
    html += '<div class="target-row" data-index="' + i + '">'
      + '<div class="target-left">'
      + '<input type="text" class="target-path-input" data-index="' + i + '" value="' + escapeAttr(path) + '" placeholder="相对根目录的路径，如 bin">'
      + '<input type="text" class="target-desc-input" data-index="' + i + '" value="' + escapeAttr(desc) + '" placeholder="描述（可选）">'
      + '<div class="dir-deploy">'
      + '<input type="text" class="target-dir-input" data-index="' + i + '" value="' + escapeAttr(t.source_dir || '') + '" placeholder="服务器目录（绝对路径），读取目录内文件部署">'
      + '<button type="button" class="btn-sm" data-index="' + i + '" onclick="deployFromDir(' + i + ')">执行更新</button>'
      + '<label class="dir-delete-label"><input type="checkbox" class="delete-source-input" data-index="' + i + '">是否删除原文件</label>'
      + '</div>'
      + '<button type="button" class="btn-sm btn-danger" data-index="' + i + '" onclick="removeTarget(' + i + ')">删除</button>'
      + '</div>'
      + '<div class="target-right">'
      + '<div class="drop-zone" data-path="' + escapeAttr(path) + '">'
      + '<p>拖拽文件到此处，或</p>'
      + '<input type="file" multiple accept=".zip,.dll,.pdb,.json,.exe,.config,.xml,.txt,.ini,.log" style="display:none">'
      + '<button type="button" class="btn-sm" onclick="chooseFile(this)">选择文件</button>'
      + '<p class="muted">支持多文件</p>'
      + '</div>'
      + '<div class="upload-info" data-path="' + escapeAttr(path) + '">' + uploadHtml + '</div>'
      + '<div class="dir-deploy-info" data-index="' + i + '"></div>'
      + '</div>'
      + '</div>';
  });
  container.innerHTML = html;
  bindDropZones();
}

function chooseFile(btn) {
  var input = btn.parentElement.querySelector('input[type=file]');
  input.onchange = function() {
    if (input.files.length > 0) {
      var zone = btn.closest('.drop-zone') || input.closest('.drop-zone');
      uploadFiles(zone, input.files);
    }
  };
  input.click();
}

function removeTarget(index) {
  var path = currentTargets[index] ? currentTargets[index].path : '';
  currentTargets.splice(index, 1);
  if (path) delete targetUploads[path];
  renderTargets();
}

document.getElementById('btn-add-target').addEventListener('click', function() {
  currentTargets.push({ path: '', desc: '', source_dir: '' });
  renderTargets();
});

document.getElementById('btn-save-targets').addEventListener('click', function() {
  // 从输入框收集值
  var pathInputs = document.querySelectorAll('.target-path-input');
  var descInputs = document.querySelectorAll('.target-desc-input');
  var dirInputs = document.querySelectorAll('.target-dir-input');
  var targets = [];
  pathInputs.forEach(function(inp, i) {
    var path = inp.value.trim();
    var desc = descInputs[i] ? descInputs[i].value.trim() : '';
    var sourceDir = dirInputs[i] ? dirInputs[i].value.trim() : '';
    if (path) { targets.push({ path: path, desc: desc, source_dir: sourceDir }); }
  });
  fetch(BASE + '/api/targets', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ service: currentService, targets: targets })
  })
    .then(function(r) { return r.json(); })
    .then(function(res) {
      var statusDiv = document.getElementById('targets-status');
      if (res.error) {
        statusDiv.innerHTML = '<p class="error">保存失败: ' + escapeHtml(res.error) + '</p>';
        return;
      }
      statusDiv.innerHTML = '<p class="step-success">已保存</p>';
      currentTargets = targets;
      renderTargets();
    })
    .catch(function(err) {
      document.getElementById('targets-status').innerHTML = '<p class="error">请求失败: ' + err.message + '</p>';
    });
});

// === 文件上传（拖拽 + 选择，多文件）===

function bindDropZones() {
  var zones = document.querySelectorAll('.drop-zone');
  zones.forEach(function(zone) {
    zone.ondragover = function(e) {
      e.preventDefault();
      zone.classList.add('drag-over');
    };
    zone.ondragleave = function() {
      zone.classList.remove('drag-over');
    };
    zone.ondrop = function(e) {
      e.preventDefault();
      zone.classList.remove('drag-over');
      if (e.dataTransfer.files.length > 0) {
        uploadFiles(zone, e.dataTransfer.files);
      }
    };
  });
}

function uploadFiles(zone, files) {
  var path = zone.getAttribute('data-path');
  if (!path) { alert('请先填写目标路径并保存'); return; }
  var infoDiv = document.querySelector('.upload-info[data-path="' + cssSelector(path) + '"]');
  if (infoDiv) infoDiv.innerHTML = '<p class="muted">正在上传 ' + files.length + ' 个文件...</p>';

  var form = new FormData();
  for (var i = 0; i < files.length; i++) {
    form.append('files', files[i], files[i].name);
  }
  fetch(BASE + '/api/upload', { method: 'POST', body: form })
    .then(function(r) { return r.json(); })
    .then(function(res) {
      if (res.error) {
        if (infoDiv) infoDiv.innerHTML = '<p class="error">上传失败: ' + escapeHtml(res.error) + '</p>';
        return;
      }
      targetUploads[path] = { uploadID: res.upload_id, files: res.files || [] };
      if (infoDiv) infoDiv.innerHTML = '<p class="step-success">已上传: ' + escapeHtml((res.files || []).map(function(f){return f.name;}).join(', ')) + '</p>'
            + '<button type="button" class="btn-sm" onclick="deployTarget(\'' + escapeAttr(path) + '\', false)">执行更新</button>'
            + ' <button type="button" class="btn-sm" onclick="deployTarget(\'' + escapeAttr(path) + '\', true)">替换文件</button>';
    })
    .catch(function(err) {
      if (infoDiv) infoDiv.innerHTML = '<p class="error">上传失败: ' + err.message + '</p>';
    });
}

function deployTarget(path, filesOnly) {
  var upload = targetUploads[path];
  if (!upload || !upload.uploadID) { alert('请先上传文件'); return; }
  if (!currentService) { alert('请选择服务'); return; }
  var chosen = selectedNodes();
  if (nodes.length > 0 && chosen.length === 0) { alert('请至少选择一个节点'); return; }

  var infoDiv = document.querySelector('.upload-info[data-path="' + cssSelector(path) + '"]');
  if (infoDiv) infoDiv.innerHTML = '<p class="muted">正在提交' + (filesOnly ? '替换' : '更新') + '...</p>';

  fetch(BASE + '/api/cluster/deploy', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      upload_id: upload.uploadID,
      service: currentService,
      target_dir: path,
      files_only: !!filesOnly,
      nodes: chosen
    })
  })
    .then(function(r) { return r.json(); })
    .then(function(res) {
      if (res.error) {
        if (infoDiv) infoDiv.innerHTML = '<p class="error">提交失败: ' + escapeHtml(res.error) + '</p>';
        return;
      }
      pollBatch(res.batch_id, infoDiv, path);
    })
    .catch(function(err) {
      if (infoDiv) infoDiv.innerHTML = '<p class="error">请求失败: ' + err.message + '</p>';
    });
}

// 从服务器本机目录读取文件并执行更新（无需上传）
function deployFromDir(index) {
  if (!currentService) { alert('请选择服务'); return; }
  var pathInput = document.querySelector('.target-path-input[data-index="' + index + '"]');
  var dirInput = document.querySelector('.target-dir-input[data-index="' + index + '"]');
  var deleteInput = document.querySelector('.delete-source-input[data-index="' + index + '"]');
  var path = pathInput ? pathInput.value.trim() : '';
  var dir = dirInput ? dirInput.value.trim() : '';
  var deleteSource = deleteInput ? deleteInput.checked : false;
  if (!path) { alert('请先填写目标路径'); return; }
  if (!dir) { alert('请填写服务器目录'); return; }
  var chosen = selectedNodes();
  if (nodes.length > 0 && chosen.length === 0) { alert('请至少选择一个节点'); return; }

  var infoDiv = document.querySelector('.dir-deploy-info[data-index="' + index + '"]');
  if (infoDiv) infoDiv.innerHTML = '<p class="muted">正在提交更新...</p>';

  fetch(BASE + '/api/cluster/deploy', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      source_dir: dir,
      service: currentService,
      target_dir: path,
      delete_source: deleteSource,
      nodes: chosen
    })
  })
    .then(function(r) { return r.json(); })
    .then(function(res) {
      if (res.error) {
        if (infoDiv) infoDiv.innerHTML = '<p class="error">提交失败: ' + escapeHtml(res.error) + '</p>';
        return;
      }
      pollBatch(res.batch_id, infoDiv, null);
    })
    .catch(function(err) {
      if (infoDiv) infoDiv.innerHTML = '<p class="error">请求失败: ' + err.message + '</p>';
    });
}

// 轮询批量部署状态，完成后展示各节点结果
function pollBatch(batchID, infoDiv, path) {
  var timer = setInterval(function() {
    fetch(BASE + '/api/cluster/status?id=' + encodeURIComponent(batchID))
      .then(function(r) { return r.json(); })
      .then(function(batch) {
        if (batch.error) {
          clearInterval(timer);
          if (infoDiv) infoDiv.innerHTML = '<p class="error">查询状态失败: ' + escapeHtml(batch.error) + '</p>';
          return;
        }
        if (infoDiv) infoDiv.innerHTML = renderBatchHtml(batch);
        if (batch.status === 'success' || batch.status === 'partial' || batch.status === 'failed') {
          clearInterval(timer);
          if (path) delete targetUploads[path];
          loadServices();
          loadHistory();
        }
      })
      .catch(function(err) {
        clearInterval(timer);
        if (infoDiv) infoDiv.innerHTML = '<p class="error">查询状态失败: ' + err.message + '</p>';
      });
  }, 1000);
}

function renderBatchHtml(batch) {
  var stateText = { pending: '等待执行', running: '执行中', success: '全部成功', partial: '部分失败', failed: '全部失败' }[batch.status] || batch.status;
  var stateCls = batch.status === 'success' ? 'step-success'
    : (batch.status === 'running' || batch.status === 'pending' ? 'muted' : 'step-failed');
  var html = '<p class="' + stateCls + '">批量结果: ' + escapeHtml(stateText) + '</p>';
  (batch.nodes || []).forEach(function(n) {
    var nodeCls = n.status === 'success' ? 'step-success' : (n.status === 'failed' ? 'step-failed' : 'muted');
    html += '<h4>' + escapeHtml(n.name) + (n.is_self ? '（本机）' : '') + ' <span class="' + nodeCls + '">' + escapeHtml(n.status) + '</span></h4>';
    var steps = n.steps || [];
    if (steps.length === 0) return;
    html += '<table><thead><tr><th>步骤</th><th>状态</th><th>说明</th><th>耗时(ms)</th></tr></thead><tbody>';
    steps.forEach(function(step) {
      var cls = 'step-' + step.status;
      html += '<tr><td>' + escapeHtml(step.name) + '</td><td class="' + cls + '">' + escapeHtml(step.status) + '</td><td>' + escapeHtml(step.message || '') + '</td><td>' + (step.duration || 0) + '</td></tr>';
    });
    html += '</tbody></table>';
  });
  return html;
}

// === 目标节点 ===
var nodes = [];

function loadNodes() {
  fetch(BASE + '/api/nodes')
    .then(function(r) { return r.json(); })
    .then(function(list) {
      nodes = list || [];
      renderNodes();
    })
    .catch(function(err) {
      document.getElementById('nodes-section').style.display = '';
      document.getElementById('nodes-list').innerHTML = '<p class="error">加载节点失败: ' + escapeHtml(err.message) + '</p>';
    });
}

function renderNodes() {
  // 单节点时无需展示选择项
  document.getElementById('nodes-section').style.display = nodes.length > 1 ? '' : 'none';
  if (nodes.length === 0) return;
  var html = '';
  nodes.forEach(function(n) {
    html += '<label class="node-item"><input type="checkbox" class="node-checkbox" value="' + escapeAttr(n.name) + '" checked> '
      + escapeHtml(n.name) + (n.is_self ? ' <span class="muted">(本机)</span>' : '') + '</label>';
  });
  document.getElementById('nodes-list').innerHTML = html;
}

function selectedNodes() {
  var names = [];
  document.querySelectorAll('.node-checkbox:checked').forEach(function(box) {
    names.push(box.value);
  });
  return names;
}

// 加载操作历史
function loadHistory() {
  fetch(BASE + '/api/history')
    .then(function(r) { return r.json(); })
    .then(function(list) {
      if (!list || list.length === 0) {
        document.getElementById('history').innerHTML = '<p class="muted">暂无历史记录</p>';
        return;
      }
      var html = '<table><thead><tr><th>时间</th><th>操作人</th><th>服务</th><th>节点</th><th>文件</th><th>结果</th></tr></thead><tbody>';
      list.forEach(function(h) {
        var t = h.time || h.Time || '';
        t = t ? new Date(t).toLocaleString('zh-CN') : '';
        var state = h.final_state || h.FinalState || '';
        var cls = state === 'success' ? 'step-success' : 'step-failed';
        html += '<tr><td>' + t + '</td><td>' + escapeHtml(h.operator || h.Operator || '') + '</td><td>' + escapeHtml(h.service || h.Service || '') + '</td><td>' + escapeHtml(h.node || h.Node || '') + '</td><td>' + escapeHtml(h.file_name || h.FileName || '') + '</td><td class="' + cls + '">' + state + '</td></tr>';
      });
      html += '</tbody></table>';
      document.getElementById('history').innerHTML = html;
    })
    .catch(function(err) {
      document.getElementById('history').innerHTML = '<p class="error">加载失败: ' + err.message + '</p>';
    });
}

// 工具函数
function escapeHtml(s) {
  if (s == null) return '';
  return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}
function escapeAttr(s) {
  return String(s).replace(/'/g, "\\'").replace(/"/g, '&quot;');
}
function cssSelector(s) {
  return String(s).replace(/"/g, '\\"');
}

// 初始化
loadServices();
loadNodes();
loadHistory();
setInterval(loadServices, 5000);

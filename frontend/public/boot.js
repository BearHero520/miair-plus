// Classic script: report module download/parse failures even when Vue cannot start.
(function () {
  var done = false;
  var failed = false;
  var phase = '下载入口脚本';
  var entries = [];
  var diagnosed = {};
  function diagnose(address) {
    if (diagnosed[address]) return;
    diagnosed[address] = true;
    try {
      var url = new URL(address, document.baseURI);
      if (url.origin !== location.origin) return;
      var request = new XMLHttpRequest();
      request.open('GET', url.href, true);
      request.timeout = 10000;
      request.withCredentials = true;
      request.onload = function () {
        fail('资源检查：' + url.pathname + ' → HTTP ' + request.status + ' / ' + (request.getResponseHeader('Content-Type') || '未知类型'));
      };
      request.onerror = function () { fail('资源检查：' + url.pathname + ' → 网络或跨域访问失败'); };
      request.ontimeout = function () { fail('资源检查：' + url.pathname + ' → 请求超时'); };
      request.send();
    } catch (_) { /* Diagnostics must not interrupt the page. */ }
  }
  function clean(value) {
    return String(value || '').replace(/https?:\/\/[^\s"'<>]+/g, function (value) {
      try { return new URL(value).pathname; } catch (_) { return '[地址]'; }
    }).replace(/\?[^\s"'<>]*/g, '?[已省略参数]').slice(0, 600);
  }
  function render() {
    if (done) return;
    var message = document.getElementById('boot-message');
    if (!message || !failed) return;
    message.textContent = '页面未能完成加载。请查看下方错误详情，或重新加载。';
    document.getElementById('boot-reload').hidden = false;
    document.getElementById('boot-details').hidden = false;
    document.getElementById('boot-error').textContent =
      '阶段：' + phase + '\n页面：' + location.pathname + '\n' + entries.join('\n');
  }
  function fail(error) {
    if (done) return;
    failed = true;
    var detail = error && (error.message || error.type) || error || '未知错误';
    if (entries.length < 8) entries.push(clean(detail));
    render();
  }
  var timer = setTimeout(function () { fail('启动等待超过 35 秒'); }, 35000);
  window.miairBoot = {
    phase: function (value) { phase = value; },
    fail: fail,
    ready: function () {
      if (!document.getElementById('app').textContent.trim()) {
        fail('界面没有渲染内容');
        return;
      }
      done = true;
      clearTimeout(timer);
      document.getElementById('boot-status').remove();
    }
  };
  window.addEventListener('error', function (event) {
    var target = event.target;
    if (target && target !== window) {
      // An icon/image failure must not be mistaken for an app startup failure.
      if (target.tagName !== 'SCRIPT' && !(target.tagName === 'LINK' && target.rel === 'stylesheet')) return;
      fail('资源加载失败：' + (target.src || target.href));
      diagnose(target.src || target.href);
    } else {
      fail((event.message || '脚本执行失败') + (event.filename ? ' @ ' + clean(event.filename) + ':' + event.lineno : ''));
    }
  }, true);
  window.addEventListener('unhandledrejection', function (event) { fail(event.reason || '异步启动失败'); });
  document.addEventListener('DOMContentLoaded', render);
})();

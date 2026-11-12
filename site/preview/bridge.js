/* Replaces XHR only inside generated demo frames. Native application JS is unchanged. */
(function () {
  'use strict';
  var host;
  try { host = window.parent.inkpikaDemo; } catch (ignore) {}
  var screen = document.body.getAttribute('data-demo-screen');
  if (!host) {
    var notice = document.createElement('p'), link = document.createElement('a');
    notice.textContent = I18n.t('This is a sample page from the full preview.'); link.href = '../index.html'; link.textContent = I18n.t('Open the full preview');
    notice.appendChild(link); document.body.insertBefore(notice, document.body.firstChild);
  }
  function Request() { this.upload = {}; this.headers = {}; this.readyState = 0; this.status = 0; this.responseText = ''; this.aborted = false; }
  Request.prototype.open = function (method, url) { this.method = method; this.url = url; this.readyState = 1; };
  Request.prototype.getResponseHeader = function (name) { return name.toLowerCase() === 'content-language' && host ? host.language() : null; };
  Request.prototype.setRequestHeader = function (name, value) { this.headers[name.toLowerCase()] = value; };
  Request.prototype.abort = function () { this.aborted = true; clearTimeout(this.timer); if (this.onabort) { this.onabort(); } };
  Request.prototype.send = function (body) {
    var xhr = this, state = host && host.state;
    function finish(data) {
      if (xhr.aborted || (host && host.state !== state)) { return; }
      var response = host ? host.api(screen, xhr.method, xhr.url, data, xhr.headers) : {status: 503, data: {error: I18n.t('Open this page from the full feature preview.')}};
      xhr.status = response.status; xhr.responseText = JSON.stringify(response.data); xhr.readyState = 4;
      if (xhr.url === '/api/export' && response.status === 200) { document.getElementById('download-link').download = response.data.name; }
      if (xhr.onreadystatechange) { xhr.onreadystatechange(); }
      if (xhr.onload) { xhr.onload(); }
    }
    if (body && typeof body.size === 'number') {
      var progress = 0;
      function chunk() {
        if (xhr.aborted) { return; }
        progress += 25;
        if (xhr.upload.onprogress) { xhr.upload.onprogress({lengthComputable: true, loaded: Math.ceil(body.size * progress / 100), total: body.size}); }
        if (progress < 100) { xhr.timer = setTimeout(chunk, 250); return; }
        var reader = new FileReader();
        reader.onload = function () { finish({bytes: new Uint8Array(reader.result)}); };
        reader.onerror = function () { if (xhr.onerror) { xhr.onerror(); } }; reader.readAsArrayBuffer(body);
      }
      xhr.timer = setTimeout(chunk, 250);
    } else {
      xhr.timer = setTimeout(function () { finish(body ? JSON.parse(body) : null); }, xhr.url === '/api/connection-check' && xhr.method === 'POST' ? 600 : 60);
    }
  };
  window.XMLHttpRequest = Request;
  window.inkpikaSampleFiles = function () {
    var input = document.getElementById('files');
    if (!input || !host || input.disabled) { return false; }
    try {
      var transfer = new DataTransfer();
      host.books.forEach(function (book) { transfer.items.add(new File([book.bytes], book.name, {type: 'application/octet-stream', lastModified: 1794096000000})); });
      input.files = transfer.files; input.onchange(); return true;
    } catch (error) { return false; }
  };
}());

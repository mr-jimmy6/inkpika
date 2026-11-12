(function () {
  'use strict';
  var token = '', maxBytes = 0, queue = [], busy = false, active = null, cancelled = false, formats = [], selected = {}, pageBooks = [], offset = 0, generation = 0;
  function el(id) { return document.getElementById(id); }
  function size(n) { return n >= 1048576 ? (n / 1048576).toFixed(1) + ' MB' : Math.ceil(n / 1024) + ' KB'; }
  function message(value) { el('message').textContent = value; }
  function api(method, path, data, done) {
    var xhr = new XMLHttpRequest(); xhr.open(method, path, true); xhr.timeout = 15000;
    xhr.setRequestHeader('X-Transfer-Token', token);
    if (data !== null) { xhr.setRequestHeader('Content-Type', 'application/json'); }
    xhr.onload = function () {
      var responseLanguage = xhr.getResponseHeader('Content-Language');
      if (responseLanguage && responseLanguage !== I18n.language()) { I18n.setLanguage(responseLanguage); } var result; try { result = JSON.parse(xhr.responseText); } catch (ignore) { done(I18n.t("Cannot read the Kindle response")); return; } done(xhr.status === 200 ? null : result.error || I18n.t("Request failed"), result); };
    xhr.onerror = function () { done(I18n.t("Cannot connect to Kindle; check Wi-Fi and the transfer service.")); }; xhr.ontimeout = function () { done(I18n.t("Connection timed out; keep Kindle awake.")); };
    xhr.send(data === null ? null : JSON.stringify(data));
  }
  function connect(event) {
    if (event) { event.preventDefault(); }
    var code = el('code').value;
    if (!/^\d{6}$/.test(code)) { message(I18n.t("Enter the six-digit connection code.")); return; }
    el('connect').disabled = true; message(I18n.t("Connecting…"));
    api('POST', '/api/pair', {code: code}, function (err, result) {
      el('connect').disabled = false;
      if (err) { message(err); return; }
      token = result.token;
      api('GET', '/api/info', null, function (error, info) {
        if (error) { message(error); return; }
        maxBytes = info.max_upload_mb * 1048576;
        formats = info.ebook_formats || [];
        el('formats').textContent = I18n.t("Allowed formats: ") + formats.join(I18n.language() === 'zh' ? '、' : ', ').toUpperCase() + I18n.t(". Up to 50 books per batch.");
        el('files').setAttribute('accept', formats.map(function (f) { return '.' + f; }).join(','));
        el('pair').hidden = true; el('upload').hidden = false;
        el('space').textContent = I18n.t("Free ") + size(info.free_bytes) + I18n.t(" · Per-book limit ") + info.max_upload_mb + ' MB';
        message(I18n.t("Connected to Kindle."));
      });
    });
  }
  function selectFiles() {
    if (busy) { return; }
    queue = []; el('queue').innerHTML = '';
    var files = el('files').files;
    if (files.length > 50) { message(I18n.t("Select at most 50 books at a time.")); el('send').disabled = true; return; }
    for (var i = 0; i < files.length; i++) {
      var file = files[i], row = document.createElement('div'), name = document.createElement('p'), status = document.createElement('p');
      row.className = 'book'; name.className = 'name'; name.textContent = file.name; status.className = 'note';
      var extension = file.name.lastIndexOf('.') >= 0 ? file.name.split('.').pop().toLowerCase() : '';
      var error = formats.indexOf(extension) < 0 ? I18n.t("Format is not allowed") : file.size > maxBytes ? I18n.t("Exceeds the per-book size limit") : !file.size ? I18n.t("File is empty") : '';
      status.textContent = error || I18n.t("Waiting to upload · ") + size(file.size); row.appendChild(name); row.appendChild(status); el('queue').appendChild(row);
      queue.push({file: file, status: status, skip: !!error});
    }
    el('send').disabled = !queue.some(function (item) { return !item.skip; });
  }
  function finish() { busy = false; active = null; el('files').disabled = false; el('files').value = ''; el('cancel').hidden = true; message(cancelled ? I18n.t("Upload cancelled; completed books preserved.") : I18n.t("Upload batch finished; see results below.")); }
  function next(index) {
    if (cancelled) { for (var j = index; j < queue.length; j++) { if (!queue[j].skip) { queue[j].status.textContent = I18n.t("Cancelled"); } } finish(); return; }
    if (index >= queue.length) { finish(); return; }
    var item = queue[index]; if (item.skip) { next(index + 1); return; }
    var xhr = new XMLHttpRequest(); active = xhr; xhr.open('POST', '/api/upload', true); xhr.timeout = 20 * 60 * 1000;
    xhr.setRequestHeader('X-Transfer-Token', token); xhr.setRequestHeader('X-File-Name', encodeURIComponent(item.file.name)); xhr.setRequestHeader('Content-Type', 'application/octet-stream');
    item.status.textContent = I18n.t("Uploading · 0%");
    xhr.upload.onprogress = function (event) { if (event.lengthComputable) { var percent = Math.floor(event.loaded / event.total * 100); item.status.textContent = percent < 100 ? I18n.t("Uploading · ") + percent + '%' : I18n.t("Transferred; waiting for Kindle to save…"); } };
    xhr.onload = function () {
      var responseLanguage = xhr.getResponseHeader('Content-Language');
      if (responseLanguage && responseLanguage !== I18n.language()) { I18n.setLanguage(responseLanguage); }
      var result; try { result = JSON.parse(xhr.responseText); } catch (ignore) { result = {error: I18n.t("Cannot read the save result; check Kindle before retrying")}; }
      item.status.textContent = xhr.status === 201 ? I18n.t("Saved to Kindle") + (result.name !== item.file.name ? I18n.t(" · Saved as: ") + result.name : '') : I18n.t("Failed: ") + (result.error || I18n.t("Upload failed")); next(index + 1);
    };
    xhr.onerror = function () { item.status.textContent = I18n.t("Connection interrupted; check Kindle, then select files again"); next(index + 1); };
    xhr.ontimeout = function () { item.status.textContent = I18n.t("Upload timed out; select files again"); next(index + 1); };
    xhr.onabort = function () { item.status.textContent = I18n.t("Cancelled"); next(index + 1); }; xhr.send(item.file);
  }
  el('pair').onsubmit = connect; el('files').onchange = selectFiles;
  el('send').onclick = function () { if (busy || !queue.length) { return; } busy = true; cancelled = false; el('send').disabled = true; el('files').disabled = true; el('cancel').hidden = false; message(I18n.t("Transferring books; keep Kindle awake.")); next(0); };
  el('cancel').onclick = function () { cancelled = true; if (active) { active.abort(); } };
  function selectionChanged() {
    var count = Object.keys(selected).length;
    el('export').disabled = !count || count > 50; el('export').textContent = I18n.t("Download selected: ") + count + I18n.t(" books");
    el('download-link').hidden = true;
    if (count > 50) { el('download-message').textContent = I18n.t("Select fewer books; at most 50 per download."); }
  }
  function loadBooks() {
    var current = ++generation;
    el('download-message').textContent = I18n.t("Loading local library…");
    api('GET', '/api/books?q=' + encodeURIComponent(el('search').value) + '&offset=' + offset, null, function (err, data) {
      if (current !== generation) { return; }
      if (err) { el('download-message').textContent = err; return; }
      el('books').innerHTML = ''; pageBooks = data.books; offset = data.offset;
      el('download-message').textContent = data.total ? I18n.t("Total: ") + data.total + I18n.t(" matching books") : I18n.t("No matching ebooks.");
      el('page-count').textContent = data.total ? (offset + 1) + '–' + (offset + pageBooks.length) : '0';
      el('previous').disabled = offset === 0; el('next').disabled = !data.has_more;
      pageBooks.forEach(function (book) {
        var row = document.createElement('div'), label = document.createElement('label'), input = document.createElement('input'), name = document.createElement('span'), meta = document.createElement('p');
        row.className = 'book'; input.type = 'checkbox'; input.checked = !!selected[book.id]; input.setAttribute('data-book-id', book.id);
        input.onchange = function () { if (input.checked) { selected[book.id] = true; } else { delete selected[book.id]; } selectionChanged(); };
        name.textContent = book.name; name.className = 'name'; label.appendChild(input); label.appendChild(name); row.appendChild(label);
        meta.className = 'note'; meta.textContent = size(book.size) + (book.folder ? ' · ' + book.folder : I18n.t(" · Library root")); row.appendChild(meta); el('books').appendChild(row);
      });
      selectionChanged();
    });
  }
  function switchPanel(download) {
    el('upload-panel').hidden = download; el('download-panel').hidden = !download;
    el('upload-tab').className = download ? '' : 'selected'; el('download-tab').className = download ? 'selected' : '';
    if (download) { loadBooks(); }
  }
  el('upload-tab').onclick = function () { switchPanel(false); }; el('download-tab').onclick = function () { switchPanel(true); };
  el('search-form').onsubmit = function (event) { event.preventDefault(); offset = 0; loadBooks(); };
  el('refresh-books').onclick = function () { offset = 0; loadBooks(); };
  el('previous').onclick = function () { offset = Math.max(0, offset - 50); loadBooks(); }; el('next').onclick = function () { offset += 50; loadBooks(); };
  el('select-page').onclick = function () { pageBooks.forEach(function (b) { selected[b.id] = true; }); loadBooks(); };
  el('clear-selection').onclick = function () { selected = {}; loadBooks(); };
  el('export').onclick = function () {
    el('export').disabled = true; el('download-message').textContent = I18n.t("Preparing download…");
    api('POST', '/api/export', {ids: Object.keys(selected)}, function (err, result) {
      selectionChanged();
      if (err) { el('download-message').textContent = err; return; }
      el('download-link').href = result.url; el('download-link').hidden = false;
      el('download-link').textContent = I18n.t("Save selected: ") + result.count + I18n.t(" books") + (result.count > 1 ? '（ZIP）' : '');
      el('download-message').textContent = I18n.t("Ready. Use the link below to save; it expires in two minutes.");
    });
  };
  el('download-link').onclick = function () { el('download-message').textContent = I18n.t("Download handed to the browser; check its downloads list."); };
  var code = /(?:^#|&)code=(\d{6})(?:&|$)/.exec(window.location.hash);
  if (code) { el('code').value = code[1]; if (window.history.replaceState) { window.history.replaceState(null, '', window.location.pathname); } connect(); }
}());

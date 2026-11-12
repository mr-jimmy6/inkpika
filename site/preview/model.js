/* Browser-only sample state. No fetch, real credentials, device or cloud access. */
(function () {
  'use strict';
  var deviceLanguage = window.I18n.language();
  var I18n = {t: function (message) { return window.I18n.forLanguage(language()).t(message); }};
  var fixtures = window.inkpikaDemoData;
  var defaults = ['azw', 'azw3', 'mobi', 'prc', 'pdf', 'txt'];
  var timers = [], urls = [], state, serial = 0, epoch = 0;
  var model = {onchange: function () {}};
  function copy(value) { return JSON.parse(JSON.stringify(value)); }
  function notify(message) { model.onchange(message || ''); }
  function later(fn, delay) { var id = setTimeout(fn, delay); timers.push(id); return id; }
  function bytes(base64) {
    var raw = atob(base64), out = new Uint8Array(raw.length);
    for (var i = 0; i < raw.length; i += 1) { out[i] = raw.charCodeAt(i); }
    return out;
  }
  var books = fixtures.books.map(function (book) { return {name: book.name, size: book.size, bytes: bytes(book.base64)}; });
  function allowed(name) { return state.config.ebook_formats.indexOf(name.split('.').pop().toLowerCase()) >= 0; }
  function addLocal(name, content, folder) {
    var base = name, n = 1, dot = name.lastIndexOf('.');
    while (state.local.some(function (book) { return book.name === name && book.folder === folder; })) {
      name = base.slice(0, dot) + ' (' + n++ + ')' + base.slice(dot);
    }
    var book = {id: 'local-' + (++serial), name: name, size: content.length, bytes: content, folder: folder};
    state.local.push(book); return book;
  }
  function saveCloud(key, book) {
    if (!state.saved[key]) {
      state.saved[key] = true;
      addLocal(book.name.replace(/\.azw3$/, '-sample.azw3'), book.bytes, 'InkPika');
    }
  }
  function syncBooks() { return books.filter(function (book) { return allowed(book.name); }); }
  function syncDone() { return syncBooks().filter(function (book) { return state.saved['sync/' + book.name]; }).length; }
  function snapshotCount() { return {total: syncBooks().length, downloaded: syncDone(), pending: syncBooks().length - syncDone()}; }
  function cancelSync() {
    state.syncRun += 1;
    if (state.syncPhase === 'running' || state.syncPhase === 'scanning') { state.syncPhase = 'cancelled'; }
  }
  function service(name, on) {
    state.services[name] = on;
    if (!on) {
      state.pairVersion[name] += 1;
      if (name === 'cloud') {
        cancelSync();
        state.jobs.forEach(function (job) { if (job.status === 'queued' || job.status === 'downloading') { job.status = 'paused'; } });
      }
    }
    notify((on ? I18n.t("Running") : I18n.t("Stopped")) + {cloud: I18n.t("Cloud library"), wireless: I18n.t("Wireless transfer"), settings: I18n.t("Settings")}[name] + I18n.t(" sample service."));
  }
  function startSync() {
    if (state.syncPhase === 'running' || state.syncPhase === 'scanning') { return; }
    if (state.config.prefix && state.config.prefix !== 'sync/') {
      state.syncPhase = 'error'; notify(I18n.t("Sync scope must include sync/; leave the cloud folder scope blank or set it to sync/ in Settings.")); return;
    }
    service('cloud', true); state.syncPhase = 'running'; state.syncFetched = 0; state.syncSkipped = syncDone();
    var run = ++state.syncRun;
    function step() {
      if (run !== state.syncRun) { return; }
      var pending = syncBooks().filter(function (book) { return !state.saved['sync/' + book.name]; });
      if (!pending.length) { state.syncPhase = 'after'; notify(I18n.t("Sample sync complete. View and export saved books in Wireless transfer.")); return; }
      later(function () {
        if (run !== state.syncRun) { return; }
        saveCloud('sync/' + pending[0].name, pending[0]); state.syncFetched += 1;
        notify(I18n.t("Sample sync progress: ") + syncDone() + '/' + syncBooks().length + I18n.t(" books.")); step();
      }, 2200);
    }
    notify(I18n.t("Sample sync is running; view status or select Stop sync.")); step();
  }
  function refreshSync() {
    if (state.syncPhase === 'running' || state.syncPhase === 'scanning') { notify(I18n.t("Current sync progress refreshed.")); return; }
    var run = ++state.syncRun;
    state.syncPhase = 'scanning'; notify(I18n.t("Checking sample remote folders and local files."));
    later(function () {
      if (run !== state.syncRun) { return; }
      state.syncPhase = syncDone() === syncBooks().length ? 'after' : 'before';
      notify(I18n.t("Check complete: ") + snapshotCount().pending + I18n.t(" books pending sync."));
    }, 700);
  }
  function menu() {
    var root = copy(fixtures.menu_locales[language()][state.syncPhase]), counts = snapshotCount();
    var current = state.syncPhase === 'running' ? syncBooks().filter(function (book) { return !state.saved['sync/' + book.name]; })[0] : null;
    root[0].name = I18n.t("All books · ") + counts.downloaded + '/' + counts.total + I18n.t(" downloaded");
    root[0].items = [];
    books.forEach(function (book, index) {
      if (!allowed(book.name)) { return; }
      var entry = copy(fixtures.menu_locales[language()][state.saved['sync/' + book.name] ? 'after' : 'before'][0].items[index]);
      if (current && current.name === book.name) {
        entry.name = entry.name.replace('[NEW]', '[DL]'); entry.items[1].name = I18n.t("Status: Downloading");
        entry.items = entry.items.filter(function (line) { return line.name !== I18n.t("Not downloaded"); });
      }
      root[0].items.push(entry);
    });
    if (!root[0].items.length) { root[0].items = [{name: I18n.t("No ebooks to sync")}]; }
    if (state.syncPhase !== 'running' && state.syncPhase !== 'scanning') {
      root[2].name = I18n.t("View status") + (counts.pending ? I18n.t(" · Pending ") + counts.pending : '');
    }
    root[2].items[2].name = I18n.t("Total %d Saved %d Pending %d").replace("%d", counts.total).replace("%d", counts.downloaded).replace("%d", counts.pending);
    root[2].items[4].name = I18n.t("Downloaded ") + state.syncFetched + I18n.t(" · Skipped ") + state.syncSkipped;
    if (current) {
      var value = I18n.t("Current: sync/") + current.name, line = '', width = 0, lines = [];
      for (var c = 0; c < value.length; c += 1) {
        var step = value.charCodeAt(c) > 127 ? 2 : 1;
        if (width + step > 36) { lines.push({name: line}); line = ''; width = 0; }
        line += value.charAt(c); width += step;
      }
      lines.push({name: line});
      root[2].items.splice.apply(root[2].items, [5, 0].concat(lines));
    }
    if (state.syncPhase === 'error' && state.config.prefix && state.config.prefix !== 'sync/') {
      root[2].items[5].name = I18n.t("Leave cloud folder scope blank or set it to sync/");
    }
    root[6].items = root[6].items.filter(function (item) {
      var parts = item.preview.split('-'); return parts[0] === (state.services[parts[1]] ? 'stop' : 'start');
    });
    return root;
  }
  function jobTick() {
    if (state.jobTimer || !state.services.cloud || state.syncPhase === 'running') { return; }
    var job = state.jobs.filter(function (j) { return j.status === 'downloading' || j.status === 'queued'; })[0];
    if (!job) { return; }
    job.status = 'downloading'; state.jobTimer = true;
    later(function () {
      state.jobTimer = false;
      if (job.status === 'downloading' && state.services.cloud && state.syncPhase !== 'running') {
        job.downloaded = Math.min(job.size, job.downloaded + Math.ceil(job.size / 5));
        if (job.downloaded >= job.size) {
          job.status = 'completed'; job.transfer_complete = true; job.saved_locally = true; job.local_state = 'saved';
          saveCloud(job.key, books[job.bookIndex]); notify(I18n.t("Sample download complete: ") + books[job.bookIndex].name);
        }
      }
      jobTick();
    }, 900);
  }
  function query(url) {
    var out = {}, parts = (url.split('?')[1] || '').split('&');
    parts.forEach(function (part) { var split = part.split('='); out[split[0]] = decodeURIComponent(split.slice(1).join('=').replace(/\+/g, ' ')); });
    return out;
  }
  function mask(value) { return value.length > 4 ? value.slice(0, 2) + '****' + value.slice(-2) : '****'; }
  function providerLabel() {
    if (state.config.provider === 's3') { return {aws: 'Amazon S3', r2: 'Cloudflare R2', b2: 'Backblaze B2', spaces: 'DigitalOcean Spaces', tos: I18n.t("Volcengine TOS"), ibm: 'IBM Cloud Object Storage', gcs: 'Google Cloud Storage', cos: I18n.t("Tencent COS"), custom: I18n.t("S3-compatible storage / NAS")}[state.config.s3_preset] || I18n.t("S3-compatible storage / NAS"); }
    if (state.config.provider === 'azure_blob') { return 'Microsoft Azure Blob Storage'; }
    return state.config.provider === 'aliyun_oss' ? I18n.t("Alibaba Cloud OSS") : 'QingStor';
  }
  function configView() {
    var out = copy(state.config); out.provider_label = providerLabel(); out.sync_prefix = "sync/"; out.language = state.config.language || "auto"; out.effective_language = language(); out.revision = String(state.revision); out.default_formats = defaults.slice(); return out;
  }
  function language() { return state && state.config.language === "en" || state && state.config.language === "zh" ? state.config.language : deviceLanguage; }
  function saveConfig(data) {
    if (data.revision !== String(state.revision)) { return I18n.t("Configuration changed; reload before saving."); }
    if (data.language !== undefined && ['auto', 'en', 'zh'].indexOf(data.language) < 0) { return I18n.t('language must be auto, en or zh'); }
    if (data.language_only) { state.config.language = data.language; state.revision += 1; return ''; }
    if (['aliyun_oss', 'qingstor', 's3', 'azure_blob'].indexOf(data.provider) < 0) { return I18n.t("Select a supported storage platform."); }
    var s3 = data.provider === 's3', preset = data.s3_preset || 'custom', azure = data.provider === 'azure_blob';
    if (s3 && ['aws', 'r2', 'b2', 'spaces', 'tos', 'ibm', 'gcs', 'cos', 'custom'].indexOf(preset) < 0) { return I18n.t("Select a supported storage platform."); }
    if (s3 && !data.region) { data.region = preset === 'r2' || preset === 'gcs' ? 'auto' : preset === 'custom' ? 'us-east-1' : ''; }
    if (azure) { data.region = ''; }
    if (s3 && preset === 'r2' && data.region !== 'auto') { return I18n.t("R2 region must be auto."); }
    if (s3 && (preset === 'r2' || preset === 'custom') && !data.endpoint) { return I18n.t("Enter an HTTPS S3 endpoint."); }
    if (!(s3 ? /^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$/ : /^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$/).test(data.bucket)) { return I18n.t("Enter a valid bucket name."); }
    if (!azure && !/^[a-z0-9-]+$/.test(data.region)) { return I18n.t("Enter a region, such as cn-beijing or pek3b."); }
    if (azure && data.bucket.indexOf('--') >= 0) { return I18n.t("Container names cannot contain consecutive hyphens."); }
    if (azure && data.access_key_id && !/^[a-z0-9]{3,24}$/.test(data.access_key_id)) { return I18n.t("Account names must contain 3–24 lowercase letters or digits."); }
    if (azure && data.secret_access_key && !/^[A-Za-z0-9+/]{85}[AQgw]==$/.test(data.secret_access_key)) { return I18n.t("Enter a Base64-encoded 64-byte account key."); }
    if (s3 && preset === 'cos' && !/^[a-z0-9][a-z0-9-]*-[0-9]+$/.test(data.bucket)) { return I18n.t("Tencent COS bucket requires an APPID suffix."); }
    if (s3 && (preset === 'tos' || preset === 'cos') && data.s3_addressing && data.s3_addressing !== 'virtual') { return I18n.t("This platform uses virtual-hosted addressing."); }
    if ((azure || (s3 && (preset === 'ibm' || preset === 'gcs'))) && data.security_token) { return I18n.t("This platform does not use temporary security tokens."); }
    if (data.endpoint && !/^https:\/\/[a-zA-Z0-9.-]+(?::\d+)?\/?$/.test(data.endpoint)) { return I18n.t("Endpoint must be a regional HTTPS address without a path, credentials or query."); }
    if (data.socks5_proxy && !/^socks5h?:\/\/(?:[^\/\s]+@)?(?:[^\s\/:]+|\[[^\]]+\]):\d+$/.test(data.socks5_proxy)) { return I18n.t("Enter a socks5:// or socks5h://proxy:port address."); }
    if (data.ca_bundle && data.ca_bundle.charAt(0) !== '/') { return I18n.t("CA certificate requires an absolute path on Kindle."); }
    if (data.download_dir.indexOf('/mnt/us/documents/') !== 0 || /(?:^|\/)\.\.?(?:\/|$)/.test(data.download_dir)) { return I18n.t("Download folder must be under /mnt/us/documents."); }
    if (!(data.max_download_mb >= 1 && data.max_download_mb <= 16384 && data.max_download_mb % 1 === 0)) { return I18n.t("Per-book limit must be an integer between 1 and 16384 MB."); }
    var formats = [], invalid = false;
    if (data.ebook_formats.length > 32) { return I18n.t("Specify at most 32 ebook formats."); }
    (data.ebook_formats.length ? data.ebook_formats : defaults).forEach(function (format) {
      format = format.toLowerCase().replace(/^\./, '');
      if (!/^[a-z0-9]{1,12}$/.test(format)) { invalid = true; }
      if (formats.indexOf(format) < 0) { formats.push(format); }
    });
    if (invalid) { return I18n.t("Ebook extensions may contain only letters and digits."); }
    var sourceChanged = ['provider', 'bucket', 'region', 'endpoint'].some(function (field) { return state.config[field] !== data[field]; });
    cancelSync();
    ['language', 'provider', 'bucket', 'region', 'endpoint', 'prefix', 'download_dir', 'max_download_mb'].forEach(function (field) { state.config[field] = data[field]; });
    state.config.s3_preset = s3 ? preset : ''; state.config.s3_addressing = s3 ? data.s3_addressing || (preset === 'tos' || preset === 'cos' || ((preset === 'aws' || preset === 'spaces') && data.bucket.indexOf('.') < 0) ? 'virtual' : 'path') : '';
    state.config.ca_bundle = data.ca_bundle || '';
    if (data.clear_socks5_proxy) { state.config.socks5_proxy_set = false; }
    else if (data.socks5_proxy) { state.config.socks5_proxy_set = true; }
    if (state.config.prefix && state.config.prefix.slice(-1) !== '/') { state.config.prefix += '/'; }
    state.config.ebook_formats = formats;
    if (data.access_key_id) { state.config.access_key_id_masked = mask(data.access_key_id); }
    if (data.secret_access_key) { state.config.secret_access_key_masked = mask(data.secret_access_key); }
    if (data.clear_security_token) { state.config.security_token_set = false; }
    else if (data.security_token) { state.config.security_token_set = true; }
    // Retain only masked display values; never retain typed credential strings.
    state.jobs.forEach(function (job) { if (job.status === 'downloading' || job.status === 'queued') { job.status = 'paused'; } });
    if (sourceChanged) { state.saved = {}; state.jobs = []; state.syncPhase = 'before'; state.syncFetched = 0; state.syncSkipped = 0; }
    state.revision += 1; state.check = {};
    notify(I18n.t("Sample configuration saved. Refresh Configuration in Settings to view it; no real device was changed."));
    return '';
  }
  function utf8(text) { var s = unescape(encodeURIComponent(text)), out = new Uint8Array(s.length); for (var i = 0; i < s.length; i += 1) { out[i] = s.charCodeAt(i); } return out; }
  function crc32(data) {
    var crc = -1, i, j;
    for (i = 0; i < data.length; i += 1) { crc ^= data[i]; for (j = 0; j < 8; j += 1) { crc = (crc >>> 1) ^ ((crc & 1) ? 0xedb88320 : 0); } }
    return (crc ^ -1) >>> 0;
  }
  // ZIP STORE, with UTF-8 names and CRC32. Files never leave the browser.
  function zip(selected) {
    var chunks = [], directory = [], offset = 0, directorySize = 0;
    selected.forEach(function (book) {
      var name = utf8((book.folder ? book.folder + '/' : '') + book.name), crc = crc32(book.bytes);
      var local = new Uint8Array(30 + name.length), l = new DataView(local.buffer);
      l.setUint32(0, 0x04034b50, true); l.setUint16(4, 20, true); l.setUint16(6, 0x800, true); l.setUint16(12, 33, true);
      l.setUint32(14, crc, true); l.setUint32(18, book.size, true); l.setUint32(22, book.size, true); l.setUint16(26, name.length, true); local.set(name, 30);
      var central = new Uint8Array(46 + name.length), c = new DataView(central.buffer);
      c.setUint32(0, 0x02014b50, true); c.setUint16(4, 20, true); c.setUint16(6, 20, true); c.setUint16(8, 0x800, true); c.setUint16(14, 33, true);
      c.setUint32(16, crc, true); c.setUint32(20, book.size, true); c.setUint32(24, book.size, true); c.setUint16(28, name.length, true); c.setUint32(42, offset, true); central.set(name, 46);
      chunks.push(local, book.bytes); directory.push(central); offset += local.length + book.size; directorySize += central.length;
    });
    var end = new Uint8Array(22), e = new DataView(end.buffer);
    e.setUint32(0, 0x06054b50, true); e.setUint16(8, selected.length, true); e.setUint16(10, selected.length, true); e.setUint32(12, directorySize, true); e.setUint32(16, offset, true);
    return new Blob(chunks.concat(directory, [end]), {type: 'application/zip'});
  }
  function result(code, value) { return {status: code, data: copy(value)}; }
  function api(screen, method, url, body, headers) {
    var type = screen.split('-')[0], path = url.split('?')[0], args = query(url), data = body || {}, i;
    if (!state.services[type]) { return result(503, {error: I18n.t("Sample service stopped; reopen from KUAL.")}); }
    if (path === '/api/pair') {
      return data.code === '123456' ? result(200, {token: type + '-' + state.pairVersion[type]}) : result(403, {error: I18n.t("Incorrect code. The sample connection code is 123456.")});
    }
    if (type !== 'cloud' && screen.indexOf('-device') < 0 && headers['x-transfer-token'] !== type + '-' + state.pairVersion[type]) {
      return result(401, {error: I18n.t("Pairing expired; reopen the page and enter the sample code.")});
    }
    if (path === '/api/connection') {
      var settings = type === 'settings';
      return result(200, {title: settings ? I18n.t("Settings") : I18n.t("Wireless transfer"), description: settings ? I18n.t("Edit InkPika storage credentials, download folder and ebook formats from a phone or computer.") : I18n.t("Transfer ebooks between a phone or computer and Kindle without cloud storage."), stop_label: settings ? I18n.t("Stop settings") : I18n.t("Stop wireless transfer"), wifi: I18n.t("InkPika-Demo (sample)"), code: '123456', endpoints: [{ip: '192.0.2.10', url: 'http://192.0.2.10:' + (settings ? '8791' : '8789') + '/', qr: fixtures.qr}]});
    }
    if (path === '/api/info') {
      return result(200, {provider_label: providerLabel(), bucket: state.config.bucket, prefix: state.config.prefix, free_bytes: 1610612736, max_upload_mb: 512, ebook_formats: state.config.ebook_formats});
    }
    if (path === '/api/config') {
      if (type === 'wireless' || (screen === 'settings-device' && method !== 'GET')) { return result(404, {error: 'Unknown endpoint'}); }
      if (method === 'POST') { var error = saveConfig(data); return result(error ? 400 : 200, error ? {error: error} : {message: I18n.t("Sample configuration saved and applied for this browser preview only.")}); }
      if (type === 'settings') { return result(200, configView()); }
      return result(200, {version: state.config.version, packaged_at: state.config.packaged_at, provider: state.config.provider, provider_label: providerLabel(), connection: state.config.socks5_proxy_set ? I18n.t("SOCKS5 (configured)") : I18n.t("Direct"), bucket: state.config.bucket, download_dir: state.config.download_dir, sync_prefix: 'sync/', ebook_formats: state.config.ebook_formats.join(language() === 'zh' ? '、' : ', ').toUpperCase(), access_key_id_masked: state.config.access_key_id_masked, secret_access_key_masked: state.config.secret_access_key_masked});
    }
    if (path === '/api/connection-check') {
      if (screen !== 'settings-device' && type !== 'cloud') { return result(404, {error: 'Unknown endpoint'}); }
      if (method === 'POST') {
        state.check = {updated: 1794096000, updated_local: I18n.t("2026-11-08 08:00:00 (sample time)"), network: I18n.t("Passed"), access: I18n.t("Passed"), read: I18n.t("Passed"), detail: I18n.t("Local sample result; no request was sent to cloud storage.")};
      }
      return result(200, state.check);
    }
    if (path === '/api/objects') {
      var prefix = args.prefix || '', page = args.marker ? 1 : 0, visible = [];
      if (['', 'sync/', '分类/随笔/'].indexOf(prefix) >= 0) { visible = books.slice(page ? 2 : 0, page ? 3 : 2); }
      return result(200, {common_prefixes: prefix === '' && !page ? ['sync/', '分类/'] : prefix === '分类/' ? ['分类/随笔/'] : [], keys: visible.map(function (book) {
        return {key: prefix + book.name, size: book.size, modified_local: '2026-11-08 08:00', supported: allowed(book.name), downloaded: !!state.saved[prefix + book.name]};
      }), next_marker: visible.length === 2 && !page ? 'sample-page-2' : ''});
    }
    if (path === '/api/download') {
      var index = -1; books.forEach(function (book, n) { if (data.key.split('/').pop() === book.name) { index = n; } });
      if (index < 0 || !allowed(data.key)) { return result(400, {error: I18n.t("File not found or format not allowed.")}); }
      if (state.syncPhase === 'running') { return result(409, {error: I18n.t("Sync is using the transfer channel; try again shortly.")}); }
      var existing = state.jobs.filter(function (job) { return job.key === data.key; })[0];
      if (existing) { return result(200, existing); }
      var book = books[index], completed = !!state.saved[data.key];
      var job = {id: 'job-' + (++serial), key: data.key, name: book.name.replace(/\.azw3$/, '-sample.azw3'), size: book.size, downloaded: completed ? book.size : 0, status: completed ? 'completed' : 'queued', saved_locally: completed, transfer_complete: completed, local_state: completed ? 'saved' : '', bookIndex: index};
      state.jobs.push(job); jobTick(); return result(200, job);
    }
    if (path === '/api/jobs') { jobTick(); return result(200, state.jobs); }
    if (path === '/api/jobs/action') {
      var active = state.jobs.filter(function (job) { return job.id === data.id; })[0];
      if (!active) { return result(404, {error: I18n.t("Download record does not exist.")}); }
      active.status = data.action === 'cancel' ? 'cancelled' : 'queued'; jobTick(); return result(200, active);
    }
    if (path === '/api/jobs/clear') { state.jobs = state.jobs.filter(function (job) { return ['downloading', 'queued', 'paused', 'cancelled'].indexOf(job.status) >= 0; }); return result(200, {}); }
    if (path === '/api/books') {
      var matches = state.local.filter(function (book) { return allowed(book.name) && book.name.toLowerCase().indexOf((args.q || '').toLowerCase()) >= 0; }), offset = Number(args.offset) || 0;
      return result(200, {books: matches.slice(offset, offset + 50).map(function (book) { return {id: book.id, name: book.name, size: book.size, folder: book.folder}; }), offset: offset, total: matches.length, has_more: offset + 50 < matches.length});
    }
    if (path === '/api/upload') {
      if (state.syncPhase === 'running' || state.jobs.some(function (job) { return job.status === 'downloading'; })) { return result(409, {error: I18n.t("Sample transfer channel is busy; try again shortly.")}); }
      var name = decodeURIComponent(headers['x-file-name'] || '');
      if (!allowed(name) || !data.bytes || !data.bytes.length || /[\\/]/.test(name)) { return result(400, {error: I18n.t("Invalid filename, format or file content.")}); }
      var saved = addLocal(name, data.bytes, 'InkPika/Wireless'); notify(I18n.t("Saved ") + saved.name + I18n.t(" to the browser sample library.")); return result(201, {name: saved.name});
    }
    if (path === '/api/export') {
      var selected = state.local.filter(function (book) { return data.ids.indexOf(book.id) >= 0 && allowed(book.name); });
      if (!selected.length || selected.length > 50 || selected.length !== data.ids.length) { return result(400, {error: I18n.t("Select 1–50 valid sample books.")}); }
      var blob = selected.length > 1 ? zip(selected) : new Blob([selected[0].bytes], {type: 'application/octet-stream'});
      var objectURL = URL.createObjectURL(blob); urls.push(objectURL);
      return result(200, {url: objectURL, count: selected.length, name: selected.length > 1 ? 'KindleInkPika-样例图书.zip' : selected[0].name});
    }
    return result(404, {error: I18n.t("No matching action in this sample.")});
  }
  function reset() {
    timers.forEach(clearTimeout); timers = []; urls.forEach(function (url) { URL.revokeObjectURL(url); }); urls = []; epoch += 1;
    state = {services: {cloud: false, wireless: false, settings: false}, pairVersion: {cloud: epoch, wireless: epoch, settings: epoch}, saved: {}, local: [], jobs: [], jobTimer: false, syncPhase: 'before', syncRun: 0, syncFetched: 0, syncSkipped: 0, revision: 1, check: {}, config: {version: '0.0.1.dev', packaged_at: '', provider: 'aliyun_oss', bucket: 'demo-library', region: 'cn-beijing', endpoint: '', prefix: '', download_dir: '/mnt/us/documents/InkPika', max_download_mb: 512, ebook_formats: defaults.slice(), access_key_id_masked: 'de****id', secret_access_key_masked: 'de****et', security_token_set: false, s3_preset: '', s3_addressing: '', ca_bundle: '', socks5_proxy_set: false}};
    books.forEach(function (book) { addLocal(book.name, book.bytes, 'InkPika/Wireless'); });
    model.state = state; notify(I18n.t("Sample reset; three test books are waiting to sync."));
  }
  model.reset = reset; model.menu = menu; model.api = api; model.books = books; model.service = service;
  model.sync = function (action) { if (action === 'start') { startSync(); } else if (action === 'cancel') { cancelSync(); notify(I18n.t("Sample sync stopped; completed books preserved.")); jobTick(); } else { refreshSync(); } };
  model.language = language; model.copy = copy; reset(); window.inkpikaDemo = model;
}());

(function () {
  'use strict';
  var match = /(?:^#|&)token=([a-f0-9]{64})(?:&|$)/.exec(window.location.hash);
  var checkBusy = false, checkGeneration = 0, configGeneration = 0;
  function el(id) { return document.getElementById(id); }
  function text(node, value) { node.textContent = value; }
  function make(tag, className, value) { var node = document.createElement(tag); node.className = className || ''; if (typeof value !== 'undefined') { text(node, value); } return node; }
  function api(method, path, data, callback) {
    if (!match) { callback(I18n.t("Open this page from KUAL.")); return; }
    var xhr = new XMLHttpRequest(), finished = false;
    function done(err, value) { if (!finished) { finished = true; callback(err, value); } }
    xhr.open(method, path, true); xhr.timeout = 45000;
    xhr.setRequestHeader('X-Transfer-Token', match[1]);
    if (data !== null) { xhr.setRequestHeader('Content-Type', 'application/json'); }
    xhr.onreadystatechange = function () {
      if (xhr.readyState !== 4) { return; }
      var result; try { result = JSON.parse(xhr.responseText); } catch (ignore) { done(I18n.t("Cannot read the Kindle response")); return; }
      done(xhr.status === 200 ? null : result.error || I18n.t("Request failed"), result);
    };
    xhr.onerror = function () { done(I18n.t("Service disconnected; reopen this page from KUAL.")); };
    xhr.ontimeout = function () { done(I18n.t("Connection timed out; check Wi-Fi and try again.")); };
    xhr.send(data === null ? null : JSON.stringify(data));
  }
  function renderCheck(result) {
    var box = el('check-result'); box.innerHTML = '';
    el('start-check').disabled = result.running || checkBusy;
    text(el('start-check'), result.running || checkBusy ? I18n.t("Testing…") : I18n.t("Start test"));
    if (result.running) { text(el('check-message'), I18n.t("Test in progress; use Refresh info shortly to see the results.")); return; }
    if (!result.updated) { text(el('check-message'), I18n.t("Not tested yet. Select Start test to check connectivity and access permissions.")); return; }
    text(el('check-message'), I18n.t("Last test: ") + result.updated_local);
    var stages = make('div', 'check-stages');
    stages.appendChild(make('div', '', I18n.t("HTTPS connection: ") + result.network));
    stages.appendChild(make('div', '', I18n.t("Folder listing: ") + result.access));
    stages.appendChild(make('div', '', I18n.t("File reading: ") + result.read));
    box.appendChild(stages);
    if (result.detail) { box.appendChild(make('p', 'job-error', result.detail)); }
  }
  function loadCheck() {
    if (checkBusy) { return; }
    var request = ++checkGeneration;
    api('GET', '/api/connection-check', null, function (err, result) {
      if (request !== checkGeneration) { return; }
      if (err) { text(el('check-message'), err); return; }
      renderCheck(result);
    });
  }
  function loadConfiguration() {
    var request = ++configGeneration;
    el('refresh-config').disabled = true;
    api('GET', '/api/config', null, function (err, info) {
      if (request !== configGeneration) { return; }
      el('refresh-config').disabled = false;
      text(el('config-message'), err || '');
      if (err) { return; }
      var fields = [[I18n.t("Storage platform"), info.provider_label || info.provider], ['bucket', info.bucket], [I18n.t("Connection"), (info.socks5_proxy_set ? I18n.t("SOCKS5 (configured)") : I18n.t("Direct"))], ['download_dir', info.download_dir], [I18n.t("Sync scope"), info.sync_prefix], [I18n.t("Allowed formats"), (info.ebook_formats && info.ebook_formats.length ? info.ebook_formats : info.default_formats).join(", ").toUpperCase()], ['AccessKey ID', I18n.t(info.access_key_id_masked)], ['AccessKey Secret', I18n.t(info.secret_access_key_masked)], [I18n.t("Version"), info.version], [I18n.t("Packaged at (UTC)"), info.packaged_at || I18n.t("Not packaged (source run)")]];
      var table = el('config-fields'), body = make('tbody'), i, row;
      table.innerHTML = '';
      for (i = 0; i < fields.length; i++) {
        row = make('tr'); row.appendChild(make('th', '', fields[i][0])); row.appendChild(make('td', '', fields[i][1])); body.appendChild(row);
      }
      table.appendChild(body);
    });
    loadCheck();
  }
  function startCheck() {
    if (checkBusy) { return; }
    checkBusy = true; ++checkGeneration;
    el('start-check').disabled = true; text(el('start-check'), I18n.t("Testing…"));
    text(el('check-message'), I18n.t("Checking HTTPS, folder listing and file reading; this may take 35 seconds…")); el('check-result').innerHTML = '';
    api('POST', '/api/connection-check', {}, function (err, result) {
      checkBusy = false; ++checkGeneration;
      el('start-check').disabled = false; text(el('start-check'), I18n.t("Start test"));
      if (err) { text(el('check-message'), err); return; }
      renderCheck(result);
    });
  }
  function tab(pairing) {
    el('configuration').hidden = pairing; el('pairing').hidden = !pairing;
    el('configuration-tab').className = pairing ? '' : 'selected';
    el('pairing-tab').className = pairing ? 'selected' : '';
    if (!pairing) { loadConfiguration(); }
  }
  el('configuration-tab').onclick = function () { tab(false); };
  el('pairing-tab').onclick = function () { tab(true); };
  el('refresh-config').onclick = loadConfiguration;
  el('start-check').onclick = startCheck;
  loadConfiguration();
}());

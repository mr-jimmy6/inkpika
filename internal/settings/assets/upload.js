(function () {
  'use strict';
  var token = '', revision = '', fields = ['language', 'bucket', 'region', 'endpoint', 'prefix', 'download_dir', 'max_download_mb', 'ca_bundle'];
  function el(id) { return document.getElementById(id); }
  function message(text, atTop) {
    var editing = !el('configuration').hidden && !atTop;
    el('message').textContent = editing ? '' : text;
    el('save-message').textContent = editing ? text : '';
  }
  function s3Preset() { var platform = el('provider').value; return platform.indexOf('s3:') === 0 ? platform.slice(3) : ''; }
  function providerHelp(reset) {
    var preset = s3Preset(), s3 = !!preset, azure = el('provider').value === 'azure_blob';
    var temporary = el('provider').value === 'aliyun_oss' || (s3 && preset !== 'ibm' && preset !== 'gcs');
    el('s3-options').hidden = !s3;
    el('endpoint').required = s3 && (preset === 'r2' || preset === 'custom');
    el('region').required = !azure && !(s3 && (preset === 'r2' || preset === 'gcs' || preset === 'custom'));
    el('region').parentNode.hidden = azure;
    el('region').disabled = azure;
    el('security_token').parentNode.hidden = !temporary;
    el('security_token').disabled = !temporary;
    document.querySelector('label[for="bucket"]').textContent = azure ? I18n.t("Container") : I18n.t("Bucket");
    document.querySelector('label[for="prefix"]').textContent = azure ? I18n.t("Cloud folder scope (leave blank for the whole container)") : I18n.t("Cloud folder scope (leave blank for the whole bucket)");
    document.querySelector('label[for="access_key_id"]').textContent = azure ? I18n.t("Storage account name") : preset === 'ibm' || preset === 'gcs' ? 'HMAC Access Key ID' : 'AccessKey ID';
    document.querySelector('label[for="secret_access_key"]').textContent = azure ? I18n.t("Account Key") : preset === 'ibm' || preset === 'gcs' ? 'HMAC Secret Access Key' : 'AccessKey Secret';
    el('endpoint').placeholder = azure ? I18n.t("HTTPS Blob address without the container") : I18n.t("HTTPS address without the bucket");
    if (reset) {
      el('endpoint').value = ''; el('s3_addressing').value = '';
      el('region').value = s3 && (preset === 'r2' || preset === 'gcs') ? 'auto' : s3 && (preset === 'aws' || preset === 'custom') ? 'us-east-1' : '';
      el('security_token').value = ''; el('clear_security_token').checked = true;
    }
    el('s3_addressing').disabled = preset === 'tos' || preset === 'cos';
    if (el('s3_addressing').disabled) { el('s3_addressing').value = 'virtual'; }
    var hints = {
      aws: I18n.t("Enter the actual region, such as us-east-1; leave Endpoint blank to generate it. Requires s3:ListBucket and s3:GetObject."),
      r2: I18n.t("Use region auto. Enter the HTTPS S3 API Endpoint from the console without the bucket, and use R2 S3 access keys."),
      b2: I18n.t("Enter the actual region, such as us-west-004. Leave Endpoint blank or copy it from B2. Use an application key with list/read permissions."),
      spaces: I18n.t("Enter the actual region, such as nyc3; leave Endpoint blank to generate it. Use Spaces access keys."),
      tos: I18n.t("Enter a region such as cn-beijing. Uses tos-s3-REGION.volces.com and virtual-hosted addressing. Requires TOS list/read permissions."),
      ibm: I18n.t("Enter a region such as us-south (or us / eu / ap for a cross-region instance, as shown in the console). Use HMAC access_key_id / secret_access_key from service credentials, not an IAM API Key. Endpoint can be generated."),
      gcs: I18n.t("Use the HMAC access ID and secret from interoperability settings, not service-account JSON. Defaults: region auto, endpoint https://storage.googleapis.com. Bucket must be a 3–63 character DNS-style name."),
      cos: I18n.t("Bucket must include the APPID suffix, such as books-1250000000. Enter a region such as ap-guangzhou. Endpoint can be generated; uses virtual-hosted addressing and Tencent SecretId / SecretKey."),
      custom: I18n.t("Enter the NAS / S3 HTTPS endpoint (optional port, no bucket or path). Blank region defaults to us-east-1; it must match the server.")
    };
    el('provider-help').textContent = s3 ? hints[preset] : azure ? I18n.t("Uses native Azure Blob APIs and a Base64 account key. Shared Key access must be allowed. Blank Endpoint generates https://ACCOUNT.blob.core.windows.net; enter the actual endpoint for sovereign clouds such as China. The client is read-only, but the account key grants broader permissions; keep it private.") : el('provider').value === 'aliyun_oss' ? I18n.t("Enter a region such as cn-beijing (without oss-); leave Endpoint blank to generate it.") : I18n.t("Enter a region such as pek3b; leave Endpoint blank to generate it.");
  }
  function api(method, path, data, done) {
    var xhr = new XMLHttpRequest(); xhr.open(method, path, true); xhr.timeout = 45000;
    xhr.setRequestHeader('X-Transfer-Token', token);
    if (data !== null) { xhr.setRequestHeader('Content-Type', 'application/json'); }
    xhr.onload = function () {
      var responseLanguage = xhr.getResponseHeader('Content-Language');
      if (responseLanguage && responseLanguage !== I18n.language()) { I18n.setLanguage(responseLanguage); } var result; try { result = JSON.parse(xhr.responseText); } catch (ignore) { done(I18n.t("Cannot read the Kindle response")); return; } done(xhr.status === 200 ? null : result.error || I18n.t("Request failed"), result); };
    xhr.onerror = function () { done(I18n.t("Connection interrupted; check Kindle. Reload configuration to verify whether it was saved.")); };
    xhr.ontimeout = function () { done(I18n.t("Request timed out; check Kindle and reload configuration to verify the result.")); };
    xhr.send(data === null ? null : JSON.stringify(data));
  }
  function load(done) {
    el('save').disabled = true;
    api('GET', '/api/config', null, function (err, data) {
      el('save').disabled = false;
      if (err) { message(err); return; }
      I18n.setLanguage(data.effective_language);
      revision = data.revision; fields.forEach(function (id) { el(id).value = data[id] || ''; });
      el('provider').value = data.provider === 's3' ? 's3:' + (data.s3_preset || 'custom') : data.provider;
      el('s3_addressing').value = data.s3_addressing || '';
      el('ebook_formats').value = (data.ebook_formats || []).join(', ');
      el('format-help').textContent = I18n.t("Leave blank for built-in formats: ") + data.default_formats.join(I18n.language() === 'zh' ? '、' : ', ').toUpperCase() + I18n.t(". Separate entries with commas or spaces.");
      ['access_key_id', 'secret_access_key', 'security_token', 'socks5_proxy'].forEach(function (id) { el(id).value = ''; });
      el('access_key_id').placeholder = I18n.t(data.access_key_id_masked) + I18n.t(" · leave blank to keep");
      el('secret_access_key').placeholder = I18n.t(data.secret_access_key_masked) + I18n.t(" · leave blank to keep");
      el('security_token').placeholder = data.security_token_set ? I18n.t("Set · leave blank to keep") : I18n.t("Not set");
      el('clear_security_token').checked = false;
      el('socks5_proxy').placeholder = data.socks5_proxy_set ? I18n.t("Set · leave blank to keep") : I18n.t("Not set · leave blank for direct access");
      el('clear_socks5_proxy').checked = false; providerHelp(false);
      el('pair').hidden = true; el('configuration').hidden = false;
      if (done) { done(); } else { message(I18n.t("Current configuration loaded.")); }
    });
  }
  function connect(event) {
    if (event) { event.preventDefault(); }
    if (!/^\d{6}$/.test(el('code').value)) { message(I18n.t("Enter the six-digit connection code.")); return; }
    el('connect').disabled = true;
    api('POST', '/api/pair', {code: el('code').value}, function (err, data) {
      el('connect').disabled = false;
      if (err) { message(err); return; }
      token = data.token; load();
    });
  }
  el('save-language').onclick = function () {
    el('save-language').disabled = true;
    api('POST', '/api/config', {revision: revision, language: el('language').value, language_only: true}, function (err, data) {
      el('save-language').disabled = false;
      if (err) { message(err, true); return; }
      load(function () { message(data.message, true); });
    });
  };
  el('advanced-toggle').onclick = function () {
    var open = el('advanced-fields').hidden;
    el('advanced-fields').hidden = !open;
    el('advanced-toggle').setAttribute('aria-expanded', open ? 'true' : 'false');
  };
  el('pair').onsubmit = connect;
  el('reload').onclick = function () { load(); };
  el('provider').onchange = function () { providerHelp(true); };
  el('configuration').onsubmit = function (event) {
    event.preventDefault();
    var preset = s3Preset(), data = {revision: revision, provider: preset ? 's3' : el('provider').value};
    fields.forEach(function (id) { data[id] = el(id).value; });
    data.max_download_mb = Number(data.max_download_mb);
    data.s3_preset = preset;
    data.s3_addressing = data.provider === 's3' ? el('s3_addressing').value : '';
    data.ebook_formats = el('ebook_formats').value.split(/[,，\s]+/).filter(function (v) { return !!v; });
    ['access_key_id', 'secret_access_key', 'security_token', 'socks5_proxy'].forEach(function (id) { data[id] = el(id).value; });
    data.clear_security_token = el('clear_security_token').checked;
    data.clear_socks5_proxy = el('clear_socks5_proxy').checked;
    el('save').disabled = true; el('reload').disabled = true; message(I18n.t("Validating and saving…"));
    api('POST', '/api/config', data, function (err, result) {
      el('save').disabled = false; el('reload').disabled = false;
      if (err) { message(err); return; }
      load(function () { message(result.message); });
    });
  };
  var code = /(?:^#|&)code=(\d{6})(?:&|$)/.exec(window.location.hash);
  if (code) { el('code').value = code[1]; if (window.history.replaceState) { window.history.replaceState(null, '', window.location.pathname); } connect(); }
}());

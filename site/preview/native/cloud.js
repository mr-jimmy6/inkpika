(function () {
  'use strict';
  var token = '', root = '', prefix = '', markers = [''], page = 0, next = '', busy = false;
  var poll = null, generation = 0, jobsGeneration = 0, activeTab = 'library';
  var visibleFiles = [];
  var expandedJobs = {};
  var statuses = {queued: I18n.t("Queued"), downloading: I18n.t("Downloading"), completed: I18n.t("Saved to Kindle"), failed: I18n.t("Download failed"), paused: I18n.t("Waiting to resume"), cancelled: I18n.t("Paused")};
  function el(id) { return document.getElementById(id); }
  function text(node, value) { node.textContent = value; }
  function notice(message) { text(el('notice'), message || ''); el('notice').hidden = !message; }
  function api(method, url, data, callback) {
    var xhr = new XMLHttpRequest();
    xhr.open(method, url, true); xhr.timeout = 55000;
    xhr.setRequestHeader('X-InkPika-Token', token);
    if (data !== null) { xhr.setRequestHeader('Content-Type', 'application/json'); }
    var finished = false;
    function done(error, value) { if (!finished) { finished = true; callback(error, value); } }
    xhr.onreadystatechange = function () {
      if (xhr.readyState !== 4) { return; }
      var out;
      try { out = JSON.parse(xhr.responseText); } catch (ignore) { done(I18n.t("Cannot connect to the local service; reopen from KUAL.")); return; }
      if (xhr.status < 200 || xhr.status >= 300) { done(out.error || I18n.t("Request failed")); return; }
      done(null, out);
    };
    xhr.onerror = function () { done(I18n.t("Connection interrupted; check that the service is still running.")); };
    xhr.ontimeout = function () { done(I18n.t("Connection timed out; check Wi-Fi and try again.")); };
    xhr.send(data === null ? null : JSON.stringify(data));
  }
  function size(n) { if (n >= 1073741824) { return (n / 1073741824).toFixed(1) + ' GB'; } if (n >= 1048576) { return (n / 1048576).toFixed(1) + ' MB'; } if (n >= 1024) { return Math.ceil(n / 1024) + ' KB'; } return n + ' B'; }
  function make(tag, className, value) { var n = document.createElement(tag); n.className = className || ''; if (typeof value !== 'undefined') { text(n, value); } return n; }
  function button(label, handler, className) { var b = make('button', className, label); b.type = 'button'; b.onclick = handler; return b; }
  function tab(name) {
    activeTab = name; el('library').hidden = name !== 'library'; el('downloads').hidden = name !== 'downloads';
    notice('');
    el('library-tab').className = name === 'library' ? 'selected' : '';
    el('downloads-tab').className = name === 'downloads' ? 'selected' : '';
    if (name === 'downloads') { loadJobs(); }
    else if (!busy) { loadFiles(); }
  }
  function navigate(folder) { if (busy) { return; } prefix = folder; markers = ['']; page = 0; loadFiles(); }
  function addRow(parent, item, folder) {
    var row = make('div', 'row');
    var label = folder ? item.slice(prefix.length).replace(/\/$/, '') : item.key.slice(prefix.length);
    var ext = folder ? I18n.t("Folder") : item.key.split('.').pop().toUpperCase();
    var info = make('div', 'file-text'); info.appendChild(make('p', 'name', label));
    if (!folder) { info.appendChild(make('p', 'meta', size(item.size) + ' · ' + ext + ' · ' + (item.modified_local ? I18n.t("Updated ") + item.modified_local : I18n.t("Update time unknown")))); }
    row.appendChild(info);
    var action = make('div', 'file-action');
    if (folder) { action.appendChild(button(I18n.t("Open ›"), function () { navigate(item); })); }
    else { visibleFiles.push({item: item, action: action}); fileAction(item, action); }
    row.appendChild(action); parent.appendChild(row);
  }
  function fileAction(item, action) {
    action.innerHTML = '';
    if (item.downloaded) { action.appendChild(make('span', 'downloaded', I18n.t("Downloaded"))); }
    else if (item.supported) {
      var b = button(I18n.t("Download ↓"), function () {
        b.disabled = true;
        api('POST', '/api/download', {key: item.key}, function (err, job) {
          b.disabled = false;
          if (err) { notice(err); return; }
          if (job.status === 'completed') { item.downloaded = true; fileAction(item, action); }
          notice(job.status === 'completed' ? I18n.t("This book is already saved to Kindle.") : I18n.t("Added to downloads. Check progress in the Downloads tab.")); loadJobs();
        });
      }); action.appendChild(b);
    } else { action.appendChild(make('span', 'unsupported', /\.epub$/i.test(item.key) ? I18n.t("Convert first") : I18n.t("Not supported"))); }
  }
  function loadFiles() {
    var request = ++generation; busy = true; next = ''; el('refresh').disabled = true; el('prev').disabled = true; el('next').disabled = true;
    text(el('path'), '/' + prefix); text(el('page'), I18n.t("Page ") + (page + 1) + I18n.t(" ")); el('up').disabled = prefix === root;
    visibleFiles = []; el('files').innerHTML = ''; el('files').appendChild(make('p', 'empty', I18n.t("Loading cloud library…")));
    el('pagination').hidden = page === 0;
    api('GET', '/api/objects?prefix=' + encodeURIComponent(prefix) + '&marker=' + encodeURIComponent(markers[page]), null, function (err, data) {
      if (request !== generation) { return; }
      busy = false; el('refresh').disabled = false; el('prev').disabled = page === 0; el('files').innerHTML = '';
      if (err) { el('files').appendChild(make('p', 'empty', err)); return; }
      var i, folders = data.common_prefixes || [], files = data.keys || [];
      for (i = 0; i < folders.length; i++) { addRow(el('files'), folders[i], true); }
      for (i = 0; i < files.length; i++) { addRow(el('files'), files[i], false); }
      if (!folders.length && !files.length) { el('files').appendChild(make('p', 'empty', I18n.t("No files on this page. Choose another folder or check the next page."))); }
      next = data.next_marker || ''; el('next').disabled = !next;
      el('pagination').hidden = page === 0 && !next;
      loadJobs();
    });
  }
  function jobAction(job, action) {
    api('POST', '/api/jobs/action', {id: job.id, action: action}, function (err) { if (err) { notice(err); } else { notice(''); } loadJobs(); });
  }
  function renderJob(job, parent) {
    var block = make('div', 'job'); block.appendChild(make('p', 'name', job.key.split('/').pop()));
    var percent = job.size > 0 ? Math.min(100, Math.floor(job.downloaded / job.size * 100)) : 0;
    if (job.status === 'completed') { percent = 100; }
    var bar = make('div', 'progress'), fill = make('span'); fill.style.width = percent + '%'; bar.appendChild(fill); block.appendChild(bar);
    var summary = make('div', 'job-summary');
    var label = statuses[job.status] || job.status;
    if (job.status === 'completed') {
      var local = job.local_state || (job.saved_locally ? 'saved' : 'removed');
      label = I18n.t("Transfer complete · ") + ({saved: I18n.t("Saved to Kindle"), removed: I18n.t("Local file removed"), changed: I18n.t("Local file changed"), unavailable: I18n.t("Cannot inspect local file")}[local] || I18n.t("Waiting to inspect local file"));
    } else if (job.transfer_complete) {
      label = I18n.t("Transfer complete · ") + (job.status === 'downloading' ? I18n.t("Saving") : I18n.t("Not saved yet"));
    }
    summary.appendChild(make('div', 'status', label + ' · ' + size(job.downloaded) + ' / ' + size(job.size) + ' · ' + percent + '%'));
    block.appendChild(summary);
    if (job.error) { block.appendChild(make('p', 'job-error', job.error)); }
    if (job.status === 'completed') {
      // Use a button instead of native <details> for the Kindle's old WebKit.
      var details = make('p', 'meta job-details', I18n.t("Saved filename: ") + job.name);
      details.id = 'job-details-' + job.id;
      var toggle = button(I18n.t("Details"), function () {
        expandedJobs[job.id] = !expandedJobs[job.id]; updateDetails();
      }, 'small');
      toggle.setAttribute('aria-controls', details.id);
      var updateDetails = function () {
        var open = !!expandedJobs[job.id];
        details.hidden = !open; text(toggle, open ? I18n.t("Collapse") : I18n.t("Details"));
        toggle.setAttribute('aria-expanded', open ? 'true' : 'false');
      };
      updateDetails();
      var detailAction = make('div', 'job-details-action'); detailAction.appendChild(toggle);
      summary.appendChild(detailAction); block.appendChild(details);
    }
    var actions = make('div', 'job-actions');
    if (job.status === 'completed' && job.local_state === 'removed') { actions.appendChild(button(I18n.t("Download again"), function () {
      api('POST', '/api/download', {key: job.key}, function (err) { notice(err || I18n.t("Added to download queue")); loadJobs(); });
    }, 'small')); }
    if (job.status === 'failed' || job.status === 'paused' || job.status === 'cancelled') { actions.appendChild(button(I18n.t("Resume"), function () { jobAction(job, 'retry'); }, 'small')); }
    if (job.status === 'downloading' || job.status === 'queued') { actions.appendChild(button(I18n.t("Pause"), function () { jobAction(job, 'cancel'); }, 'small')); }
    if (actions.childNodes.length) { block.appendChild(actions); }
    parent.appendChild(block);
  }
  function loadJobs() {
    var request = ++jobsGeneration;
    if (poll) { clearTimeout(poll); poll = null; }
    api('GET', '/api/jobs', null, function (err, jobs) {
      if (request !== jobsGeneration) { return; }
      if (err) { if (activeTab === 'downloads') { notice(err); } return; }
      var i, k, row, running = false, active = 0, nextExpanded = {}; el('jobs').innerHTML = '';
      for (i = jobs.length - 1; i >= 0; i--) {
        if (jobs[i].status === 'completed' && expandedJobs[jobs[i].id]) { nextExpanded[jobs[i].id] = true; }
        renderJob(jobs[i], el('jobs'));
        for (k = 0; k < visibleFiles.length; k++) {
          row = visibleFiles[k];
          if (row.item.key === jobs[i].key && row.item.downloaded !== jobs[i].saved_locally) {
            row.item.downloaded = jobs[i].saved_locally; fileAction(row.item, row.action);
          }
        }
        if (jobs[i].status === 'queued' || jobs[i].status === 'downloading') { running = true; active++; }
      }
      expandedJobs = nextExpanded;
      text(el('count'), active);
      if (!jobs.length) { el('jobs').appendChild(make('p', 'empty', I18n.t("No downloads yet. Choose a book from the cloud library."))); }
      if (running) { poll = setTimeout(loadJobs, 4000); }
    });
  }
  el('library-tab').onclick = function () { tab('library'); };
  el('about-toggle').onclick = function () {
    var open = el('about-text').hidden;
    el('about-text').hidden = !open;
    el('about-toggle').setAttribute('aria-expanded', open ? 'true' : 'false');
    text(el('about-toggle'), open ? I18n.t("Collapse") : I18n.t("About"));
  };
  el('downloads-tab').onclick = function () { tab('downloads'); };
  el('refresh').onclick = function () { if (!busy) { notice(''); loadFiles(); } };
  el('refresh-jobs').onclick = loadJobs;
  el('up').onclick = function () { var p = prefix.replace(/\/$/, ''); p = p.slice(0, p.lastIndexOf('/') + 1); navigate(p.length < root.length ? root : p); };
  el('prev').onclick = function () { if (!busy && page > 0) { page--; loadFiles(); } };
  el('next').onclick = function () { if (!busy && next) { markers[++page] = next; loadFiles(); } };
  el('clear').onclick = function () { api('POST', '/api/jobs/clear', {}, function (err) { if (err) { notice(err); } loadJobs(); }); };
  var match = /(?:^#|&)token=([a-f0-9]{64})(?:&|$)/.exec(window.location.hash);
  if (!match) { notice(I18n.t("Open from KUAL → InkPika; this link has no local session.")); text(el('bucket'), I18n.t("Waiting to open from KUAL")); text(el('files'), ''); return; }
  token = match[1];
  api('GET', '/api/info', null, function (err, info) {
    if (err) { notice(err); return; }
    text(el('bucket'), info.provider_label + ' · ' + info.bucket);
    text(el('space'), I18n.t("Free ") + size(info.free_bytes));
    root = info.prefix; prefix = root; loadFiles();
  });
}());

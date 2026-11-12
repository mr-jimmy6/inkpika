// Local demo regression checks; Node is a developer tool, never a device dependency.
const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const root = path.resolve(__dirname, '../..');

function setup(language = '') {
  let now = 0, id = 0;
  const pending = new Map();
  const context = vm.createContext({
    window: {location: {search: language ? '?lang=' + language : ''}, navigator: {language: 'zh-CN', languages: ['zh-CN']}}, atob, Blob, URL, Uint8Array, DataView,
    setTimeout(fn, delay) { pending.set(++id, {fn, due: now + delay}); return id; },
    clearTimeout(key) { pending.delete(key); }
  });
  for (const file of ['../assets/i18n.js', 'sample-data.js', 'model.js']) {
    vm.runInContext(fs.readFileSync(path.join(root, 'site/preview', file), 'utf8'), context);
  }
  const model = context.window.inkpikaDemo;
  function advance(ms) {
    const end = now + ms;
    for (;;) {
      const entry = [...pending].filter(([, v]) => v.due <= end).sort((a, b) => a[1].due - b[1].due)[0];
      if (!entry) break;
      pending.delete(entry[0]); now = entry[1].due; entry[1].fn();
    }
    now = end;
  }
  function api(screen, method, url, body = null, headers = {}) {
    const type = screen.split('-')[0];
    return model.api(screen, method, url, body, {'x-transfer-token': type + '-' + model.state.pairVersion[type], ...headers});
  }
  return {model, advance, api};
}

test('stopping and restarting sync retains completed files without duplication', () => {
  const {model, advance} = setup();
  model.sync('start'); advance(2200); model.sync('cancel'); advance(20000);
  assert.equal(Object.keys(model.state.saved).length, 1);
  assert.equal(model.state.syncPhase, 'cancelled');
  model.sync('start'); advance(10000);
  assert.equal(Object.keys(model.state.saved).length, 3);
  assert.equal(model.state.local.length, 6);
  assert.equal(model.state.syncPhase, 'after');
  model.sync('start'); advance(10000);
  assert.equal(model.state.local.length, 6);
  assert.equal(model.state.syncSkipped, 3);
});

test('reset cancels pending work; service stops preserve books and isolate pairing', () => {
  const {model, advance, api} = setup();
  model.service('wireless', true); model.service('settings', true);
  const old = 'wireless-' + model.state.pairVersion.wireless;
  model.service('wireless', false); model.service('wireless', true);
  assert.equal(api('wireless', 'GET', '/api/books', null, {'x-transfer-token': old}).status, 401);
  assert.equal(api('settings', 'GET', '/api/config').status, 200);
  model.sync('start'); advance(500); model.reset(); advance(30000);
  assert.equal(model.state.local.length, 3);
  assert.equal(Object.keys(model.state.saved).length, 0);
  assert.equal(model.state.services.cloud, false);
});

test('pause/resume and clearing history preserve incomplete jobs and completed books', () => {
  const {model, advance, api} = setup();
  model.service('cloud', true);
  const job = api('cloud', 'POST', '/api/download', {key: model.books[0].name}).data;
  advance(900);
  api('cloud', 'POST', '/api/jobs/action', {id: job.id, action: 'cancel'});
  const progress = model.state.jobs[0].downloaded; advance(30000);
  assert.equal(model.state.jobs[0].downloaded, progress);
  api('cloud', 'POST', '/api/jobs/clear', {}); assert.equal(model.state.jobs.length, 1);
  api('cloud', 'POST', '/api/jobs/action', {id: job.id, action: 'retry'}); advance(10000);
  assert.equal(model.state.jobs[0].status, 'completed');
  api('cloud', 'POST', '/api/jobs/clear', {});
  assert.equal(model.state.jobs.length, 0); assert.equal(model.state.local.length, 4);
  assert.equal(api('cloud', 'GET', '/api/objects?prefix=&marker=').data.keys[0].downloaded, true);
});

test('configuration rejects stale saves, shares format filtering, masks credentials and retains local files', () => {
  const {model, api} = setup(); model.service('settings', true); model.service('wireless', true);
  const cfg = api('settings', 'GET', '/api/config').data;
  cfg.access_key_id = 'sample-id-value'; cfg.secret_access_key = 'sample-secret-value'; cfg.security_token = '';
  cfg.bucket = 'another-demo'; cfg.ebook_formats = ['PDF'];
  assert.equal(api('settings', 'POST', '/api/config', cfg).status, 200);
  assert.equal(api('settings', 'POST', '/api/config', cfg).status, 400);
  assert.equal(api('wireless', 'GET', '/api/books').data.total, 0);
  assert.equal(model.state.local.length, 3);
  assert.ok(!JSON.stringify(model.state).includes('sample-secret-value'));
  assert.match(model.menu()[0].name, /0\/0/);
});

test('upload duplicates are renamed and exported ZIP bytes match fixtures', async () => {
  const {model, api} = setup(); model.service('wireless', true);
  const book = model.books[0];
  const uploaded = api('wireless', 'POST', '/api/upload', {bytes: book.bytes}, {'x-file-name': encodeURIComponent(book.name)});
  assert.equal(uploaded.status, 201); assert.notEqual(uploaded.data.name, book.name);
  const ids = model.state.local.slice(0, 2).map(b => b.id);
  const exported = api('wireless', 'POST', '/api/export', {ids}).data;
  const zip = Buffer.from(await (await fetch(exported.url)).arrayBuffer());
  let offset = 0;
  for (let index = 0; index < 2; index++) {
    assert.equal(zip.readUInt32LE(offset), 0x04034b50);
    const size = zip.readUInt32LE(offset + 18), nameLength = zip.readUInt16LE(offset + 26);
    const dataStart = offset + 30 + nameLength;
    assert.deepEqual(zip.subarray(dataStart, dataStart + size), Buffer.from(model.books[index].bytes));
    offset = dataStart + size;
  }
  assert.equal(zip.readUInt32LE(offset), 0x02014b50);
  assert.equal(zip.readUInt32LE(zip.length - 22), 0x06054b50);
  model.reset();
});

test('S3 presets and proxy settings are displayed without retaining proxy credentials', () => {
  const {model, api} = setup(); model.service('settings', true); model.service('cloud', true);
  const cfg = api('settings', 'GET', '/api/config').data;
  Object.assign(cfg, {provider: 's3', s3_preset: 'r2', region: 'auto', endpoint: 'https://example.r2.cloudflarestorage.com', socks5_proxy: 'socks5://demo:sample-proxy-secret@proxy.example:1080'});
  assert.equal(api('settings', 'POST', '/api/config', cfg).status, 200);
  assert.equal(api('cloud', 'GET', '/api/info').data.provider_label, 'Cloudflare R2');
  assert.equal(api('settings', 'GET', '/api/config').data.socks5_proxy_set, true);
  assert.ok(!JSON.stringify(model.state).includes('sample-proxy-secret'));
  const next = api('settings', 'GET', '/api/config').data;
  next.socks5_proxy = ''; next.clear_socks5_proxy = true;
  assert.equal(api('settings', 'POST', '/api/config', next).status, 200);
  assert.equal(api('cloud', 'GET', '/api/config').data.connection, 'Direct');
});

test('supplier forms preserve the single platform model and enforce provider constraints', () => {
  for (const [provider, preset, region, bucket] of [
    ['s3', 'tos', 'cn-beijing', 'books'], ['s3', 'ibm', 'us-south', 'books'],
    ['s3', 'gcs', '', 'books'], ['s3', 'cos', 'ap-guangzhou', 'books-1250000000'],
    ['azure_blob', '', '', 'books']
  ]) {
    const {model, api} = setup(); model.service('settings', true);
    const cfg = {...api('settings', 'GET', '/api/config').data, provider, s3_preset: preset, region, bucket,
      endpoint: '', access_key_id: 'bookaccount', secret_access_key: Buffer.alloc(64).toString('base64'), clear_security_token: true};
    assert.equal(api('settings', 'POST', '/api/config', cfg).status, 200, preset || provider);
    const saved = api('settings', 'GET', '/api/config').data;
    assert.equal(saved.provider, provider); assert.equal(saved.s3_preset, preset);
    assert.equal(saved.region, preset === 'gcs' ? 'auto' : region);
    assert.equal(saved.s3_addressing, provider === 'azure_blob' ? '' : ['tos', 'cos'].includes(preset) ? 'virtual' : 'path');
    assert.equal(saved.secret_access_key, undefined);
    if (provider === 'azure_blob') {
      assert.equal(api('settings', 'POST', '/api/config', {...cfg, revision: saved.revision, secret_access_key: 'invalid'}).status, 400);
    }
  }
});


test('explicit page language and device override preserve sample books and pairing', () => {
  for (const initial of ['', 'zh']) {
    const {model, api} = setup(initial);
    assert.equal(model.language(), initial || 'en');
    assert.equal(model.menu()[1].name, initial === 'zh' ? '开始同步' : 'Start sync');
    model.service('settings', true);
    const names = model.state.local.map(book => book.name);
    const pairing = model.state.pairVersion.settings;
    const config = api('settings', 'GET', '/api/config').data;
    const target = initial === 'zh' ? 'en' : 'zh';
    assert.equal(api('settings', 'POST', '/api/config', {revision: config.revision, language: target, language_only: true}).status, 200);
    assert.equal(model.language(), target);
    assert.equal(model.menu()[1].name, target === 'zh' ? '开始同步' : 'Start sync');
    assert.equal(model.state.pairVersion.settings, pairing);
    assert.deepEqual(model.state.local.map(book => book.name), names);
    const reloaded = api('settings', 'GET', '/api/config').data;
    assert.equal(reloaded.effective_language, target);
    assert.equal(reloaded.provider, config.provider);
    assert.equal(reloaded.bucket, config.bucket);
  }
});

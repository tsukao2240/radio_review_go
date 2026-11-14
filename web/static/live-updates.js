(() => {
  'use strict';
  if (window.radioLive) return;
  const nonce = document.querySelector('script[nonce]')?.nonce;
  const originalFetch = window.fetch.bind(window);
  let busy = false, pending = false, source, fallback;
  const editing = () => document.activeElement?.matches('main input, main textarea, main select') || [...document.querySelectorAll('main input[type=file]')].some(input => input.files.length);
  async function refresh() {
    pending = true;
    if (busy || editing() || document.hidden) return;
    busy = true;
    pending = false;
    try {
      const response = await originalFetch(location.href, {cache: 'no-store', headers: {'X-Live-Refresh': '1'}});
      if (!response.ok) throw new Error('refresh failed');
      const page = new DOMParser().parseFromString(await response.text(), 'text/html');
      const oldMain = document.querySelector('main.main-content');
      const nextMain = page.querySelector('main.main-content');
      if (!oldMain || !nextMain) return;
      if (editing()) { pending = true; return; }
      const scripts = [...nextMain.querySelectorAll('script')];
      scripts.forEach(s => s.remove());
      // Preserve unsent edits even when focus moved to another control.
      oldMain.querySelectorAll('input:not([type=hidden]):not([type=file]), textarea, select').forEach(input => {
        const dirty = input.tagName === 'SELECT'
          ? [...input.options].some(o => o.selected !== o.defaultSelected)
          : /^(checkbox|radio)$/.test(input.type) ? input.checked !== input.defaultChecked : input.value !== input.defaultValue;
        if (!dirty || (!input.id && !input.name)) return;
        const replacement = input.id ? nextMain.querySelector('#' + CSS.escape(input.id))
          : [...nextMain.querySelectorAll(input.tagName)].find(el => el.name === input.name && el.type === input.type);
        if (!replacement) return;
        replacement.value = input.value;
        if (/^(checkbox|radio)$/.test(input.type)) replacement.checked = input.checked;
      });
      const token = page.querySelector('meta[name="csrf-token"]')?.content;
      if (token) document.querySelector('meta[name="csrf-token"]').content = token;
      // Move existing audio players so updates do not interrupt playback or seeking.
      oldMain.querySelectorAll('.audio-player-container[id]').forEach(player => {
        const replacement = nextMain.querySelector('#' + CSS.escape(player.id));
        if (replacement) replacement.replaceWith(player);
      });
      oldMain.querySelectorAll('a[data-downloading]').forEach(link => {
        const replacement = [...nextMain.querySelectorAll('a')].find(a => a.getAttribute('href') === link.getAttribute('href'));
        if (replacement) { replacement.dataset.downloading = 'true'; replacement.textContent = link.textContent; }
      });
      const y = scrollY;
      oldMain.replaceWith(nextMain);
      scripts.forEach(oldScript => {
        if (oldScript.src) return;
        const script = document.createElement('script');
        script.nonce = nonce;
        script.textContent = '(function(){\n' + oldScript.textContent.replace(/document\.addEventListener\(['"]DOMContentLoaded['"],/g, 'window.radioLive.onReady(') + '\n})();';
        nextMain.appendChild(script);
        script.remove();
      });
      scrollTo(scrollX, y);
      document.dispatchEvent(new CustomEvent('radio:updated'));
    } catch (_) {
      pending = true;
    } finally {
      busy = false;
      if (pending && !editing() && !document.hidden) setTimeout(refresh, 250);
    }
  }
  window.radioLive = {refresh, onReady: callback => callback()};
  window.fetch = async function(input, options) {
    const response = await originalFetch(input, options);
    const method = (options?.method || input?.method || 'GET').toUpperCase();
    if (response.ok && !['GET', 'HEAD'].includes(method)) setTimeout(refresh, 0);
    return response;
  };
  function connect() {
    if (source || document.hidden) return;
    source = new EventSource('/api/live');
    source.addEventListener('changed', refresh);
    source.addEventListener('ready', () => {
      clearInterval(fallback); fallback = null;
      refresh();
    });
    source.onerror = () => { if (!fallback) fallback = setInterval(refresh, 3000); };
  }
  document.addEventListener('visibilitychange', () => {
    if (document.hidden) { source?.close(); source = null; clearInterval(fallback); fallback = null; }
    else { connect(); refresh(); }
  });
  document.addEventListener('focusout', () => { if (pending) setTimeout(refresh, 0); });
  window.addEventListener('online', () => {connect(); refresh();});
  // Open the native save dialog in the click gesture, before requesting audio.
  document.addEventListener('click', async event => {
    const link = event.target.closest('a[href^="/recording/download?"]');
    if (!link || event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
    event.preventDefault(); event.stopImmediatePropagation();
    if (link.dataset.downloading) return;
    link.dataset.downloading = 'true';
    const label = link.innerHTML;
    const id = new URL(link.href).searchParams.get('recording_id');
    const suggestedName = (link.dataset.downloadName || 'recording.m4a').split(/[\\/]/).pop();
    let saved = false;
    function setLabel(text) {
      link.textContent = text;
      document.querySelectorAll('a[data-downloading]').forEach(a => { if (a.href === link.href) a.textContent = text; });
    }
    try {
      setLabel('保存先を確認中…');
      let handle = await window.radioDownloadSettings?.createTarget(suggestedName);
      if (!handle && typeof window.showSaveFilePicker === 'function') {
        setLabel('保存先を選択中…');
        handle = await window.showSaveFilePicker({suggestedName,
          types: [{description: '録音音声', accept: {'audio/mp4': ['.m4a'], 'audio/aac': ['.aac']}}]});
      }
      setLabel('保存中…');
      const response = await originalFetch(link.href, {cache: 'no-store'});
      if (!response.ok) throw new Error('ダウンロードに失敗しました');
      const receipt = response.headers.get('X-Download-Receipt');
      if (handle) {
        if (!receipt || !response.body) throw new Error('保存の準備を確認できませんでした');
        const writable = await handle.createWritable();
        // pipeTo resolves only after the destination has successfully closed.
        await response.body.pipeTo(writable);
        saved = true;
        setLabel('保存済み・一覧更新中…');
        let acknowledgement;
        for (let attempt = 0; attempt < 6; attempt++) {
          acknowledgement = await originalFetch('/recording/download/complete', {
            method: 'POST', headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({recording_id: id, receipt})
          });
          if (acknowledgement.status !== 409) break;
          await new Promise(resolve => setTimeout(resolve, 50 * (attempt + 1)));
        }
        if (!acknowledgement.ok) throw new Error('保存は完了しましたが、元の録音を削除できませんでした');
        await refresh();
      } else {
        // Standard browser downloads cannot report save/cancel: retain the original.
        const disposition = response.headers.get('Content-Disposition') || '';
        const match = /filename\*=utf-8''([^;]+)/i.exec(disposition);
        const name = match ? decodeURIComponent(match[1]) : suggestedName;
        const url = URL.createObjectURL(await response.blob());
        const save = document.createElement('a');
        save.href = url; save.download = name; document.body.appendChild(save); save.click(); save.remove();
        setTimeout(() => URL.revokeObjectURL(url), 60000);
        window.toast?.info?.('保存後、不要な録音は「削除」で消せます');
      }
    } catch (error) {
      if (error.name !== 'AbortError') window.toast?.error(saved ? '保存は完了しましたが、元の録音を削除できませんでした' : error.message);
    } finally {
      document.querySelectorAll('a[data-downloading]').forEach(a => {
        if (a.href === link.href) {delete a.dataset.downloading; a.innerHTML = label;}
      });
      delete link.dataset.downloading;
      if (link.isConnected) link.innerHTML = label;
    }
  }, true);

  connect();
})();

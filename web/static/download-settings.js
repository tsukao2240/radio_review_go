(() => {
  'use strict';
  if (window.radioDownloadSettings) return;
  const dbName = 'radio-review-preferences';
  let preference = null, problem = '', busy = false;
  const listeners = typeof BroadcastChannel === 'function' ? new BroadcastChannel('radio-download-settings') : null;
  async function database() {
    return new Promise((resolve, reject) => {
      const request = indexedDB.open(dbName, 1);
      request.onupgradeneeded = () => request.result.createObjectStore('preferences');
      request.onsuccess = () => resolve(request.result);
      request.onerror = () => reject(request.error);
    });
  }
  async function read() {
    const db = await database();
    try {
      return await new Promise((resolve, reject) => {
        const request = db.transaction('preferences').objectStore('preferences').get('download-directory');
        request.onsuccess = () => resolve(request.result || null);
        request.onerror = () => reject(request.error);
      });
    } finally { db.close(); }
  }
  async function persist(value) {
    const db = await database();
    try {
      await new Promise((resolve, reject) => {
        const tx = db.transaction('preferences', 'readwrite');
        const store = tx.objectStore('preferences');
        if (value) store.put(value, 'download-directory'); else store.delete('download-directory');
        tx.oncomplete = resolve;
        tx.onerror = () => reject(tx.error);
        tx.onabort = () => reject(tx.error);
      });
      preference = value;
      listeners?.postMessage('changed');
    } finally { db.close(); }
  }
  function render() {
    const folder = document.getElementById('download-folder-name');
    if (!folder) return;
    folder.textContent = preference?.handle?.name || '未設定（毎回、保存先を選択）';
    const supported = typeof window.showDirectoryPicker === 'function';
    document.querySelectorAll('[data-download-setting-action]').forEach(button => {
      button.disabled = busy || (button.dataset.downloadSettingAction === 'choose' && !supported);
    });
    const status = document.getElementById('download-settings-status');
    status.textContent = problem || (!supported ? 'このブラウザではフォルダ指定を利用できません。' : preference ? '指定フォルダに直接保存します。同名ファイルには番号を付けます。' : 'フォルダを選ぶと、次回から保存先の選択を省略できます。');
  }
  const ready = read().then(value => { preference = value; render(); }).catch(() => {
    problem = '保存先設定を読み込めません。設定を確認してください。'; render();
  });
  let allocation = Promise.resolve();
  async function reserve(directory, name) {
    const ext = name.includes('.') ? name.slice(name.lastIndexOf('.')) : '';
    const stem = ext ? name.slice(0, -ext.length) : name;
    for (let number = 1; number <= 10000; number++) {
      const candidate = number === 1 ? name : `${stem} (${number})${ext}`;
      try { await directory.getFileHandle(candidate); }
      catch (error) {
        if (error.name === 'TypeMismatchError') continue;
        if (error.name !== 'NotFoundError') throw error;
        return directory.getFileHandle(candidate, {create: true});
      }
    }
    throw new Error('同名ファイルが多いため保存できません');
  }
  async function createTarget(name) {
    await ready;
    if (problem) throw new Error(problem);
    const directory = preference?.handle;
    if (!directory) return null;
    const options = {mode: 'readwrite'};
    let permission = await directory.queryPermission(options);
    if (permission !== 'granted') permission = await directory.requestPermission(options);
    if (permission !== 'granted') throw new Error('保存先フォルダへの書き込みを許可してください。設定画面から変更できます。');
    const allocate = () => reserve(directory, name);
    const operation = allocation.catch(() => {}).then(() => navigator.locks
      ? navigator.locks.request('radio-download-destination', allocate)
      : allocate());
    allocation = operation;
    return operation;
  }
  window.radioDownloadSettings = {createTarget, ready};
  document.addEventListener('click', async event => {
    const button = event.target.closest('[data-download-setting-action]');
    if (!button || busy) return;
    event.preventDefault(); busy = true; problem = ''; render();
    try {
      if (button.dataset.downloadSettingAction === 'choose') {
        const handle = await window.showDirectoryPicker({id: 'radio-recordings', mode: 'readwrite'});
        await persist({handle});
      } else {
        await persist(null);
      }
    } catch (error) {
      if (error.name !== 'AbortError') problem = '保存先を設定できませんでした。もう一度選択してください。';
    } finally { busy = false; render(); }
  });
  document.addEventListener('radio:updated', render);
  document.addEventListener('DOMContentLoaded', render);
  if (listeners) listeners.onmessage = async () => {
    try { preference = await read(); problem = ''; render(); } catch (_) {}
  };
})();

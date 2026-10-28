// Copyright (c) 2026 AllIsA1. SPDX-License-Identifier: MIT
(function () {
  'use strict';

  var TEXT = {
    en: {
      title: 'Better WARP',
      intro:
        'Cloudflare WARP as a protocol of your subscriptions, over AmneziaWG: every subscriber gets WARP nodes in the same four forms as an AmneziaWG node (vpn://, amneziawg://, the older amneziawg:// link and .conf). The traffic goes from the client straight to Cloudflare and does not pass this server or its traffic counters.',
      engineMissing: function (e) { return 'The WARP engine of Better Cores is not available: ' + e + ' Checking and scanning need it; the subscription lines work without it.'; },
      inboundsTitle: 'WARP inbounds',
      inboundsNote:
        'The settings of the WARP nodes, and who gets them, are in Inbounds: add an inbound of the WARP protocol and attach clients to it. This page keeps the account, the devices and the scanner.',
      inboundsOn: 'on',
      inboundsOff: 'off',
      inboundSubs: function (n) { return n + (n === 1 ? ' subscription' : ' subscriptions'); },
      inboundsMissing: 'This plugin cannot read the panel’s inbounds, so it uses its own settings below. Reinstall or update it and confirm all its permissions.',
      defaultEndpoints: 'Default endpoints',
      defaultEndpointsHint: 'Used by WARP inbounds that name no endpoint of their own. A scan keeps them at the quickest ones.',
      protocol: 'WARP in subscriptions',
      enabled: 'Give WARP to subscribers',
      enabledHint: 'Off: subscriptions stay exactly as the panel makes them.',
      mode: 'Accounts',
      modePer: 'A device of its own for each subscription (recommended)',
      modeShared: 'One device for all subscriptions',
      modeHint: 'A device is a free anonymous WARP registration. Own devices do not disturb each other; one shared device is simpler, and the only way for WARP+ to apply.',
      scope: 'Who gets it',
      scopeAll: 'Every active subscription',
      scopeSel: 'Only the selected subscriptions',
      name: 'Name of the node',
      account: 'Main account',
      accountHint: 'The device that checks and scans use, and that every subscription gets in shared mode.',
      notRegistered: 'Not registered.',
      register: 'Register at Cloudflare',
      registerNote: 'This sends an anonymous registration to Cloudflare from this server.',
      accountId: 'Device',
      accountType: 'Type',
      free: 'WARP (free)',
      plus: 'WARP+',
      quota: 'WARP+ data left',
      refresh: 'Refresh',
      licenseLabel: 'WARP+ license key',
      licensePlaceholder: 'xxxxxxxx-xxxxxxxx-xxxxxxxx',
      applyLicense: 'Apply',
      licenseApplied: 'A license key is applied.',
      deleteAccount: 'Delete the account',
      confirmDeleteAccount: 'Delete the main account at Cloudflare?',
      devices: 'Devices of subscriptions',
      devicesCount: function (have, wanted) { return have + ' ready' + (wanted ? ', ' + wanted + ' waiting' : ''); },
      devicesRetry: function (t) { return 'Next try ' + t; },
      issueNow: 'Issue now',
      resetDevices: 'Delete all devices',
      confirmResetDevices: 'Delete every subscription device? Subscribers get new devices (new keys) and must import the subscription again.',
      settings: 'Connection',
      endpoints: 'Endpoints (one node each, at most 3)',
      endpointHint: 'host:port. engage.cloudflareclient.com:2408 is the default; a scan finds quicker ones.',
      addEndpoint: 'Add endpoint',
      mtu: 'MTU',
      dns: 'DNS',
      keepalive: 'Keepalive, s',
      ipv6: 'IPv6 inside the tunnel',
      preset: 'Junk packets',
      presetStandard: 'Standard (Jc 4, 40–70)',
      presetLight: 'Light (Jc 2, 20–40)',
      presetStrong: 'Strong (Jc 8, 50–150)',
      presetCustom: 'Custom',
      advanced: 'AmneziaWG fields',
      advancedHint: 'WARP speaks plain WireGuard: only the junk packets (Jc, Jmin, Jmax) are safe. S1, S2 and H1–H4 other than 0 and the defaults make Cloudflare drop the handshake.',
      save: 'Save',
      saved: 'Saved.',
      endpointsTitle: 'Speed: check and scan',
      endpointsIntro:
        'Cloudflare answers on many addresses and ports and they are not equally quick. A scan asks them with a real handshake from this server and ranks them; with “download” it also measures speed through the best few. It shows what this server sees, which can differ from what a client far away sees.',
      check: 'Check',
      checkSpeed: 'with a download',
      checkEndpoint: 'Endpoint',
      scan: 'Scan',
      scanSpeed: 'download through the best',
      scanV6: 'IPv6',
      stop: 'Stop',
      working: 'Working…',
      stage: { probe: 'asking addresses', measure: 'measuring the quickest', speed: 'downloading', check: 'checking', done: 'done' },
      resultOk: 'WARP works',
      resultBad: 'Does not work',
      handshake: 'handshake',
      latency: 'round trip',
      exit: 'exit',
      speed: 'speed',
      colEndpoint: 'Endpoint',
      colRtt: 'RTT, ms',
      colLoss: 'Loss',
      colSpeed: 'Mbit/s',
      use: 'Use',
      useBest: 'Use the best ',
      applied: 'The subscription now has these endpoints.',
      nothingFound: 'No endpoint answered. The account may be new for Cloudflare (try again in a minute), or the server cannot reach WARP.',
      lastScan: 'Last scan',
      autoScan: 'Scan by itself',
      autoEvery: 'every, hours',
      autoKeep: 'keep the best',
      autoSpeed: 'with a download',
      autoApplied: 'applied',
      autoNote: 'The best endpoints of a scheduled scan replace the subscription’s endpoints.',
      subs: 'Subscriptions',
      subsSearch: 'Email or subscription id',
      subsFind: 'Find',
      subsNone: 'No subscriptions.',
      subsNotLoaded: 'The panel’s clients have not been read yet.',
      colSub: 'Subscription',
      colOn: 'WARP',
      colState: 'State',
      active: 'active',
      inactive: 'off / expired',
      hasDevice: 'device ready',
      noDevice: 'no device yet',
      selectNote: 'Switch “Who gets it” to “Only the selected” to choose.',
      preview: 'What a subscriber gets',
      previewNote: 'The first form is the config; the subscription also carries the same node as vpn:// and amneziawg://. Keys are the subscriber’s own.',
      failed: function (e) { return 'Failed: ' + e; },
      yes: 'yes',
      no: 'no',
    },
    ru: {
      title: 'Better WARP',
      intro:
        'Cloudflare WARP как протокол ваших подписок, поверх AmneziaWG: каждый подписчик получает узлы WARP в тех же четырёх формах, что и узел AmneziaWG (vpn://, amneziawg://, старая ссылка amneziawg:// и .conf). Трафик идёт от клиента прямо в Cloudflare и не проходит через этот сервер и его счётчики.',
      engineMissing: function (e) { return 'Движок WARP из Better Cores недоступен: ' + e + ' Проверка и сканер без него не работают; строки подписки работают.'; },
      inboundsTitle: 'WARP-инбаунды',
      inboundsNote:
        'Настройки узлов WARP и то, кому они достаются, — в разделе Inbounds: добавьте inbound протокола WARP и привяжите к нему клиентов. Здесь — аккаунт, устройства и сканер.',
      inboundsOn: 'включён',
      inboundsOff: 'выключен',
      inboundSubs: function (n) { var m = n % 10, h = n % 100; return n + ' ' + (m === 1 && h !== 11 ? 'подписка' : m >= 2 && m <= 4 && (h < 12 || h > 14) ? 'подписки' : 'подписок'); },
      inboundsMissing: 'Плагин не может прочитать inbounds панели и пользуется своими настройками ниже. Переустановите или обновите его и подтвердите все права.',
      defaultEndpoints: 'Endpoints по умолчанию',
      defaultEndpointsHint: 'Берут WARP-инбаунды, у которых свои endpoints не заданы. Сканер держит их на самых быстрых.',
      protocol: 'WARP в подписках',
      enabled: 'Выдавать WARP подписчикам',
      enabledHint: 'Выключено: подписки остаются ровно такими, какими их делает панель.',
      mode: 'Аккаунты',
      modePer: 'Своё устройство на каждую подписку (рекомендуется)',
      modeShared: 'Одно устройство на все подписки',
      modeHint: 'Устройство — бесплатная анонимная регистрация WARP. Свои устройства не мешают друг другу; общее проще и только оно позволяет применить WARP+.',
      scope: 'Кому выдавать',
      scopeAll: 'Всем активным подпискам',
      scopeSel: 'Только выбранным подпискам',
      name: 'Название узла',
      account: 'Основной аккаунт',
      accountHint: 'Устройство для проверки и сканера; в общем режиме его получают все подписки.',
      notRegistered: 'Не зарегистрирован.',
      register: 'Зарегистрировать в Cloudflare',
      registerNote: 'С этого сервера уйдёт анонимная регистрация в Cloudflare.',
      accountId: 'Устройство',
      accountType: 'Тип',
      free: 'WARP (бесплатный)',
      plus: 'WARP+',
      quota: 'Осталось трафика WARP+',
      refresh: 'Обновить',
      licenseLabel: 'Лицензионный ключ WARP+',
      licensePlaceholder: 'xxxxxxxx-xxxxxxxx-xxxxxxxx',
      applyLicense: 'Применить',
      licenseApplied: 'Ключ применён.',
      deleteAccount: 'Удалить аккаунт',
      confirmDeleteAccount: 'Удалить основной аккаунт в Cloudflare?',
      devices: 'Устройства подписок',
      devicesCount: function (have, wanted) { return 'готово: ' + have + (wanted ? ', ждут: ' + wanted : ''); },
      devicesRetry: function (t) { return 'Следующая попытка ' + t; },
      issueNow: 'Выдать сейчас',
      resetDevices: 'Удалить все устройства',
      confirmResetDevices: 'Удалить устройства всех подписок? Подписчики получат новые устройства (новые ключи) и должны заново импортировать подписку.',
      settings: 'Подключение',
      endpoints: 'Endpoints (по узлу на каждый, не больше 3)',
      endpointHint: 'host:port. По умолчанию engage.cloudflareclient.com:2408; сканер находит более быстрые.',
      addEndpoint: 'Добавить endpoint',
      mtu: 'MTU',
      dns: 'DNS',
      keepalive: 'Keepalive, с',
      ipv6: 'IPv6 внутри туннеля',
      preset: 'Мусорные пакеты',
      presetStandard: 'Стандарт (Jc 4, 40–70)',
      presetLight: 'Лёгкий (Jc 2, 20–40)',
      presetStrong: 'Сильный (Jc 8, 50–150)',
      presetCustom: 'Свои',
      advanced: 'Поля AmneziaWG',
      advancedHint: 'WARP говорит на обычном WireGuard: безопасны только мусорные пакеты (Jc, Jmin, Jmax). S1, S2 и H1–H4, отличные от 0 и значений по умолчанию, заставят Cloudflare сбросить рукопожатие.',
      save: 'Сохранить',
      saved: 'Сохранено.',
      endpointsTitle: 'Скорость: проверка и сканер',
      endpointsIntro:
        'Cloudflare отвечает на множестве адресов и портов, и они не одинаково быстры. Сканер опрашивает их настоящим рукопожатием с этого сервера и ранжирует; с «загрузкой» ещё и меряет скорость через лучшие несколько. Это то, что видит этот сервер, а клиент далеко отсюда может видеть иначе.',
      check: 'Проверить',
      checkSpeed: 'с загрузкой',
      checkEndpoint: 'Endpoint',
      scan: 'Сканировать',
      scanSpeed: 'загрузка через лучшие',
      scanV6: 'IPv6',
      stop: 'Остановить',
      working: 'Работаю…',
      stage: { probe: 'опрашиваю адреса', measure: 'меряю самые быстрые', speed: 'качаю', check: 'проверяю', done: 'готово' },
      resultOk: 'WARP работает',
      resultBad: 'Не работает',
      handshake: 'рукопожатие',
      latency: 'отклик',
      exit: 'выход',
      speed: 'скорость',
      colEndpoint: 'Endpoint',
      colRtt: 'RTT, мс',
      colLoss: 'Потери',
      colSpeed: 'Мбит/с',
      use: 'Взять',
      useBest: 'Взять лучшие: ',
      applied: 'Теперь в подписке эти endpoints.',
      nothingFound: 'Ни один endpoint не ответил. Аккаунт мог быть слишком новым для Cloudflare (повторите через минуту) или сервер не достаёт до WARP.',
      lastScan: 'Последний скан',
      autoScan: 'Сканировать самому',
      autoEvery: 'раз в, часов',
      autoKeep: 'оставлять лучших',
      autoSpeed: 'с загрузкой',
      autoApplied: 'применено',
      autoNote: 'Лучшие endpoints планового скана заменяют endpoints подписки.',
      subs: 'Подписки',
      subsSearch: 'Email или id подписки',
      subsFind: 'Найти',
      subsNone: 'Подписок нет.',
      subsNotLoaded: 'Клиенты панели ещё не прочитаны.',
      colSub: 'Подписка',
      colOn: 'WARP',
      colState: 'Состояние',
      active: 'активна',
      inactive: 'выключена / истекла',
      hasDevice: 'устройство готово',
      noDevice: 'устройства ещё нет',
      selectNote: 'Чтобы выбирать, поставьте «Кому выдавать»: «Только выбранным».',
      preview: 'Что получит подписчик',
      previewNote: 'Конфиг показан первым; в подписке тот же узел есть и как vpn://, и как amneziawg://. Ключи — собственные для каждого подписчика.',
      failed: function (e) { return 'Ошибка: ' + e; },
      yes: 'да',
      no: 'нет',
    },
  };

  var lang = 'en';
  var state = null;
  var timers = {};
  var subsQuery = '';
  var subsData = null;
  var ui = { check: null, scan: null, advanced: false, scanSpeed: true, scanV6: false, checkSpeed: true, checkEndpoint: '' };
  var draft = null;

  function T() { return TEXT[lang] || TEXT.en; }

  function h(tag, props, kids) {
    var el = document.createElement(tag);
    Object.keys(props || {}).forEach(function (k) {
      var v = props[k];
      if (v === null || v === undefined || v === false) return;
      if (k === 'text') el.textContent = v;
      else if (k === 'class') el.className = v;
      else if (k.slice(0, 2) === 'on') el.addEventListener(k.slice(2), v);
      else if (k === 'value' || k === 'checked' || k === 'disabled' || k === 'hidden' || k === 'selected') el[k] = v;
      else el.setAttribute(k, v === true ? '' : v);
    });
    [].concat(kids || []).forEach(function (c) {
      if (c === null || c === undefined || c === false) return;
      el.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
    });
    return el;
  }

  function say(text, bad) {
    var m = document.getElementById('message');
    m.textContent = text || '';
    m.className = 'message' + (text ? (bad ? ' bad' : ' good') : '');
  }

  function api(path, method, body) {
    return lucx.api('/api' + path, { method: method || 'GET', body: body }).then(function (res) {
      if (!res.ok) {
        var e = (res.body && res.body.error) || res.error || 'HTTP ' + res.status;
        return Promise.reject(new Error(e));
      }
      return res.body;
    });
  }

  function act(fn, okText) {
    return fn().then(function (r) { if (okText) say(okText); return r; }).catch(function (e) { say(T().failed(e.message), true); });
  }

  function fmtTime(s) { return s ? new Date(s).toLocaleString() : ''; }
  function fmtBytes(n) { return n >= 1e9 ? (n / 1e9).toFixed(1) + ' GB' : n >= 1e6 ? (n / 1e6).toFixed(0) + ' MB' : n + ' B'; }

  function field(label, input, hint) {
    return h('label', { class: 'field' }, [label, input, hint ? h('span', { class: 'small', text: hint }) : null]);
  }

  function check(label, checked, onchange) {
    return h('label', { class: 'check' }, [h('input', { type: 'checkbox', checked: !!checked, onchange: function (e) { onchange(e.target.checked); } }), label]);
  }

  function chip(text, kind) { return h('span', { class: 'chip ' + (kind || ''), text: text }); }

  // ---- draft of the settings form -------------------------------------------------------------

  function editable() {
    if (!draft) draft = JSON.parse(JSON.stringify(state.settings));
    return draft;
  }

  function saveSettings() {
    var d = editable();
    d.endpoints = d.endpoints.filter(function (e) { return e.trim() !== ''; });
    ['mtu', 'keepalive'].forEach(function (k) { d[k] = Number(d[k]) || 0; });
    ['jc', 'jmin', 'jmax', 's1', 's2'].forEach(function (k) { d.awg[k] = Number(d.awg[k]) || 0; });
    d.autoScan.hours = Number(d.autoScan.hours) || 0;
    d.autoScan.count = Number(d.autoScan.count) || 0;
    return act(function () { return api('/settings', 'POST', d); }, T().saved).then(function () { draft = null; return load(); });
  }

  // ---- sections ---------------------------------------------------------------------------------

  function secInbounds() {
    var s = editable(), t = T();
    var el = document.getElementById('secProtocol');
    var rows = state.inbounds.map(function (x) {
      return h('tr', {}, [
        h('td', { text: x.remark || '#' + x.id }),
        h('td', {}, [chip(x.enable ? t.inboundsOn : t.inboundsOff, x.enable ? 'good' : '')]),
        h('td', { class: 'mono', text: (x.endpoints || []).join(', ') }),
        h('td', { text: t.inboundSubs(x.subs) }),
      ]);
    });
    el.replaceChildren(
      h('h2', { text: t.inboundsTitle }),
      h('p', { class: 'muted small', text: t.inboundsNote }),
      h('table', {}, [h('tbody', {}, rows)]),
      h('div', { class: 'grid' }, [
        field(t.mode, h('select', { onchange: function (e) { s.mode = e.target.value; saveSettings(); } }, [
          h('option', { value: 'perClient', text: t.modePer, selected: s.mode === 'perClient' }),
          h('option', { value: 'shared', text: t.modeShared, selected: s.mode === 'shared' }),
        ]), t.modeHint),
      ])
    );
  }

  function secProtocol() {
    if (state.source === 'inbounds') return secInbounds();
    var s = editable(), t = T();
    var el = document.getElementById('secProtocol');
    el.replaceChildren(
      h('h2', { text: t.protocol }),
      state.inboundsError ? h('p', { class: 'banner', text: t.inboundsMissing }) : null,
      h('div', { class: 'row' }, [check(t.enabled, s.enabled, function (v) { s.enabled = v; saveSettings(); }), h('span', { class: 'muted small', text: t.enabledHint })]),
      h('div', { class: 'grid' }, [
        field(t.mode, h('select', { onchange: function (e) { s.mode = e.target.value; saveSettings(); } }, [
          h('option', { value: 'perClient', text: t.modePer, selected: s.mode === 'perClient' }),
          h('option', { value: 'shared', text: t.modeShared, selected: s.mode === 'shared' }),
        ]), t.modeHint),
        field(t.scope, h('select', { onchange: function (e) { s.scope = e.target.value; saveSettings(); } }, [
          h('option', { value: 'all', text: t.scopeAll, selected: s.scope === 'all' }),
          h('option', { value: 'selected', text: t.scopeSel, selected: s.scope === 'selected' }),
        ])),
        field(t.name, h('input', { type: 'text', maxlength: 60, value: s.name, onchange: function (e) { s.name = e.target.value; saveSettings(); } })),
      ])
    );
  }

  function secAccount() {
    var a = state.account, t = T(), el = document.getElementById('secAccount');
    var body = [h('h2', { text: t.account }), h('p', { class: 'muted small', text: t.accountHint })];
    if (!a.registered) {
      body.push(h('p', { text: t.notRegistered }));
      body.push(h('div', { class: 'row' }, [h('button', { class: 'primary', text: t.register, onclick: function () { act(function () { return api('/account/register', 'POST'); }).then(load); } }), h('span', { class: 'muted small', text: t.registerNote })]));
    } else {
      var rows = [
        h('span', { class: 'mono', text: t.accountId + ': ' + a.id }),
        chip(a.plus ? t.plus : t.free, a.plus ? 'good' : ''),
      ];
      if (a.license) rows.push(chip(t.licenseApplied, 'good'));
      if (a.quota) rows.push(h('span', { class: 'muted small', text: t.quota + ': ' + fmtBytes(a.quota) }));
      if (a.error) rows.push(chip(a.error, 'bad'));
      body.push(h('div', { class: 'row' }, rows));
      var lic = h('input', { type: 'text', placeholder: t.licensePlaceholder, autocomplete: 'off', spellcheck: 'false' });
      body.push(h('div', { class: 'row' }, [
        h('span', { text: t.licenseLabel }), lic,
        h('button', { text: t.applyLicense, onclick: function () { act(function () { return api('/account/license', 'POST', { license: lic.value }); }, t.licenseApplied).then(load); } }),
        h('button', { text: t.refresh, onclick: function () { act(function () { return api('/account/refresh', 'POST'); }).then(load); } }),
        h('button', { class: 'danger', text: t.deleteAccount, onclick: function () { if (confirm(t.confirmDeleteAccount)) act(function () { return api('/account/delete', 'POST'); }).then(load); } }),
      ]));
    }
    var d = state.devices, drow = [h('strong', { text: t.devices + ': ' }), h('span', { text: t.devicesCount(d.have, d.wanted) })];
    if (d.lastError) drow.push(chip(d.lastError, 'bad'));
    if (d.retryAt) drow.push(h('span', { class: 'muted small', text: t.devicesRetry(fmtTime(d.retryAt)) }));
    body.push(h('h3', { text: t.devices }));
    body.push(h('div', { class: 'row' }, drow.slice(1)));
    body.push(h('div', { class: 'row' }, [
      h('button', { text: t.issueNow, onclick: function () { act(function () { return api('/devices/sync', 'POST'); }).then(function () { setTimeout(load, 1500); }); } }),
      h('button', { class: 'danger', text: t.resetDevices, onclick: function () { if (confirm(t.confirmResetDevices)) act(function () { return api('/devices/reset', 'POST'); }).then(load); } }),
    ]));
    el.replaceChildren.apply(el, body);
  }

  function secSettings() {
    var s = editable(), t = T(), el = document.getElementById('secSettings');
    var eps = h('div', {});
    s.endpoints.forEach(function (e, i) {
      eps.appendChild(h('div', { class: 'row' }, [
        h('input', { type: 'text', class: 'mono', value: e, onchange: function (ev) { s.endpoints[i] = ev.target.value; } }),
        s.endpoints.length > 1 ? h('button', { text: '×', onclick: function () { s.endpoints.splice(i, 1); secSettings(); } }) : null,
      ]));
    });
    if (s.endpoints.length < 3) eps.appendChild(h('button', { text: t.addEndpoint, onclick: function () { s.endpoints.push(''); secSettings(); } }));

    function num(key, obj) {
      return field(key.toUpperCase(), h('input', { type: 'number', value: String(obj[key]), onchange: function (e) { obj[key] = e.target.value; s.preset = 'custom'; } }));
    }
    function txt(key) {
      return field(key.toUpperCase(), h('input', { type: 'text', class: 'mono', value: s.awg[key] || '', onchange: function (e) { s.awg[key] = e.target.value; s.preset = 'custom'; } }));
    }
    var adv = h('div', { class: 'advanced', hidden: !ui.advanced }, [
      h('p', { class: 'muted small', text: t.advancedHint }),
      h('div', { class: 'grid' }, [num('jc', s.awg), num('jmin', s.awg), num('jmax', s.awg), num('s1', s.awg), num('s2', s.awg), txt('h1'), txt('h2'), txt('h3'), txt('h4'), txt('i1'), txt('i2'), txt('i3'), txt('i4'), txt('i5')]),
    ]);
    var preset = h('select', { onchange: function (e) {
      s.preset = e.target.value;
      if (state.presets[s.preset]) { s.awg = JSON.parse(JSON.stringify(state.presets[s.preset])); }
      secSettings();
    } }, [
      ['standard', t.presetStandard], ['light', t.presetLight], ['strong', t.presetStrong], ['custom', t.presetCustom],
    ].map(function (p) { return h('option', { value: p[0], text: p[1], selected: s.preset === p[0] }); }));

    if (state.source === 'inbounds') {
      el.replaceChildren(
        h('h2', { text: t.defaultEndpoints }), eps, h('p', { class: 'muted small', text: t.defaultEndpointsHint }),
        h('div', { class: 'row' }, [h('button', { class: 'primary', text: t.save, onclick: saveSettings })])
      );
      return;
    }
    el.replaceChildren(
      h('h2', { text: t.settings }),
      h('h3', { text: t.endpoints }), eps, h('p', { class: 'muted small', text: t.endpointHint }),
      h('div', { class: 'grid' }, [
        field(t.mtu, h('input', { type: 'number', value: String(s.mtu), onchange: function (e) { s.mtu = e.target.value; } })),
        field(t.keepalive, h('input', { type: 'number', value: String(s.keepalive), onchange: function (e) { s.keepalive = e.target.value; } })),
        field(t.dns, h('input', { type: 'text', class: 'mono', value: s.dns, onchange: function (e) { s.dns = e.target.value; } })),
        field(t.preset, preset),
      ]),
      h('div', { class: 'row' }, [
        check(t.ipv6, s.ipv6, function (v) { s.ipv6 = v; }),
        h('button', { text: t.advanced, onclick: function () { ui.advanced = !ui.advanced; adv.hidden = !ui.advanced; } }),
      ]),
      adv,
      h('div', { class: 'row' }, [h('button', { class: 'primary', text: t.save, onclick: saveSettings })])
    );
  }

  function progressBar(job) {
    var t = T(), pct = job.total ? Math.round((100 * job.done) / job.total) : 0;
    var bar = h('i', {});
    bar.style.width = pct + '%';
    return h('div', {}, [h('span', { class: 'muted small', text: t.working + ' ' + (t.stage[job.stage] || job.stage || '') + (job.total ? ' ' + job.done + '/' + job.total : '') }), h('div', { class: 'progress' }, [bar])]);
  }

  function checkResult(r) {
    var t = T();
    if (!r) return null;
    var parts = [chip(r.ok ? t.resultOk : t.resultBad, r.ok ? 'good' : 'bad')];
    if (r.handshakeMs) parts.push(h('span', { text: t.handshake + ' ' + r.handshakeMs + ' ms' }));
    if (r.latencyMs) parts.push(h('span', { text: t.latency + ' ' + r.latencyMs + ' ms' }));
    if (r.colo || r.ip) parts.push(h('span', { text: t.exit + ' ' + [r.colo, r.loc, r.ip].filter(Boolean).join(' ') }));
    if (r.downMbps) parts.push(h('span', { text: t.speed + ' ' + r.downMbps + ' Mbit/s' }));
    if (r.error) parts.push(h('span', { class: 'muted', text: r.error }));
    return h('div', { class: 'row' }, parts);
  }

  function hitsTable(hits, applied) {
    var t = T();
    var tbody = h('tbody', {});
    hits.forEach(function (x) {
      tbody.appendChild(h('tr', {}, [
        h('td', { class: 'mono', text: x.endpoint }), h('td', { text: String(x.rttMs) }), h('td', { text: x.lossPct + '%' }),
        h('td', { text: x.tested ? (x.downMbps ? String(x.downMbps) : '–') : '' }),
        h('td', {}, [h('button', { text: t.use, onclick: function () { useEndpoints([x.endpoint]); } })]),
      ]));
    });
    var best = hits.slice(0, Math.min(3, hits.length)).map(function (x) { return x.endpoint; });
    return h('div', {}, [
      h('table', {}, [h('thead', {}, [h('tr', {}, [t.colEndpoint, t.colRtt, t.colLoss, t.colSpeed, ''].map(function (c) { return h('th', { text: c }); }))]), tbody]),
      h('div', { class: 'row' }, [h('button', { class: 'primary', text: t.useBest + best.length, onclick: function () { useEndpoints(best); } })]),
      applied && applied.length ? h('p', { class: 'muted small', text: t.autoApplied + ': ' + applied.join(', ') }) : null,
    ]);
  }

  function useEndpoints(list) {
    act(function () { return api('/endpoints', 'POST', { endpoints: list }); }, T().applied).then(function () { draft = null; load(); });
  }

  function secEndpoints() {
    var t = T(), s = editable(), el = document.getElementById('secEndpoints');
    var out = [h('h2', { text: t.endpointsTitle }), h('p', { class: 'muted small', text: t.endpointsIntro })];
    var disabled = !state.account.registered || !state.engine.ok;

    var epInput = h('input', { type: 'text', class: 'mono', value: ui.checkEndpoint || s.endpoints[0] || '', onchange: function (e) { ui.checkEndpoint = e.target.value; } });
    out.push(h('div', { class: 'row' }, [
      h('span', { text: t.checkEndpoint }), epInput, check(t.checkSpeed, ui.checkSpeed, function (v) { ui.checkSpeed = v; }),
      h('button', { text: t.check, disabled: disabled || (ui.check && ui.check.running), onclick: function () {
        ui.check = { running: true, job: { stage: 'check' } };
        render();
        act(function () { return api('/check', 'POST', { endpoint: epInput.value.trim(), speed: ui.checkSpeed }); }).then(function (r) {
          if (!r) { ui.check = null; render(); return; }
          pollJob('check', r.id);
        });
      } }),
    ]));
    if (ui.check) out.push(ui.check.running ? progressBar(ui.check.job || {}) : checkResult(ui.check.result));

    out.push(h('div', { class: 'row' }, [
      check(t.scanSpeed, ui.scanSpeed, function (v) { ui.scanSpeed = v; }), check(t.scanV6, ui.scanV6, function (v) { ui.scanV6 = v; }),
      h('button', { class: 'primary', text: t.scan, disabled: disabled || state.scanning || (ui.scan && ui.scan.running), onclick: function () {
        ui.scan = { running: true, job: { stage: 'probe' } };
        render();
        act(function () { return api('/scan', 'POST', { speed: ui.scanSpeed, ipv6: ui.scanV6 }); }).then(function (r) {
          if (!r) { ui.scan = null; render(); return; }
          ui.scan.id = r.id;
          pollJob('scan', r.id);
        });
      } }),
      ui.scan && ui.scan.running && ui.scan.id ? h('button', { text: t.stop, onclick: function () { act(function () { return api('/jobs/' + ui.scan.id, 'DELETE'); }); } }) : null,
    ]));
    if (ui.scan && ui.scan.running) out.push(progressBar(ui.scan.job || {}));
    var shown = ui.scan && !ui.scan.running ? ui.scan.result : null;
    if (shown && shown.hits && shown.hits.length) out.push(hitsTable(shown.hits));
    else if (shown) out.push(h('p', { class: 'muted', text: ui.scan.error || t.nothingFound }));
    else if (state.lastScan && !state.scanning) {
      var ls = state.lastScan, res = null;
      try { res = ls.result; } catch (e) { res = null; }
      out.push(h('h3', { text: t.lastScan + ' · ' + fmtTime(ls.at) + (ls.auto ? ' (auto)' : '') }));
      if (ls.error) out.push(h('p', { class: 'muted', text: ls.error }));
      else if (res && res.hits && res.hits.length) out.push(hitsTable(res.hits, ls.applied));
      else out.push(h('p', { class: 'muted', text: t.nothingFound }));
    }

    var a = s.autoScan;
    out.push(h('h3', { text: t.autoScan }));
    out.push(h('div', { class: 'row' }, [
      check(t.autoScan, a.enabled, function (v) { a.enabled = v; }),
      field(t.autoEvery, h('input', { type: 'number', value: String(a.hours), onchange: function (e) { a.hours = e.target.value; } })),
      field(t.autoKeep, h('input', { type: 'number', value: String(a.count), onchange: function (e) { a.count = e.target.value; } })),
      check(t.autoSpeed, a.speed, function (v) { a.speed = v; }),
      h('button', { text: t.save, onclick: saveSettings }),
    ]));
    out.push(h('p', { class: 'muted small', text: t.autoNote }));
    el.replaceChildren.apply(el, out);
  }

  function pollJob(kind, id) {
    clearTimeout(timers[kind]);
    api('/jobs/' + id).then(function (job) {
      var slot = ui[kind];
      if (!slot) return;
      slot.job = job;
      if (job.state === 'running') {
        timers[kind] = setTimeout(function () { pollJob(kind, id); }, 1500);
        secEndpoints();
        return;
      }
      slot.running = false;
      slot.result = job.state === 'done' ? job.result : null;
      slot.error = job.state === 'failed' ? job.error : job.state === 'canceled' ? 'canceled' : '';
      if (kind === 'check' && !slot.result && slot.error) slot.result = { ok: false, error: slot.error };
      if (kind === 'scan' && slot.result === null) slot.result = { hits: [] };
      load();
    }).catch(function (e) {
      if (ui[kind]) { ui[kind].running = false; ui[kind].error = e.message; ui[kind].result = kind === 'check' ? { ok: false, error: e.message } : { hits: [] }; }
      say(T().failed(e.message), true);
      render();
    });
  }

  function secSubs() {
    var t = T(), s = editable(), el = document.getElementById('secSubs');
    var search = h('input', { type: 'search', placeholder: t.subsSearch, value: subsQuery, onkeydown: function (e) { if (e.key === 'Enter') { subsQuery = e.target.value; loadSubs(); } } });
    var out = [h('h2', { text: t.subs }), h('div', { class: 'row' }, [search, h('button', { text: t.subsFind, onclick: function () { subsQuery = search.value; loadSubs(); } })])];
    var byInbounds = state.source === 'inbounds';
    if (!byInbounds && s.scope !== 'selected') out.push(h('p', { class: 'muted small', text: t.selectNote }));
    if (!subsData || !subsData.loaded) out.push(h('p', { class: 'muted', text: subsData && subsData.error ? subsData.error : t.subsNotLoaded }));
    else if (!subsData.subs.length) out.push(h('p', { class: 'muted', text: t.subsNone }));
    else {
      var tbody = h('tbody', {});
      subsData.subs.forEach(function (x) {
        tbody.appendChild(h('tr', {}, [
          h('td', {}, [check('', x.selected, function (v) {
            act(function () { return api('/scope', 'POST', v ? { add: [x.subId] } : { remove: [x.subId] }); }).then(function () { loadSubs(); });
          })]),
          h('td', {}, [h('div', { text: x.emails.join(', ') }), h('div', { class: 'mono muted', text: x.subId })]),
          h('td', {}, [chip(x.active ? t.active : t.inactive, x.active ? 'good' : 'warn'), ' ', chip(x.device ? t.hasDevice : t.noDevice, x.device ? 'good' : '')]),
        ]));
      });
      var tbl = h('table', {}, [h('thead', {}, [h('tr', {}, [t.colOn, t.colSub, t.colState].map(function (c) { return h('th', { text: c }); }))]), tbody]);
      if (byInbounds || s.scope !== 'selected') tbl.querySelectorAll('input').forEach(function (i) { i.disabled = true; });
      out.push(tbl);
    }
    el.replaceChildren.apply(el, out);
  }

  function secPreview() {
    var t = T(), el = document.getElementById('secPreview');
    api('/preview').then(function (list) {
      var out = [h('h2', { text: t.preview }), h('p', { class: 'muted small', text: t.previewNote })];
      list.forEach(function (n) { out.push(h('h3', { text: n.name + ' · ' + n.endpoint })); out.push(h('pre', { text: n.conf })); });
      el.replaceChildren.apply(el, out);
    }).catch(function () {});
  }

  function render() {
    var t = T();
    document.documentElement.setAttribute('lang', lang);
    document.querySelectorAll('[data-i18n]').forEach(function (n) { n.textContent = t[n.getAttribute('data-i18n')]; });
    var banner = document.getElementById('engineBanner');
    banner.hidden = state.engine.ok;
    banner.textContent = state.engine.ok ? '' : t.engineMissing(state.engine.error || '');
    secProtocol(); secAccount(); secSettings(); secEndpoints(); secSubs();
  }

  function loadSubs() {
    return api('/subs?search=' + encodeURIComponent(subsQuery)).then(function (d) { subsData = d; secSubs(); }).catch(function (e) { say(T().failed(e.message), true); });
  }

  function load() {
    return api('/state').then(function (s) {
      state = s;
      if (!draft) draft = JSON.parse(JSON.stringify(s.settings));
      render();
      secPreview();
      return loadSubs();
    }).catch(function (e) { say(T().failed(e.message), true); });
  }

  lucx.ready().then(function (ctx) {
    lucx.applyTheme();
    lang = (ctx.lang || 'en').slice(0, 2) === 'ru' ? 'ru' : 'en';
    load();
    setInterval(function () { if (!document.hidden && !(ui.scan && ui.scan.running) && !(ui.check && ui.check.running)) { var d = draft; api('/state').then(function (s) { state = s; draft = d; secAccount(); }).catch(function () {}); } }, 20000);
  });
  lucx.onContext(function (ctx) {
    lucx.applyTheme();
    lang = (ctx.lang || 'en').slice(0, 2) === 'ru' ? 'ru' : 'en';
    if (state) render();
  });
})();

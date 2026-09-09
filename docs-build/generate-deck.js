// CAP — презентация для выступления (RU), 11 слайдов, 13.33x7.5
// Палитра: BG_DARK (титул/финал) -> светлые контентные слайды; PRIMARY сталь, ACCENT циан CAP.
const pptxgen = require("/home/narziev/Documents/AutoPro/docs-build/node_modules/pptxgenjs");

const BG_DARK = "0A0E13", BG = "FFFFFF", TINT = "F1F5F9";
const PRIMARY = "12314F", ACCENT = "38BDF8", TEXT = "0F172A", MUTED = "64748B";
const W = 13.33, H = 7.5, M = 0.55;
const FONT = "Arial";

const pres = new pptxgen();
pres.layout = "LAYOUT_WIDE";
pres.author = "4codegit";
pres.title = "Complex Automation Pro (CAP)";

// Фирменный знак: пульс-линия (сегменты)
function pulseMark(slide, x, y, scale, color) {
  const seg = [[0, 0.5], [0.22, 0.5], [0.34, 0.08], [0.5, 0.86], [0.62, 0.32], [0.78, 0.5], [1, 0.5]];
  for (let i = 0; i < seg.length - 1; i++) {
    slide.addShape(pres.shapes.LINE, {
      x: x + seg[i][0] * scale, y: y + seg[i][1] * scale,
      w: (seg[i + 1][0] - seg[i][0]) * scale, h: (seg[i + 1][1] - seg[i][1]) * scale,
      line: { color, width: 2.4 },
    });
  }
}
function footer(slide, n, dark) {
  slide.addText(`CAP · Complex Automation Pro — MVP · ${n} / 11`, {
    x: M, y: H - 0.42, w: W - 2 * M, h: 0.3, fontSize: 10,
    color: dark ? "5C6875" : MUTED, fontFace: FONT, margin: 0,
  });
}
function h(slide, text, opts = {}) {
  slide.addText(text, { x: M + 0.75, y: 0.42, w: W - 2 * M, h: 0.75, fontSize: 30, bold: true, color: opts.color || PRIMARY, fontFace: FONT, margin: 0, ...opts });
}
const bu = () => ({ code: "25B8", indent: 12 });

// ── 1. Титул (dark) ─────────────────────────────────────────────────────────
{
  const s = pres.addSlide();
  s.background = { color: BG_DARK };
  pulseMark(s, M, 0.85, 1.7, ACCENT);
  s.addText("COMPLEX AUTOMATION PRO", { x: M, y: 2.35, w: 10, h: 0.4, fontSize: 15, color: "5C6875", charSpacing: 4, fontFace: FONT, margin: 0 });
  s.addText("CAP", { x: M - 0.04, y: 2.7, w: 9, h: 1.55, fontSize: 92, bold: true, color: "FFFFFF", fontFace: FONT, margin: 0 });
  s.addText("Мониторинг обогатительной фабрики\nв реальном времени — от приёма руды до отгрузки концентрата", {
    x: M, y: 4.45, w: 9.4, h: 1.0, fontSize: 18, color: "93A1B0", fontFace: FONT, margin: 0,
  });
  s.addText([
    { text: "MVP · сентябрь 2026", options: { breakLine: true } },
    { text: "github.com/4codegit/complex_automation_pro · лицензия MIT" },
  ], { x: M, y: 6.35, w: 10, h: 0.7, fontSize: 12, color: "5C6875", fontFace: FONT, margin: 0 });
}

// ── 2. Задача (light) ───────────────────────────────────────────────────────
{
  const s = pres.addSlide();
  s.background = { color: BG };
  pulseMark(s, M, 0.5, 0.5, ACCENT);
  h(s, "Задача: завод видит только часть себя");
  s.addText(
    "Технологические участки фабрики разрознены: сигналы живут в локальных контроллерах, оператор узнаёт о проблеме по звонку, а обрыв связи означает потерю данных.",
    { x: M, y: 1.5, w: 6.1, h: 1.7, fontSize: 17, color: TEXT, fontFace: FONT, margin: 0, lineSpacingMultiple: 1.25 },
  );
  s.addText([
    { text: "Данные с датчиков не доходят до оператора", options: { bullet: bu(), breakLine: true } },
    { text: "История измерений не хранится централизованно", options: { bullet: bu(), breakLine: true } },
    { text: "Отклонения от регламента видны постфактум", options: { bullet: bu() } },
  ], { x: M, y: 3.4, w: 6.1, h: 2.2, fontSize: 15, color: MUTED, fontFace: FONT, paraSpaceAfter: 10, margin: 0 });

  const cards = [
    { n: "9", t: "технологических участков\nпод наблюдением" },
    { n: "3", t: "промышленных протокола:\nModbus, OPC UA, Sparkplug B" },
    { n: "0", t: "команд записи в АСУ ТП —\nплатформа только наблюдает" },
  ];
  cards.forEach((c, i) => {
    const y = 1.35 + i * 1.75;
    s.addShape(pres.shapes.ROUNDED_RECTANGLE, { x: 7.3, y, w: 5.5, h: 1.55, fill: { color: TINT }, rectRadius: 0.08 });
    s.addText(c.n, { x: 7.55, y: y + 0.18, w: 1.35, h: 1.2, fontSize: 52, bold: true, color: ACCENT, fontFace: FONT, margin: 0 });
    s.addText(c.t, { x: 9.0, y: y + 0.3, w: 3.7, h: 1.0, fontSize: 14, color: TEXT, fontFace: FONT, margin: 0 });
  });
  footer(s, 2);
}

// ── 3. Архитектура (light, диаграмма) ───────────────────────────────────────
{
  const s = pres.addSlide();
  s.background = { color: BG };
  pulseMark(s, M, 0.5, 0.5, ACCENT);
  h(s, "Архитектура: edge-шлюзы и микросервисы");
  const box = (x, y, w, hh, title, sub, dark) => {
    s.addShape(pres.shapes.ROUNDED_RECTANGLE, {
      x, y, w, h: hh, rectRadius: 0.06,
      fill: { color: dark ? PRIMARY : TINT },
      line: dark ? { color: PRIMARY, width: 0 } : { color: "E2E8F0", width: 1 },
    });
    s.addText(title, { x, y: y + 0.12, w, h: 0.4, fontSize: 15, bold: true, color: dark ? "FFFFFF" : TEXT, align: "center", fontFace: FONT, margin: 0 });
    s.addText(sub, { x, y: y + 0.52, w, h: 0.55, fontSize: 11.5, color: dark ? "93A1B0" : MUTED, align: "center", fontFace: FONT, margin: 0 });
  };
  const arrow = (x1, y1, x2, y2) => s.addShape(pres.shapes.LINE, { x: x1, y: y1, w: x2 - x1, h: y2 - y1, line: { color: MUTED, width: 1.6, endArrowType: "triangle" } });

  box(M, 1.75, 2.6, 1.05, "Датчики · PLC · DCS", "уровни 0–2, вне CAP");
  box(M, 3.35, 2.6, 1.35, "Edge-шлюз", "буфер SQLite/WAL,\nstore-and-forward", true);
  box(4.1, 1.75, 2.2, 2.95, "gateway-api", "единая точка входа\n:8000", true);
  const svcs = [["ingest", "приём"], ["historian", "история"], ["alarms", "аварии"], ["profiles", "профили"], ["registry", "реестр"], ["identity", "доступ"]];
  svcs.forEach(([name, sub], i) => box(6.9 + (i % 2) * 1.9, 1.75 + Math.floor(i / 2) * 1.0, 1.75, 0.86, name, sub));
  box(11.1, 1.75, 1.75, 2.95, "Панель", "React · WebSocket", false);
  arrow(2.6, 2.3, 4.1, 2.3);
  arrow(6.3, 2.3, 6.9, 2.3);
  arrow(10.55, 3.0, 11.1, 3.0);
  s.addText("Все сервисы — отдельные процессы из одного кода; в режиме разработки работают как один процесс. Контракт API и панель не меняются между режимами.", {
    x: M, y: 5.15, w: W - 2 * M, h: 0.8, fontSize: 13, color: MUTED, fontFace: FONT, margin: 0,
  });
  footer(s, 3);
}

// ── 4. Драйверы (light) ─────────────────────────────────────────────────────
{
  const s = pres.addSlide();
  s.background = { color: BG };
  pulseMark(s, M, 0.5, 0.5, ACCENT);
  h(s, "Три промышленных протокола из коробки");
  const cards = [
    { t: "Modbus TCP", d: "Только чтение, функции 1–4. Регистры: bool, u16…f32, масштаб, перестановка слов CDAB. Реальный источник для демо — планшет с Modbus-сервером." },
    { t: "OPC UA", d: "Poll и подписки (subscriptions), безопасность Sign/SignAndEncrypt, аутентификация по сертификату x509, анонимный или логин/пароль." },
    { t: "MQTT Sparkplug B", d: "Роль Primary Host Application: подписка spBv1.0/#, разрешение alias→name по Birth-сообщениям, protobuf-декодер без кодогенерации." },
  ];
  cards.forEach((c, i) => {
    const x = M + i * 4.15;
    s.addShape(pres.shapes.ROUNDED_RECTANGLE, { x, y: 1.6, w: 3.9, h: 3.3, fill: { color: TINT }, rectRadius: 0.08 });
    s.addText(c.t, { x: x + 0.25, y: 1.85, w: 3.4, h: 0.45, fontSize: 19, bold: true, color: PRIMARY, fontFace: FONT, margin: 0 });
    s.addText(c.d, { x: x + 0.25, y: 2.4, w: 3.4, h: 2.3, fontSize: 13, color: TEXT, fontFace: FONT, margin: 0, lineSpacingMultiple: 1.2 });
  });
  s.addText([
    { text: "Граница read-only — в коде: ", options: { bold: true, color: TEXT } },
    { text: "драйверы не содержат функций записи (FC 5/6/15/16 отсутствуют). Конфигурация сигналов — одна строка на тег:  mill_power:kW|reg=hr:100:f32", options: { color: MUTED } },
  ], { x: M, y: 5.35, w: W - 2 * M, h: 0.85, fontSize: 14, fontFace: FONT, margin: 0 });
  footer(s, 4);
}

// ── 5. Интерфейс (light, скриншот) ──────────────────────────────────────────
{
  const s = pres.addSlide();
  s.background = { color: BG };
  pulseMark(s, M, 0.5, 0.5, ACCENT);
  h(s, "Панель оператора");
  const iw = 8.6, ih = iw * 1000 / 1680;
  s.addShape(pres.shapes.ROUNDED_RECTANGLE, { x: M - 0.06, y: 1.53, w: iw + 0.12, h: ih + 0.12, fill: { color: "FFFFFF" }, line: { color: "E2E8F0", width: 1 }, rectRadius: 0.05 });
  s.addImage({ path: "assets/overview-light.png", x: M, y: 1.59, w: iw, h: ih });
  const pts = [
    ["Участки и живой график", "карточки стадий, WebSocket-поток"],
    ["Качество каждого сигнала", "good / устаревшее / нет связи"],
    ["Светлая и тёмная темы", "переключатель в меню"],
  ];
  pts.forEach(([t, d], i) => {
    const y = 1.8 + i * 1.35;
    s.addText(t, { x: 9.45, y, w: 3.4, h: 0.4, fontSize: 15, bold: true, color: PRIMARY, fontFace: FONT, margin: 0 });
    s.addText(d, { x: 9.45, y: y + 0.38, w: 3.4, h: 0.6, fontSize: 12.5, color: MUTED, fontFace: FONT, margin: 0 });
  });
  footer(s, 5);
}

// ── 6. Аналитика (light, скриншот + формулы) ────────────────────────────────
{
  const s = pres.addSlide();
  s.background = { color: BG };
  pulseMark(s, M, 0.5, 0.5, ACCENT);
  h(s, "Металлургия считается, а не угадывается");
  const iw = 8.6, ih = iw * 1000 / 1680;
  s.addShape(pres.shapes.ROUNDED_RECTANGLE, { x: M - 0.06, y: 1.53, w: iw + 0.12, h: ih + 0.12, fill: { color: "FFFFFF" }, line: { color: "E2E8F0", width: 1 }, rectRadius: 0.05 });
  s.addImage({ path: "assets/analytics-light.png", x: M, y: 1.59, w: iw, h: ih });
  const formulas = [
    ["Твёрдое в пульпе", "%S = ρт(ρп−ρв) / (ρт−ρв)ρп"],
    ["Удельный расход", "q = Qр·60 / Qт  [мЛ/т]"],
    ["Сухая производительность", "Qсух = Qт·(1 − W/100)"],
  ];
  formulas.forEach(([t, f], i) => {
    const y = 1.7 + i * 1.15;
    s.addShape(pres.shapes.ROUNDED_RECTANGLE, { x: 9.45, y, w: 3.35, h: 0.98, fill: { color: TINT }, rectRadius: 0.06 });
    s.addText(t, { x: 9.62, y: y + 0.1, w: 3.05, h: 0.32, fontSize: 12, bold: true, color: PRIMARY, fontFace: FONT, margin: 0 });
    s.addText(f, { x: 9.62, y: y + 0.44, w: 3.05, h: 0.4, fontSize: 12.5, color: TEXT, fontFace: "Consolas", margin: 0 });
  });
  s.addText("Формула на каждой карточке; нет данных — честное «нет данных», без заглушек.", {
    x: 9.45, y: 5.25, w: 3.35, h: 0.9, fontSize: 11.5, color: MUTED, fontFace: FONT, margin: 0,
  });
  footer(s, 6);
}

// ── 7. Надёжность (light, поток) ────────────────────────────────────────────
{
  const s = pres.addSlide();
  s.background = { color: BG };
  pulseMark(s, M, 0.5, 0.5, ACCENT);
  h(s, "Обрыв связи не теряет ни одного измерения");
  const steps = [
    ["1", "Сбор", "шлюз опрашивает приборы\nкаждую секунду"],
    ["2", "Буфер", "каждое сообщение —\nв SQLite/WAL на краю"],
    ["3", "Сбой сервера", "данные копятся,\nотсчёт не останавливается"],
    ["4", "Возврат", "backfill: очередь досылается\nполностью, порядок сохранён"],
  ];
  steps.forEach(([n, t, d], i) => {
    const x = M + i * 3.2;
    s.addShape(pres.shapes.OVAL, { x: x + 1.25, y: 1.75, w: 0.62, h: 0.62, fill: { color: PRIMARY } });
    s.addText(n, { x: x + 1.25, y: 1.75, w: 0.62, h: 0.62, fontSize: 20, bold: true, color: "FFFFFF", align: "center", valign: "middle", fontFace: FONT, margin: 0 });
    if (i < 3) s.addShape(pres.shapes.LINE, { x: x + 2.05, y: 2.06, w: 1.2, h: 0, line: { color: MUTED, width: 1.4, endArrowType: "triangle" } });
    s.addText(t, { x, y: 2.6, w: 3.1, h: 0.4, fontSize: 17, bold: true, color: TEXT, align: "center", fontFace: FONT, margin: 0 });
    s.addText(d, { x: x + 0.15, y: 3.02, w: 2.8, h: 0.85, fontSize: 12, color: MUTED, align: "center", fontFace: FONT, margin: 0 });
  });
  s.addShape(pres.shapes.ROUNDED_RECTANGLE, { x: M, y: 4.5, w: W - 2 * M, h: 1.7, fill: { color: TINT }, rectRadius: 0.08 });
  s.addText("64", { x: M + 0.5, y: 4.72, w: 1.8, h: 1.25, fontSize: 60, bold: true, color: ACCENT, fontFace: FONT, margin: 0 });
  s.addText("сообщений пережили демонстрационный отказ сервера —\nи все 64 дошли после восстановления, без потерь и дублей (сценарий demo.sh)", {
    x: M + 2.6, y: 4.95, w: 9.3, h: 0.9, fontSize: 14.5, color: TEXT, fontFace: FONT, margin: 0,
  });
  footer(s, 7);
}

// ── 8. Аварии и аудит (light) ───────────────────────────────────────────────
{
  const s = pres.addSlide();
  s.background = { color: BG };
  pulseMark(s, M, 0.5, 0.5, ACCENT);
  h(s, "Аварии по ISA-18.2, каждое действие — в аудите");
  const states = ["normal", "active_unack", "active_ack", "returned", "shelved", "suppressed"];
  states.forEach((st, i) => {
    const x = M + i * 2.07;
    s.addShape(pres.shapes.ROUNDED_RECTANGLE, { x, y: 1.7, w: 1.9, h: 0.62, fill: { color: i === 1 ? "FFF1F0" : TINT }, line: i === 1 ? { color: "FFA39E", width: 1 } : { color: "E2E8F0", width: 1 }, rectRadius: 0.05 });
    s.addText(st, { x, y: 1.7, w: 1.9, h: 0.62, fontSize: 11.5, color: i === 1 ? "CF1322" : MUTED, align: "center", valign: "middle", fontFace: "Consolas", margin: 0 });
  });
  s.addText("Жизненный цикл аварии управляется уставками активного профиля руды.", { x: M, y: 2.45, w: W - 2 * M, h: 0.35, fontSize: 12.5, color: MUTED, fontFace: FONT, margin: 0 });

  const rows = [
    ["Квитирование", "оператор подтверждает аварию; действие фиксируется в неизменяемом журнале аудита: кто, что, когда"],
    ["Рационализация", "уставки по каждому тегу согласуются технологом и версионируются через профили"],
    ["Права доступа", "RBAC: manage_tags, acknowledge_alarms, manage_profiles… субъект без роли получает 403 и запись в аудите"],
  ];
  rows.forEach(([t, d], i) => {
    const y = 3.1 + i * 1.15;
    s.addShape(pres.shapes.ROUNDED_RECTANGLE, { x: M, y, w: 12.2, h: 1.0, fill: { color: TINT }, rectRadius: 0.06 });
    s.addText(t, { x: M + 0.3, y: y + 0.12, w: 2.6, h: 0.75, fontSize: 14.5, bold: true, color: PRIMARY, valign: "middle", fontFace: FONT, margin: 0 });
    s.addText(d, { x: M + 3.1, y: y + 0.12, w: 8.9, h: 0.75, fontSize: 13, color: TEXT, valign: "middle", fontFace: FONT, margin: 0 });
  });
  footer(s, 8);
}

// ── 9. Безопасность (light, statement) ──────────────────────────────────────
{
  const s = pres.addSlide();
  s.background = { color: BG };
  pulseMark(s, M, 0.5, 0.5, ACCENT);
  h(s, "Наблюдает — но не управляет");
  s.addText("Потеря или компрометация CAP\nне останавливает производство.", {
    x: M, y: 1.6, w: 12.2, h: 1.7, fontSize: 34, bold: true, color: PRIMARY, fontFace: FONT, margin: 0, lineSpacingMultiple: 1.15,
  });
  const cards = [
    ["Уровень 3 по ISA-95", "чтение из уровней 0–2; контуры регулирования, блокировки и аварийные остановы остаются в независимых системах"],
    ["Только чтение", "в коде драйверов нет операций записи; pull используется для дозаполнения истории, никогда — для команд"],
    ["Прозрачность данных", "коды качества good/uncertain/bad/offline: плохие данные не превращаются в нормальные, прорехи видны"],
  ];
  cards.forEach(([t, d], i) => {
    const x = M + i * 4.15;
    s.addShape(pres.shapes.ROUNDED_RECTANGLE, { x, y: 3.6, w: 3.9, h: 2.5, fill: { color: TINT }, rectRadius: 0.08 });
    s.addText(t, { x: x + 0.25, y: 3.85, w: 3.4, h: 0.5, fontSize: 16, bold: true, color: PRIMARY, fontFace: FONT, margin: 0 });
    s.addText(d, { x: x + 0.25, y: 4.4, w: 3.4, h: 1.6, fontSize: 12.5, color: TEXT, fontFace: FONT, margin: 0, lineSpacingMultiple: 1.2 });
  });
  footer(s, 9);
}

// ── 10. Статус и план (light) ───────────────────────────────────────────────
{
  const s = pres.addSlide();
  s.background = { color: BG };
  pulseMark(s, M, 0.5, 0.5, ACCENT);
  h(s, "Статус MVP и дорожная карта");
  s.addShape(pres.shapes.ROUNDED_RECTANGLE, { x: M, y: 1.6, w: 5.95, h: 4.9, fill: { color: "F0FDF4" }, rectRadius: 0.08 });
  s.addText("Готово и проверено", { x: M + 0.3, y: 1.85, w: 5.3, h: 0.45, fontSize: 18, bold: true, color: "166534", fontFace: FONT, margin: 0 });
  s.addText([
    { text: "7 микросервисов + edge-шлюз, живой прогон", options: { bullet: bu(), breakLine: true } },
    { text: "Драйверы Modbus TCP, OPC UA, Sparkplug B", options: { bullet: bu(), breakLine: true } },
    { text: "Store-and-forward: отказ → полный backfill", options: { bullet: bu(), breakLine: true } },
    { text: "Расчётные показатели с формулами", options: { bullet: bu(), breakLine: true } },
    { text: "Аварии ISA-18.2, RBAC, аудит, профили руды", options: { bullet: bu(), breakLine: true } },
    { text: "Контрактные тесты, светлая и тёмная темы", options: { bullet: bu() } },
  ], { x: M + 0.3, y: 2.4, w: 5.35, h: 3.9, fontSize: 13.5, color: TEXT, fontFace: FONT, paraSpaceAfter: 10, margin: 0 });

  s.addShape(pres.shapes.ROUNDED_RECTANGLE, { x: 6.85, y: 1.6, w: 5.95, h: 4.9, fill: { color: TINT }, rectRadius: 0.08 });
  s.addText("Дальше", { x: 7.15, y: 1.85, w: 5.3, h: 0.45, fontSize: 18, bold: true, color: PRIMARY, fontFace: FONT, margin: 0 });
  s.addText([
    { text: "Полевые испытания драйверов на оборудовании площадки", options: { bullet: bu(), breakLine: true } },
    { text: "Брокер событий (NATS) вместо точечных EVENT_SINKS", options: { bullet: bu(), breakLine: true } },
    { text: "Перенос ядра на Rust, панель оператора — на Slint", options: { bullet: bu(), breakLine: true } },
    { text: "PostgreSQL-профиль, резервирование, приёмочные испытания", options: { bullet: bu() } },
  ], { x: 7.15, y: 2.45, w: 5.35, h: 3.4, fontSize: 13.5, color: TEXT, fontFace: FONT, paraSpaceAfter: 12, margin: 0 });
  footer(s, 10);
}

// ── 11. Финал (dark) ────────────────────────────────────────────────────────
{
  const s = pres.addSlide();
  s.background = { color: BG_DARK };
  pulseMark(s, M, 2.0, 2.2, ACCENT);
  s.addText("CAP", { x: M - 0.04, y: 3.1, w: 9, h: 1.3, fontSize: 76, bold: true, color: "FFFFFF", fontFace: FONT, margin: 0 });
  s.addText("Комплексная автоматизация, которую можно проверить:\nоткрытый код, прозрачные формулы, честные прорехи в данных.", {
    x: M, y: 4.55, w: 10.5, h: 0.95, fontSize: 16, color: "93A1B0", fontFace: FONT, margin: 0,
  });
  s.addText([
    { text: "github.com/4codegit/complex_automation_pro", options: { breakLine: true } },
    { text: "Лицензия MIT · Правообладатель 4codegit · 2026" },
  ], { x: M, y: 5.9, w: 10, h: 0.7, fontSize: 13, color: "5C6875", fontFace: FONT, margin: 0 });
}

pres.writeFile({ fileName: "CAP_presentation.pptx" }).then(() => console.log("PPTX written"));

// CAP presentation deck (pptx). Запуск: node generate-deck.js
const pptxgen = require("pptxgenjs");
const fs = require("fs");
const { imageSize } = require("image-size");

const A = "/home/narziev/Documents/AutoPro/docs-build/assets";
const OUT = "/home/narziev/Documents/AutoPro/docs/CAP_Презентация.pptx";

const C = {
  bg: "0E1622", panel: "16202E", panel2: "1B2838",
  accent: "38BDF8", green: "34D399", amber: "F59E0B", red: "F87171",
  text: "E6EDF5", muted: "8CA0B8", line: "2A3A4E",
};
const W = 13.33, H = 7.5;

const pptx = new pptxgen();
pptx.defineLayout({ name: "WIDE", width: W, height: H });
pptx.layout = "WIDE";
pptx.author = "CAP";
pptx.title = "CAP — АСУ ТП участка флотационного обогащения руды";

function baseSlide(num, title, tag) {
  const s = pptx.addSlide();
  s.background = { color: C.bg };
  // header
  s.addShape("rect", { x: 0, y: 0, w: W, h: 0.16, fill: { color: C.accent } });
  if (title) {
    s.addText(title, { x: 0.55, y: 0.32, w: 10.6, h: 0.7, fontSize: 27, bold: true,
      color: C.text, fontFace: "Arial" });
    if (tag) s.addText(tag, { x: 10.0, y: 0.38, w: 2.8, h: 0.5, fontSize: 11, align: "right",
      color: C.muted, fontFace: "Consolas" });
  }
  // footer
  s.addShape("line", { x: 0.55, y: H - 0.5, w: W - 1.1, h: 0, line: { color: C.line, width: 0.75 } });
  if (num) s.addText(`CAP · ${num}`, { x: 0.55, y: H - 0.42, w: 5, h: 0.3, fontSize: 10,
    color: C.muted, fontFace: "Consolas" });
  s.addText("Complex Automation Pro", { x: W - 4.05, y: H - 0.42, w: 3.5, h: 0.3, fontSize: 10,
    align: "right", color: C.muted, fontFace: "Consolas" });
  return s;
}

function shot(s, path, x, y, w) {
  const buf = fs.readFileSync(path);
  const dim = imageSize(buf);
  const h = w * dim.height / dim.width;
  s.addShape("rect", { x: x - 0.06, y: y - 0.06, w: w + 0.12, h: h + 0.12,
    fill: { color: C.panel2 }, line: { color: C.line, width: 1 } });
  s.addImage({ data: `image/png;base64,${buf.toString("base64")}`, x, y, w, h });
  return h;
}

function chips(s, items, x, y, w, opts = {}) {
  const gap = 0.12, h = opts.h || 0.92;
  const cw = (w - gap * (items.length - 1)) / items.length;
  items.forEach((it, i) => {
    s.addShape("roundRect", { x: x + i * (cw + gap), y, w: cw, h,
      fill: { color: C.panel }, line: { color: C.line, width: 1 }, rectRadius: 0.05 });
    s.addText(it.value, { x: x + i * (cw + gap), y: y + 0.08, w: cw, h: h * 0.52,
      fontSize: opts.big || 26, bold: true, color: it.color || C.accent, align: "center", fontFace: "Arial" });
    s.addText(it.label, { x: x + i * (cw + gap), y: y + h * 0.56, w: cw, h: h * 0.4,
      fontSize: 10.5, color: C.muted, align: "center", fontFace: "Arial" });
  });
}

function bullets(s, items, x, y, w, opts = {}) {
  const size = opts.size || 14;
  const rows = items.map(t => ({
    text: t, options: { bullet: { code: "25CF", indent: 14 }, color: C.text,
      paraSpaceAfter: opts.gap || 8, fontSize: size, fontFace: "Arial" },
  }));
  s.addText(rows, { x, y, w, h: opts.h || 4.5, valign: "top", lineSpacingMultiple: 1.12 });
}

// ---------- 1. Title ----------
{
  const s = pptx.addSlide();
  s.background = { color: C.bg };
  s.addShape("rect", { x: 0, y: 0, w: W, h: 0.2, fill: { color: C.accent } });
  s.addShape("rect", { x: 0, y: H - 0.2, w: W, h: 0.2, fill: { color: C.accent } });
  s.addText("COMPLEX  AUTOMATION  PRO", { x: 0.9, y: 1.15, w: 11.5, h: 0.5, fontSize: 15,
    color: C.accent, charSpacing: 6, fontFace: "Consolas" });
  s.addText("CAP", { x: 0.85, y: 1.7, w: 11.5, h: 1.7, fontSize: 96, bold: true, color: C.text, fontFace: "Arial" });
  s.addText("АСУ ТП участка флотационного обогащения руды", {
    x: 0.9, y: 3.55, w: 11.5, h: 0.7, fontSize: 28, bold: true, color: C.text, fontFace: "Arial" });
  s.addText("Диспетчерский контроль · металлургический баланс · супервизорное управление · тревоги ISA-18.2", {
    x: 0.9, y: 4.35, w: 11.5, h: 0.5, fontSize: 16, color: C.muted, fontFace: "Arial" });
  s.addText([
    { text: "Демонстрация MVP 1.0", options: { color: C.text, bold: true } },
    { text: "   ·   сентябрь 2026   ·   один сервер, реальный Modbus TCP, русский веб-интерфейс", options: { color: C.muted } },
  ], { x: 0.9, y: 6.35, w: 11.5, h: 0.5, fontSize: 14, fontFace: "Arial" });
}

// ---------- 2. Что это ----------
{
  const s = baseSlide(2, "Что такое CAP", "обзор");
  s.addText("SCADA-система участка обогащения в одном бинарнике", {
    x: 0.55, y: 1.15, w: 12.2, h: 0.5, fontSize: 18, color: C.muted, fontFace: "Arial" });
  chips(s, [
    { value: "1", label: "серверный процесс capd — UI, API, историк, тревоги, контуры", },
    { value: "43", label: "тега: 28 датчиков, 7 актуаторов, 8 расчётных" },
    { value: "1 Гц", label: "опрос Modbus TCP, задержка до экрана ≤ 2 с" },
    { value: "3", label: "супервизорные ПИД-контура АВТО/РУЧН" },
  ], 0.55, 1.9, 12.23, { big: 30 });
  bullets(s, [
    "Полный контур: контроллер → шлюз → сервер → экран оператора — только реальный протокол Modbus TCP, без «подрисованных» данных.",
    "Инженерные расчёты, а не украшения: двухпродуктовый баланс, извлечение ε, энергия Бонда, циркулирующая нагрузка — каждая формула показана в интерфейсе.",
    "Управление, а не только мониторинг: уставки, режимы, watchdog и запись воздействий в станцию (FC6) с аудитом.",
    "Тревоги по ISA-18.2: рационализированные уставки, приоритеты, квитирование, неизменяемый журнал.",
    "HMI по мотивам ISA-101: мнемосхема, faceplate, тренды, тёмная тема дежурного пункта.",
  ], 0.55, 3.35, 12.2, { size: 15, gap: 10 });
}

// ---------- 3. Архитектура ----------
{
  const s = baseSlide(3, "Архитектура: модульный монолит + шлюз + стенд", "схема");
  const box = (x, y, w, h, title, lines, color) => {
    s.addShape("roundRect", { x, y, w, h, fill: { color: C.panel }, line: { color: color || C.line, width: 1.25 }, rectRadius: 0.06 });
    s.addText(title, { x, y: y + 0.1, w, h: 0.42, fontSize: 15, bold: true, color: color || C.text, align: "center", fontFace: "Consolas" });
    s.addText(lines, { x: x + 0.12, y: y + 0.52, w: w - 0.24, h: h - 0.62, fontSize: 11.5, color: C.muted, align: "center", valign: "top", fontFace: "Arial", lineSpacingMultiple: 1.05 });
  };
  // capd
  box(4.05, 1.35, 5.2, 1.95, "capd — сервер :8000",
    "REST API · WebSocket · аутентификация и RBAC\nприём телеметрии (идемпотентно) · историк\nтревоги ISA-18.2 · ПИД-контуры · металлургия\nвстроенный веб-интерфейс (SPA)", C.accent);
  // edge
  box(4.05, 4.35, 5.2, 1.6, "edge — шлюз сбора",
    "опрос Modbus 1 Гц · буфер store-and-forward\nдоставка с подтверждением, без дублей\nFC6-мост записи актуаторов", C.green);
  // plantsim
  box(0.55, 4.35, 2.6, 1.6, "plantsim", "демо-стенд процесса\nModbus TCP :1502\n(в эксплуатации — ПЛК)", C.amber);
  // ПЛК label right
  box(10.2, 4.35, 2.6, 1.6, "завод", "контроллеры и датчики\nтого же протокола", C.amber);
  // arrows
  const arrow = (x1, y1, x2, y2, label, color) => {
    s.addShape("line", { x: x1, y: y1, w: x2 - x1, h: y2 - y1, line: { color, width: 2, endArrowType: "triangle" } });
    if (label) s.addText(label, { x: (x1 + x2) / 2 - 1.0, y: (y1 + y2) / 2 - 0.42, w: 2.0, h: 0.34,
      fontSize: 10.5, color: C.muted, align: "center", fontFace: "Consolas" });
  };
  arrow(1.9, 4.3, 4.6, 3.35, "Modbus TCP", C.green);
  arrow(6.6, 4.3, 6.6, 3.35, "HTTP batch · ingest", C.green);
  arrow(8.7, 3.35, 8.7, 4.3, "GET /control/output", C.accent);
  arrow(11.5, 4.3, 9.0, 3.35, "FC6 запись", C.accent);
  s.addText("Единственный путь данных: станция → Modbus → edge → ingest → capd → экран. Расчётные теги calc_* идут через тот же контракт.", {
    x: 0.55, y: 6.35, w: 12.2, h: 0.45, fontSize: 13, italic: true, color: C.muted, fontFace: "Arial" });
}

// ---------- 4. Мнемосхема (light) ----------
{
  const s = baseSlide(4, "Мнемосхема: процесс целиком на одном экране", "HMI");
  shot(s, `${A}/synoptic-light.png`, 0.8, 1.3, 8.3);
  bullets(s, [
    "Технологическая цепочка с живыми значениями: бункер → дробилка → мельница + гидроциклон → флотация → сгуститель → фильтр → отгрузка",
    "Цвет узла = состояние: норма / тревога / нет связи",
    "Клик по узлу — паспорт объекта, клик по метрике — тренд",
    "Обновление в реальном времени через WebSocket",
  ], 9.35, 1.6, 3.45, { size: 12.5, gap: 9 });
}

// ---------- 5. Тёмная тема ----------
{
  const s = baseSlide(5, "Тёмная тема дежурного пункта (ISA-101)", "HMI");
  shot(s, `${A}/synoptic-dark.png`, 0.8, 1.3, 8.3);
  bullets(s, [
    "Приглушённая палитра — глаз оператора не «выгорает» за смену",
    "Цвет используется только для смысла: состояние и поток, не декор",
    "Обе темы работают на всех страницах без исключений",
    "Системные шрифты — работа в закрытом контуре (air-gap), без CDN",
  ], 9.35, 1.6, 3.45, { size: 12.5, gap: 9 });
}

// ---------- 6. Данные ----------
{
  const s = baseSlide(6, "Данные: реальный протокол и честная надёжность", "платформа");
  chips(s, [
    { value: "28 + 7", label: "датчиков и актуаторов в карте регистров" },
    { value: "1 с", label: "период опроса и шаг историка" },
    { value: "30 сут", label: "глубина историка при SQLite WAL" },
    { value: "0", label: "дублей после обрыва — идемпотентная доставка" },
  ], 0.55, 1.5, 12.23, { big: 28 });
  bullets(s, [
    "Карта регистров Modbus (FC3/FC4/FC6) — контракт со станцией: адрес, масштаб, шкала инженерных единиц для каждого тега.",
    "Обрыв связи станция ↔ шлюз: качество offline, разрыв линии на тренде — фиктивных значений нет.",
    "Обрыв шлюз ↔ сервер: буфер SQLite store-and-forward, после восстановления — полная досылка без дублей и потери порядка.",
    "Актуаторы при потере сервера удерживают последние значения, а не сбрасываются в ноль (инстинкт IEC 61511).",
    "Выгрузки CSV за период — отчётность без ручного копирования.",
  ], 0.55, 2.95, 12.2, { size: 15, gap: 10 });
}

// ---------- 7. Металлургия ----------
{
  const s = baseSlide(7, "Металлургия: баланс, который можно проверить", "расчёты");
  const h = shot(s, `${A}/metallurgy.png`, 0.8, 1.45, 8.3);
  bullets(s, [
    "ε ≈ 88–90 % в номинале — и формула на карточке: ε = β(α−θ)/(α(β−θ))×100",
    "Невязка баланса ≤ ±1 %, иначе — предупреждение «проверь XRF/весы»",
    "Энергия Бонда, КПД мельницы, циркулирующая нагрузка, удельные расходы",
    "Источники каждого показателя указаны — расчёт аудируется",
    "Целевые коридоры — из активного профиля руды",
  ], 9.35, 1.7, 3.45, { size: 12.5, gap: 9 });
}

// ---------- 8. Управление ----------
{
  const s = baseSlide(8, "Управление: супервизорные ПИД-контуры", "управление");
  shot(s, `${A}/control.png`, 0.8, 1.45, 8.3);
  bullets(s, [
    "Три контура: уровень флотомашины lic301, расход собирателя fic301, плотность сгущения dic401",
    "Режимы АВТО/РУЧН; любая запись — с подтверждением и записью в аудит",
    "Безбамперный переход в АВТО — от фактической позиции актуатора",
    "Watchdog 10 с: нет достоверного PV → РУЧН, выход заморожен, тревога",
    "В станцию пишет только шлюз (FC6) и только в АВТО",
  ], 9.35, 1.7, 3.45, { size: 12.5, gap: 9 });
}

// ---------- 9. Тревоги ----------
{
  const s = baseSlide(9, "Тревоги по ISA-18.2", "тревоги");
  shot(s, `${A}/alarms.png`, 0.8, 1.45, 8.3);
  bullets(s, [
    "Рационализированные уставки: lo_lo / lo / hi / hi_hi с приоритетами",
    "Задержка 3 с (критические) и 10 с, гистерезис 1 % шкалы — без «дребезга»",
    "Квитирование от имени оператора (из сессии) + комментарий",
    "Неизменяемый журнал: сработала / сброшена / квитирована",
    "comm_loss по активу при потере данных > 10 с; критическая тревога — баннер на всех страницах",
  ], 9.35, 1.7, 3.45, { size: 12.5, gap: 9 });
}

// ---------- 10. Faceplate ----------
{
  const s = baseSlide(10, "Паспорт объекта: всё по узлу — в одном окне", "HMI");
  shot(s, `${A}/faceplate.png`, 0.8, 1.45, 8.3);
  bullets(s, [
    "Клик по узлу мнемосхемы — модальный паспорт объекта",
    "Все параметры узла с качеством каждого значения (good / offline)",
    "Панели контуров узла: PV / SP / MV, полоса положения, кнопки с подтверждением",
    "Уставки сигнализации — в разделе «Тревоги» (ISA-18.2)",
  ], 9.35, 1.7, 3.45, { size: 12.5, gap: 9 });
}

// ---------- 11. Безопасность ----------
{
  const s = baseSlide(11, "Безопасность и подотчётность", "платформа");
  const row = (y, title, text) => {
    s.addShape("roundRect", { x: 0.55, y, w: 12.23, h: 1.02, fill: { color: C.panel }, line: { color: C.line, width: 1 }, rectRadius: 0.05 });
    s.addText(title, { x: 0.85, y: y + 0.09, w: 3.4, h: 0.84, fontSize: 15, bold: true, color: C.accent, valign: "middle", fontFace: "Arial" });
    s.addText(text, { x: 4.4, y: y + 0.09, w: 8.2, h: 0.84, fontSize: 12.5, color: C.text, valign: "middle", fontFace: "Arial", lineSpacingMultiple: 1.05 });
  };
  row(1.45, "Аутентификация", "Локальные учётные записи (PBKDF2-SHA256), серверные сессии в HttpOnly-cookie на 12 ч, выход по кнопке.");
  row(2.62, "Роли (RBAC)", "Оператор: квитирование, уставки, сценарии. Администратор: реестр, доступ, аудит. Права проверяются на API.");
  row(3.79, "Аудит", "Каждая мутация — с субъектом из сессии и меткой времени; команды управления только с явным подтверждением.");
  row(4.96, "Машины и человек", "Шлюзы аутентифицируются отдельным секретом GATEWAY_TOKEN; пользовательские сессии и устройства разделены.");
  row(6.13, "Граница безопасности", "Супервизорный контур не заменяет противоаварийную защиту: при потере данных воздействие замораживается (IEC 61511).");
}

// ---------- 12. Демо ----------
{
  const s = baseSlide(12, "Демонстрация: 15 минут, 7 эпизодов", "сценарий");
  const steps = [
    ["1", "Нормальный режим", "баланс сходится, ε ≈ 90 %, тренды живые"],
    ["2", "Контур в АВТО", "SP 500→550 мм: задвижка отрабатывает, уровень выходит на уставку"],
    ["3", "Возмущение по руде", "крепче руда → P80 растёт → ε падает → тревога по хвостам; оператор: вода мельницы + расход собирателя → ε восстановлено"],
    ["4", "Обрыв связи", "offline и разрывы трендов → восстановление; буфер шлюза досылает всё без дублей"],
    ["5", "Отказ насоса сгустителя", "постель и момент растут → hi_hi; оператор берёт контур в РУЧН → тревоги очищаются"],
    ["6", "Дрейф XRF", "невязка уходит в предупреждение — встроенный детектор несогласованности"],
    ["7", "Отчёты", "выгрузка показаний и тревог в CSV"],
  ];
  steps.forEach((st, i) => {
    const y = 1.4 + i * 0.72;
    s.addShape("ellipse", { x: 0.62, y: y + 0.08, w: 0.5, h: 0.5, fill: { color: C.panel2 }, line: { color: C.accent, width: 1.25 } });
    s.addText(st[0], { x: 0.62, y: y + 0.08, w: 0.5, h: 0.5, fontSize: 16, bold: true, color: C.accent, align: "center", valign: "middle", fontFace: "Consolas" });
    s.addText([
      { text: st[1] + "   ", options: { bold: true, color: C.text } },
      { text: st[2], options: { color: C.muted } },
    ], { x: 1.35, y, w: 11.4, h: 0.68, fontSize: 13.5, valign: "middle", fontFace: "Arial" });
  });
}

// ---------- 13. Характеристики ----------
{
  const s = baseSlide(13, "Характеристики и статус проекта", "итоги");
  chips(s, [
    { value: "≤ 2 с", label: "датчик → экран" },
    { value: "≥ 100/с", label: "поток значений в приёме" },
    { value: "≤ 200 МБ", label: "память сервера" },
    { value: "≤ 30 с", label: "холодный старт демо" },
  ], 0.55, 1.45, 12.23, { big: 26 });
  s.addText([
    { text: "Готово сегодня", options: { fontSize: 17, bold: true, color: C.green, breakLine: true } },
    { text: "Сбор, историк, металлургия, три контура управления, тревоги ISA-18.2, полный HMI, RBAC и аудит, демо-скрипт — приёмочные тесты зелёные (go test, сборка фронтенда).", options: { fontSize: 13.5, color: C.text, breakLine: true } },
    { text: "", options: { breakLine: true, fontSize: 8 } },
    { text: "Осознанные границы MVP", options: { fontSize: 17, bold: true, color: C.amber, breakLine: true } },
    { text: "Один узел без кластеризации · локальная аутентификация (OIDC/LDAP — далее) · без downsampling историка · Modbus TCP в демо-контуре (OPC UA/Sparkplug — в коде шлюза).", options: { fontSize: 13.5, color: C.text, breakLine: true } },
    { text: "", options: { breakLine: true, fontSize: 8 } },
    { text: "Следующие шаги", options: { fontSize: 17, bold: true, color: C.accent, breakLine: true } },
    { text: "Пилот на реальном контроллере → PostgreSQL и резервирование → нагрузочные испытания → подключение OPC UA.", options: { fontSize: 13.5, color: C.text } },
  ], { x: 0.55, y: 2.75, w: 12.2, h: 3.9, valign: "top", lineSpacingMultiple: 1.12, fontFace: "Arial" });
}

pptx.writeFile({ fileName: OUT }).then(() => console.log("written:", OUT));

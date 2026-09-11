// Inspirational presentation for CAP performance. Запуск: node generate-inspiring-deck.js
const pptxgen = require("pptxgenjs");
const fs = require("fs");
const { imageSize } = require("image-size");

const A = "/home/narziev/Documents/AutoPro/docs-build/assets";
const OUT = "/home/narziev/Documents/AutoPro/docs/Презентация_Выступление.pptx";

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
pptx.title = "CAP — Вдохновляющая презентация для выступления";

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
  s.addText("Вдохновляющая презентация для выступления", {
    x: 0.9, y: 3.55, w: 11.5, h: 0.7, fontSize: 28, bold: true, color: C.text, fontFace: "Arial" });
  s.addText("Преобразуем обогащение руды через интеллектуальную автоматизацию", {
    x: 0.9, y: 4.35, w: 11.5, h: 0.5, fontSize: 16, color: C.muted, fontFace: "Arial" });
  s.addText([
    { text: "Демонстрация возможностей", options: { color: C.text, bold: true } },
    { text: "   ·   сентябрь 2026   ·   один сервер, реальный Modbus TCP, русский веб-интерфейс", options: { color: C.muted } },
  ], { x: 0.9, y: 6.35, w: 11.5, h: 0.5, fontSize: 14, fontFace: "Arial" });
}

// ---------- 2. Inspirational Quote ----------
{
  const s = baseSlide(2, "Вдохновение", "цитата");
  s.addText("\"Автоматизация — это не замена человека, а усиление его способностей.\"", {
    x: 0.55, y: 2.0, w: 12.2, h: 1.2, fontSize: 24, align: "center", color: C.text, fontFace: "Arial", lineSpacingMultiple: 1.2 });
  s.addText("— Неизвестный инженер", { x: 0.55, y: 3.5, w: 12.2, h: 0.5, fontSize: 16, align: "center", color: C.muted, fontFace: "Arial" });
}

// ---------- 3. Problem ----------
{
  const s = baseSlide(3, "Вызов современного обогащения", "проблема");
  bullets(s, [
    "Низкая извлекаемость полезных компонентов из-за колебаний сырья",
    "Задержки в получении данных prowadят к несвоевременным решениям",
    "Высокие потери энергии и реагентов из‑за неоптимального управления",
    "Сложность интеграции разнородного оборудования (ПЛК, датчики, анализаторы)",
    "Недостаток прозрачности расчётов — сложно аудировать и улучшать процесс"
  ], 0.55, 1.5, 12.2, { size: 15, gap: 10 });
}

// ---------- 4. Solution: CAP ----------
{
  const s = baseSlide(4, "Решение: CAP — интеллектуальная автоматизация", "решение");
  bullets(s, [
    "Полный контур: датчик → шлюз → сервер → исполнительные механизмы (Modbus FC6)",
    "Реальное время: задержка ≤ 2 с от измерения до действия",
    "Металлургический баланс с аудируемыми формулами — прозрачность и доверие",
    "Три супервизорных ПИД‑контура с адаптивной настройкой и защитой",
    "Тревоги по ISA‑18.2: рационализированные уставки, неизменяемый журнал",
    "Надёжная телеметрия: store‑and‑forward, идемпотентная доставка",
    "Безопасность: RBAC, аудит, машинная аутентификация шлюзов",
    "Гибкость: ПИД‑алгоритмы могут работать на сервере, внешних ПЛК или ESP32"
  ], 0.55, 1.5, 12.2, { size: 14, gap: 9 });
}

// ---------- 5. Architectural Scheme ----------
{
  const s = baseSlide(5, "Схема взаимодействия CAP", "схема");
  // Draw blocks similar to earlier architecture but simplified
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
  // plantsim (demo stand)
  box(0.55, 4.35, 2.6, 1.6, "plantsim", "демо-стенд процесса\nModbus TCP :1502\n(в эксплуатации — ПЛК)", C.amber);
  // Plant label right
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

// ---------- 6. Benefits ----------
{
  const s = baseSlide(6, "Преимущества CAP", "выгоды");
  bullets(s, [
    "Увеличение извлечения ε до 90 % и более благодаря точному управлению",
    "Снижение удельных расходов реагентов и энергии за счёт оптимизации",
    "Повышение надёжности процесса: отсутствие фиктивных данных при обрывах",
    "Уменьшение простоев благодаря быстрой диагностике тревог и автоматическому восстановлению",
    "Прозрачность и аудируемость: каждый KPI сопровождается формулой и источниками данных",
    "Масштабируемость: возможность добавлять OPC UA, Sparkplug, кластеризацию",
    "Соответствие международным стандартам: ISA‑18.2, IEC 61511, ISA‑101"
  ], 0.55, 1.5, 12.2, { size: 14, gap: 9 });
}

// ---------- 7. Call to Action ----------
{
  const s = baseSlide(7, "Приглашение к действию", "призыв");
  bullets(s, [
    "Запустить демо одной командой: cd backend && ./run.sh",
    "Оценить работу системы на http://127.0.0.1:8000 (логин operator/operator)",
    "Пройти скриптованную демонстрацию: demo/run-demo.sh (~15 мин)",
    "Изучить техническую документацию: технический паспорт, руководство оператора",
    "Присоединиться к развитию CAP: открытый код, лицензия MIT, сообщество разработчиков"
  ], 0.55, 1.5, 12.2, { size: 14, gap: 9 });
  s.addText("Вместе мы сделаем обогащение руды более эффективным, чистым и безопасным.", {
    x: 0.55, y: 5.0, w: 12.2, h: 0.6, fontSize: 18, bold: true, align: "center", color: C.green });
}

// ---------- 8. Closing ----------
{
  const s = pptx.addSlide();
  s.background = { color: C.bg };
  s.addShape("rect", { x: 0, y: 0, w: W, h: 0.2, fill: { color: C.accent } });
  s.addShape("rect", { x: 0, y: H - 0.2, w: W, h: 0.2, fill: { color: C.accent } });
  s.addText("Спасибо за внимание!", { x: 0.9, y: 2.5, w: 11.5, h: 1.0, fontSize: 36, bold: true, align: "center", color: C.text, fontFace: "Arial" });
  s.addText("Вопросы и обсуждение", { x: 0.9, y: 4.0, w: 11.5, h: 0.6, fontSize: 24, align: "center", color: C.muted, fontFace: "Arial" });
  s.addText("Контакт: cap@example.com", { x: 0.9, y: 5.0, w: 11.5, h: 0.4, fontSize: 16, align: "center", color: C.muted, fontFace: "Arial" });
}

pptx.writeFile({ fileName: OUT }).then(() => console.log("written:", OUT));
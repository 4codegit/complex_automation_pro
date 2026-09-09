// CAP — Руководство по эксплуатации и технический паспорт (RU)
// Cover: recipe R1 (report/tech) + DM-1 palette. 3-section numbering:
// cover (no footer) -> TOC (Roman) -> body (Arabic from 1).
const {
  Document, Packer, Paragraph, TextRun, Table, TableRow, TableCell,
  ImageRun, PageBreak, Header, Footer, PageNumber, NumberFormat,
  AlignmentType, HeadingLevel, WidthType, BorderStyle, ShadingType,
  SectionType, TableOfContents, TableLayoutType,
} = require("docx");
const fs = require("fs");

// ── Palette: DM-1 (Deep Cyan, tech) ─────────────────────────────────────────
const P = {
  bg: "162235", accent: "37DCF2",
  cover: { titleColor: "FFFFFF", subtitleColor: "B0B8C0", metaColor: "90989F", footerColor: "687078" },
  table: { headerBg: "1B6B7A", headerText: "FFFFFF", accentLine: "1B6B7A", innerLine: "C8DDE2", surface: "EDF3F5" },
  primary: "0A1628", body: "000000", secondary: "506070",
};

const NB = { style: BorderStyle.NONE, size: 0, color: "FFFFFF" };
const noBorders = { top: NB, bottom: NB, left: NB, right: NB };
const allNoBorders = { top: NB, bottom: NB, left: NB, right: NB, insideHorizontal: NB, insideVertical: NB };

// Latin/Cyrillic char width ≈ pt*11 twips (half of CJK)
function calcTitleLayout(title, maxWidthTwips, preferredPt = 40, minPt = 24) {
  const charsPerLine = (pt) => Math.floor(maxWidthTwips / (pt * 11));
  let titlePt = preferredPt;
  let lines = [title];
  while (titlePt >= minPt) {
    const cpl = charsPerLine(titlePt);
    if (cpl >= title.length) { lines = [title]; break; }
    lines = splitLines(title, cpl);
    if (lines.length <= 3) break;
    titlePt -= 2;
  }
  return { titlePt, titleLines: lines };
}
function splitLines(title, cpl) {
  const words = title.split(" ");
  const lines = [];
  let cur = "";
  for (const w of words) {
    if ((cur + " " + w).trim().length <= cpl || cur === "") cur = (cur + " " + w).trim();
    else { lines.push(cur); cur = w; }
  }
  if (cur) lines.push(cur);
  return lines;
}
function calcCoverSpacing(p) {
  const SAFETY = 1200;
  const usable = 16838 - SAFETY;
  const titleH = p.titleLineCount * (p.titlePt * 23 + 200);
  const subH = p.hasSubtitle ? (12 * 23 + 600) : 0;
  const engH = p.hasEnglishLabel ? (9 * 23 + 600) : 0;
  const metaH = p.metaLineCount * (10 * 23 + 100);
  const content = titleH + subH + engH + metaH + (p.fixedHeight || 400) + 900;
  const remaining = Math.max(usable - content, 400);
  const FOOTER_MIN = 800;
  const rawTop = Math.floor(remaining * 0.45);
  const rawBottom = Math.floor(remaining * 0.45);
  const bottomSpacing = Math.max(rawBottom, FOOTER_MIN);
  const topSpacing = Math.max(rawTop - Math.max(0, FOOTER_MIN - rawBottom), 400);
  return { topSpacing, bottomSpacing };
}

// ── Cover R1 ────────────────────────────────────────────────────────────────
function buildCoverR1(config) {
  const padL = 1200, padR = 800;
  const availableWidth = 11906 - padL - padR - 300;
  const { titlePt, titleLines } = calcTitleLayout(config.title, availableWidth, 40, 24);
  const titleSize = titlePt * 2;
  const spacing = calcCoverSpacing({
    titleLineCount: titleLines.length, titlePt,
    hasSubtitle: !!config.subtitle, hasEnglishLabel: !!config.englishLabel,
    metaLineCount: (config.metaLines || []).length, fixedHeight: 400,
  });
  const accentLeft = { style: BorderStyle.SINGLE, size: 8, color: P.accent, space: 12 };
  const children = [];
  children.push(new Paragraph({ spacing: { before: spacing.topSpacing } }));
  if (config.englishLabel) {
    children.push(new Paragraph({
      indent: { left: padL, right: padR }, spacing: { after: 500 },
      border: { bottom: { style: BorderStyle.SINGLE, size: 6, color: P.accent, space: 8 } },
      children: [new TextRun({ text: config.englishLabel.split("").join("  "), size: 18, color: P.accent, font: { ascii: "Arial" }, characterSpacing: 40 })],
    }));
  }
  titleLines.forEach((line, i) => {
    children.push(new Paragraph({
      indent: { left: padL },
      spacing: { after: i < titleLines.length - 1 ? 100 : 300, line: Math.ceil(titlePt * 23), lineRule: "atLeast" },
      children: [new TextRun({ text: line, size: titleSize, bold: true, color: P.cover.titleColor, font: { ascii: "Arial", eastAsia: "Arial" } })],
    }));
  });
  if (config.subtitle) {
    children.push(new Paragraph({
      indent: { left: padL }, spacing: { after: 800 },
      children: [new TextRun({ text: config.subtitle, size: 24, color: P.cover.subtitleColor, font: { ascii: "Arial" } })],
    }));
  }
  for (const line of (config.metaLines || [])) {
    children.push(new Paragraph({
      indent: { left: padL + 200 }, spacing: { after: 80 },
      border: { left: accentLeft },
      children: [new TextRun({ text: line, size: 24, color: P.cover.metaColor, font: { ascii: "Arial" } })],
    }));
  }
  children.push(new Paragraph({ spacing: { before: spacing.bottomSpacing } }));
  children.push(new Paragraph({
    indent: { left: padL, right: padR },
    border: { top: { style: BorderStyle.SINGLE, size: 2, color: P.accent, space: 8 } },
    spacing: { before: 200 },
    children: [
      new TextRun({ text: config.footerLeft || "", size: 16, color: P.cover.footerColor }),
      new TextRun({ text: "                                                            " }),
      new TextRun({ text: config.footerRight || "", size: 16, color: P.cover.footerColor }),
    ],
  }));
  return [new Table({
    width: { size: 100, type: WidthType.PERCENTAGE },
    layout: TableLayoutType.FIXED,
    borders: allNoBorders,
    rows: [new TableRow({
      height: { value: 16838, rule: "exact" },
      children: [new TableCell({ shading: { type: ShadingType.CLEAR, fill: P.bg }, borders: noBorders, verticalAlign: "top", children })],
    })],
  })];
}

// ── Body helpers ────────────────────────────────────────────────────────────
const F = { ascii: "Times New Roman", eastAsia: "Times New Roman" };
function h1(text) {
  return new Paragraph({
    heading: HeadingLevel.HEADING_1, spacing: { before: 360, after: 160, line: 312 },
    children: [new TextRun({ text, bold: true, size: 32, color: P.primary, font: F })],
  });
}
function h2(text) {
  return new Paragraph({
    heading: HeadingLevel.HEADING_2, spacing: { before: 240, after: 120, line: 312 },
    children: [new TextRun({ text, bold: true, size: 28, color: P.primary, font: F })],
  });
}
function body(text, opts = {}) {
  return new Paragraph({
    alignment: AlignmentType.JUSTIFIED,
    indent: { firstLine: 709 },
    spacing: { line: 312, after: opts.after ?? 80 },
    children: [new TextRun({ text, size: 24, color: P.body, font: F })],
  });
}
function bodyRuns(runs, opts = {}) {
  return new Paragraph({
    alignment: AlignmentType.JUSTIFIED,
    indent: { firstLine: 709 },
    spacing: { line: 312, after: opts.after ?? 80 },
    children: runs.map(([text, extra]) => new TextRun({ text, size: 24, color: P.body, font: F, ...(extra || {}) })),
  });
}
function bullet(text) {
  return new Paragraph({
    bullet: { level: 0 }, spacing: { line: 312, after: 40 },
    children: [new TextRun({ text, size: 24, color: P.body, font: F })],
  });
}
function mono(text) {
  return new Paragraph({
    spacing: { line: 312, after: 40 }, indent: { left: 709 },
    shading: { type: ShadingType.CLEAR, fill: P.table.surface },
    children: [new TextRun({ text, size: 21, font: { ascii: "Courier New", eastAsia: "Courier New" }, color: P.primary })],
  });
}
function caption(text) {
  return new Paragraph({
    keepNext: true, spacing: { before: 120, after: 80, line: 312 },
    children: [new TextRun({ text, bold: true, size: 21, color: P.secondary, font: F })],
  });
}
function img(path, widthPx, heightPx, capText) {
  return [
    new Paragraph({
      alignment: AlignmentType.CENTER, spacing: { before: 120, after: 40 },
      children: [new ImageRun({ data: fs.readFileSync(path), transformation: { width: widthPx, height: heightPx }, type: "png" })],
    }),
    new Paragraph({
      alignment: AlignmentType.CENTER, spacing: { after: 160 },
      children: [new TextRun({ text: capText, size: 20, italics: true, color: P.secondary, font: F })],
    }),
  ];
}
function tbl(headers, rows, widths) {
  return new Table({
    width: { size: 100, type: WidthType.PERCENTAGE },
    borders: {
      top: { style: BorderStyle.SINGLE, size: 4, color: P.table.accentLine },
      bottom: { style: BorderStyle.SINGLE, size: 4, color: P.table.accentLine },
      left: NB, right: NB,
      insideHorizontal: { style: BorderStyle.SINGLE, size: 1, color: P.table.innerLine },
      insideVertical: NB,
    },
    rows: [
      new TableRow({
        tableHeader: true, cantSplit: true,
        children: headers.map((text, i) => new TableCell({
          children: [new Paragraph({ spacing: { line: 276 }, children: [new TextRun({ text, bold: true, size: 21, color: P.table.headerText, font: F })] })],
          shading: { type: ShadingType.CLEAR, fill: P.table.headerBg },
          margins: { top: 60, bottom: 60, left: 120, right: 120 },
          width: { size: widths[i], type: WidthType.PERCENTAGE },
        })),
      }),
      ...rows.map((cells, ri) => new TableRow({
        cantSplit: true,
        children: cells.map((text, i) => new TableCell({
          children: [new Paragraph({ spacing: { line: 276 }, children: [new TextRun({ text: String(text), size: 21, color: P.body, font: F })] })],
          shading: { type: ShadingType.CLEAR, fill: ri % 2 === 0 ? "FFFFFF" : P.table.surface },
          margins: { top: 60, bottom: 60, left: 120, right: 120 },
          width: { size: widths[i], type: WidthType.PERCENTAGE },
        })),
      })),
    ],
  });
}
function pageNumFooter() {
  return new Footer({
    children: [new Paragraph({
      alignment: AlignmentType.CENTER,
      children: [new TextRun({ children: [PageNumber.CURRENT], size: 18, color: P.secondary, font: F })],
    })],
  });
}
const docHeader = new Header({
  children: [new Paragraph({
    alignment: AlignmentType.RIGHT,
    border: { bottom: { style: BorderStyle.SINGLE, size: 2, color: P.table.innerLine, space: 4 } },
    children: [new TextRun({ text: "CAP — Руководство по эксплуатации", size: 16, color: P.secondary, font: F })],
  })],
});

// ── Body content ────────────────────────────────────────────────────────────
const IMG_W = 580, IMG_H = Math.round(580 * 1000 / 1680); // screenshots 1680x1000
const A = "assets/";

const bodyChildren = [
  h1("1. Общие сведения"),
  body("Complex Automation Pro (CAP) — программная платформа мониторинга технологических процессов обогатительной фабрики. Платформа собирает телеметрию с датчиков и контроллеров через edge-шлюзы по промышленным протоколам (Modbus TCP, OPC UA, MQTT Sparkplug B), хранит историю измерений, вычисляет производные технологические показатели, отслеживает аварии и предоставляет оператору веб-панель контроля."),
  body("CAP является системой наблюдения (уровень 3 по модели ISA-95) и не осуществляет управления технологическим оборудованием: приём данных — только чтение, контуры регулирования, блокировки и аварийные остановы остаются в независимых системах управления. Потеря или компрометация CAP не останавливает производство."),
  caption("Таблица 1. Сведения об изделии"),
  tbl(
    ["Параметр", "Значение"],
    [
      ["Наименование", "Complex Automation Pro (CAP)"],
      ["Версия", "MVP (ветка main репозитория)"],
      ["Правообладатель", "4codegit"],
      ["Лицензия", "MIT (файл LICENSE)"],
      ["Репозиторий", "github.com/4codegit/complex_automation_pro"],
      ["Языки реализации", "Go (сервер, шлюзы), TypeScript/React (панель)"],
      ["Статус", "Опытная эксплуатация, режим разработки"],
    ],
    [35, 65],
  ),

  h1("2. Состав и архитектура системы"),
  body("Система состоит из центральной платформы и пограничных шлюзов. Центральная платформа развёртывается в двух режимах из одного кода: монолитном (разработка и демонстрация, один процесс cmd/server) либо микросервисном — семь независимых сервисов за единой входной точкой gateway-api."),
  caption("Таблица 2. Компоненты системы (микросервисный режим)"),
  tbl(
    ["Компонент", "Порт", "Назначение"],
    [
      ["gateway-api", "8000", "Единая входная точка: маршрутизация API и дашборда"],
      ["live", "8001", "Веб-панель, WebSocket-поток, симулятор (только разработка)"],
      ["ingest", "8002", "Приём телеметрии от шлюзов, идемпотентность, коды качества"],
      ["historian", "8003", "История измерений, агрегаты, CSV-отчёты, аналитика"],
      ["alarms", "8004", "Аварии: состояния, квитирование, уставки"],
      ["profiles", "8005", "Профили руды: черновик — согласование — активация"],
      ["registry", "8006", "Реестр оборудования, сигналов и шлюзов"],
      ["identity", "8007", "Роли доступа (RBAC), назначения, журнал аудита"],
      ["gateway (edge)", "—", "Опрос приборов, буфер store-and-forward, доставка на сервер"],
    ],
    [22, 10, 68],
  ),
  body("Буферизация на краю сети (store-and-forward) гарантирует сохранность данных при потере связи: каждое сообщение пишется в локальную базу SQLite (WAL) и доставляется на сервер пакетами с повторными попытками; после восстановления связи очередь полностью досылается."),

  h1("3. Требования к вычислительной среде"),
  caption("Таблица 3. Минимальные требования"),
  tbl(
    ["Компонент", "Требование"],
    [
      ["Операционная система", "Linux x86-64 (проверено на Fedora; совместимо с любым современным дистрибутивом)"],
      ["Среда выполнения Go", "Go 1.22 или новее (сборка из исходников)"],
      ["Сборка панели", "Node.js 18+ и npm (только при изменении интерфейса)"],
      ["База данных", "SQLite (встроенная, по умолчанию) или PostgreSQL"],
      ["Сеть", "TCP: порты 8000–8007 (режим микросервисов) либо 8000 (монолит); доступ шлюзов к приборам по сети"],
    ],
    [32, 68],
  ),

  h1("4. Установка и запуск"),
  h2("4.1. Быстрый старт (монолитный режим)"),
  body("Сборка и запуск одной командой — скрипт собирает веб-панель, встраивает её в бинарник сервера, запускает сервер и демонстрационный шлюз, затем проверяет готовность:"),
  mono("cd backend && ./run.sh"),
  body("После сообщения о готовности панель доступна по адресу http://127.0.0.1:8000. Остановка: fuser -k 8000/tcp. Альтернатива без пересборки панели и шлюза: scripts/start.sh (только сервер)."),
  h2("4.2. Микросервисный режим"),
  body("Запуск восьми независимых процессов (live → остальные сервисы → gateway-api; сервис live первым выполняет миграции базы):"),
  mono("cd backend && scripts/services.sh start"),
  body("Управление: scripts/services.sh status — состояние процессов; scripts/services.sh stop — остановка. Все сервисы отвечают через единый адрес http://127.0.0.1:8000; внешне режим неотличим от монолита."),
  h2("4.3. Подключение реального Modbus-устройства"),
  body("Скрипт отключает демонстрационный симулятор и направляет шлюз на указанный Modbus TCP-источник (планшет с приложением-симулятором, контроллер, преобразователь):"),
  mono("cd backend && scripts/phone-modbus.sh <IP-адрес>[:порт]"),
  body("Карта регистров демо-источника приведена в разделе 6. Изменение значения регистра отражается на панели в течение двух секунд; при потере связи историк фиксирует интервалы «нет связи» (quality=offline), восстановление — автоматическое."),

  h1("5. Интерфейс оператора"),
  body("Панель — одностраничное приложение с боковой навигацией: Обзор, Аналитика, Аварии, Профили, Настройки, Инструктор. Тема оформления переключается внизу бокового меню (по умолчанию светлая)."),
  h2("5.1. Страница «Обзор»"),
  body("Сводное состояние фабрики: карточки технологических участков с текущими значениями и индикаторами качества каждого сигнала, живой график телеметрии по всем метрикам. Карточка участка подсвечивается при выходе значения за пределы, заданные активным профилем руды."),
  ...img(A + "overview-light.png", IMG_W, IMG_H, "Рисунок 1. Страница «Обзор»: участки, живая телеметрия"),
  h2("5.2. Страница «Аналитика»"),
  body("Расчётные технологические показатели, вычисляемые на сервере из последних измерений и активного профиля руды: содержание твёрдого в пульпе, удельный расход реагента, сухая производительность, статусы коридоров pH, крупности P80 и влажности. Формула каждого показателя выводится на карточке; отсутствие исходных данных отображается явно — значения не подменяются оценками."),
  ...img(A + "analytics-light.png", IMG_W, IMG_H, "Рисунок 2. Страница «Аналитика»: расчётные показатели и статусы коридоров"),
  h2("5.3. Страница «Аварии»"),
  body("Активные аварии и алермы в стиле ISA-18.2 с приоритетами (critical/high/medium), журналом тревог и квитированием. Квитирование выполняет аутентифицированный оператор (поле «Оператор»); событие фиксируется в неизменяемом журнале аудита."),
  ...img(A + "alarms-light.png", IMG_W, IMG_H, "Рисунок 3. Страница «Аварии»"),
  h2("5.4. Страница «Профили»"),
  body("Версионированные профили руды задают технологические коридоры и уставки. Жизненный цикл: черновик — согласование — активация. Активный профиль определяет пороги, по которым оцениваются коридоры на странице «Аналитика», и уставки демонстрационного симулятора."),
  h2("5.5. Страница «Настройки»"),
  body("Администрирование платформы: реестр оборудования и сигналов (добавление участков и датчиков с выбором типа — температура, pH, плотность, расход, уровень, влажность и другие), история телеметрии с контролем качества, роли доступа и назначения (RBAC), журнал аудита. Регистрация сигнала делает его доступным приёму; источник данных указывается в конфигурации шлюза (раздел 6)."),
  ...img(A + "settings-light.png", IMG_W, IMG_H, "Рисунок 4. Страница «Настройки»: реестр, датчики, доступ"),

  h1("6. Подключение источников данных"),
  body("Шлюз опрашивает приборы по драйверу, выбираемому переменной SOURCE_DRIVER: simulated (демо-генератор), opcua (poll/subscribe), modbus (Modbus TCP, только чтение, функции 1–4), sparkplug (MQTT Sparkplug B, роль host-приложения). Каждый сигнал описывается в переменной TAGS в формате «идентификатор_тега:единица|адрес_источника»."),
  caption("Таблица 4. Синтаксис адреса источника по драйверам"),
  tbl(
    ["Драйвер", "Пример TAGS", "Пояснение"],
    [
      ["modbus", "plant-a.grinding.mill_power:kW|reg=hr:100:f32", "область hr|ir|c|di, адрес, тип bool/u16/i16/u32/i32/f32 (суффикс sw — перестановка слов), необязательный масштаб"],
      ["opcua", "…:unit|node=ns=2;s=Sim.PV", "NodeId OPC UA"],
      ["sparkplug", "…:unit|sp=group/edge/metric", "путь метрики Sparkplug B"],
    ],
    [14, 44, 42],
  ),
  body("Подключение сервера к OPC UA с политиками безопасности Sign/SignAndEncrypt и аутентификацией по сертификату x509 описано в ARCHITECTURE_DECISIONS.md (ADR-001). Драйверы не содержат путей записи — граница «только чтение» соблюдена на уровне кода."),
  body("Демонстрационная карта регистров для Modbus-симулятора на планшете:"),
  caption("Таблица 5. Демо-карта holding-регистров (масштаб применяется автоматически)"),
  tbl(
    ["Регистр", "Тег", "Показатель", "Введите", "На панели"],
    [
      ["HR 0", "plant-a.flotation.ph_level", "pH флотации", "958", "9.58 pH"],
      ["HR 2", "plant-a.crushing.pulp_density", "Плотность пульпы", "1650", "1.650 г/см³"],
      ["HR 4", "plant-a.dewatering.dryer_temperature", "Температура сушки", "1413", "141.3 °C"],
      ["HR 6", "plant-a.dewatering.cake_moisture", "Влажность кека", "806", "8.06 %"],
    ],
    [12, 34, 26, 13, 15],
  ),

  h1("7. Расчётные технологические показатели"),
  body("Сервис historian вычисляет производные показатели (GET /api/v1/analytics/process) из последних измерений и активного профиля руды. Формулы прозрачны и проверяемы; отсутствие исходных данных даёт статус «нет данных» без подстановки оценок."),
  caption("Таблица 6. Расчётные показатели"),
  tbl(
    ["Показатель", "Формула", "Пояснение"],
    [
      ["Твёрдое в пульпе, %", "%S = ρт·(ρп−ρв)/((ρт−ρв)·ρп)·100", "массовый баланс на 1 м³ пульпы; ρт = 2.7 г/см³ (сульфидные руды)"],
      ["Удельный расход реагента, мЛ/т", "q = Qр·60 / Qт", "Qр — дозировка, мЛ/мин; Qт — производительность, т/ч"],
      ["Сухая производительность, т/ч", "Qсух = Qт·(1 − W/100)", "W — влажность концентрата, %"],
      ["Коридоры pH, P80, влажности", "сравнение с thresholds профиля", "статусы: норма / внимание (запас менее 10% диапазона) / нарушение"],
    ],
    [28, 34, 38],
  ),

  h1("8. Справочник API (v1)"),
  body("Все методы доступны под префиксом /api/v1. Приём данных идемпотентен: повторная доставка не создаёт дублей. Полный контракт — TZ_DEVELOPMENT.md, контрактные тесты — internal/api."),
  caption("Таблица 7. Основные методы"),
  tbl(
    ["Метод и путь", "Назначение"],
    [
      ["POST /ingest/telemetry(:batch)", "Приём телеметрии от шлюзов (одиночный/пакетный)"],
      ["GET /telemetry, /telemetry/latest", "История; последнее значение с признаком устаревания"],
      ["GET /telemetry/aggregate", "Агрегация в корзины (30с…1д; avg/min/max/sum/count/last)"],
      ["GET /analytics/process", "Расчётные технологические показатели (раздел 7)"],
      ["GET/POST /alarms/active, /alarms/{id}/ack", "Активные аварии; квитирование с аудитом"],
      ["GET/PUT/DELETE /alarms/limits", "Уставки (рационализация по ISA-18.2)"],
      ["GET/POST /profiles(+ approve/activate)", "Профили руды и управление изменениями"],
      ["/assets, /tags, /gateways", "Реестр оборудования, сигналов, шлюзов"],
      ["GET/POST /access/roles, /assignments, /audit", "Роли, назначения, журнал аудита"],
      ["GET /health", "Готовность сервиса и зависимостей"],
      ["GET /ws", "Живой поток событий панели (WebSocket)"],
      ["GET /reports/readings/csv, /reports/alerts/csv", "Экспорт истории и тревог в CSV"],
    ],
    [42, 58],
  ),

  h1("9. Переменные окружения"),
  caption("Таблица 8. Основные переменные (полный список — backend/.env.example)"),
  tbl(
    ["Переменная", "По умолчанию", "Назначение"],
    [
      ["HTTP_ADDR", "127.0.0.1:8000", "адрес прослушивания"],
      ["DB_URL", "sqlite://./cap.db", "хранилище: sqlite:// либо postgres://"],
      ["SIMULATOR_ENABLED", "true", "демо-генератор (выключать с реальными источниками)"],
      ["STALENESS_SECONDS", "60s", "порог признака устаревания данных"],
      ["SOURCE_DRIVER", "simulated", "драйвер шлюза: simulated/opcua/modbus/sparkplug"],
      ["MODBUS_ADDR / MODBUS_UNIT_ID", "— / 1", "адрес Modbus TCP-устройства и unit id"],
      ["MQTT_BROKER", "—", "адрес брокера для драйвера sparkplug"],
      ["OPCUA_ENDPOINT", "—", "конечная точка OPC UA"],
      ["TAGS", "демо-набор", "карта сигналов шлюза (раздел 6)"],
      ["EVENT_SINKS", "—", "приёмники живых событий (режим микросервисов)"],
      ["GATEWAY_BUFFER_DB", "./gateway-buffer.db", "файл буфера store-and-forward"],
    ],
    [30, 24, 46],
  ),

  h1("10. Безопасность и границы применения"),
  bullet("Платформа выполняет только операции чтения по отношению к АСУ ТП: драйверы не содержат функций записи, каналы управления отсутствуют."),
  bullet("Изменение конфигурации (реестр, профили, уставки, роли) требует соответствующего разрешения RBAC и фиксируется в неизменяемом журнале аудита (кто, что, когда)."),
  bullet("Квитирование аварии не влияет на технологический процесс и не скрывает исходную историю событий."),
  bullet("Симулятор и страница «Инструктор» предназначены только для разработки и обучения; в производственном развёртывании SIMULATOR_ENABLED=false."),
  bullet("Для производственной среды используйте PostgreSQL, выделенные учётные записи и сетевое разграничение (OT-сегмент отделён от корпоративной сети)."),

  h1("11. Диагностика неисправностей"),
  caption("Таблица 9. Типовые ситуации"),
  tbl(
    ["Симптом", "Вероятная причина", "Действие"],
    [
      ["Панель не открывается", "Сервер не запущен", "scripts/start.sh либо ./run.sh; проверьте /api/v1/health"],
      ["Карточки показывают «нет данных», точки сигналов красные", "Шлюз потерял связь с устройством", "Проверьте адрес/порт источника, состояние прибора; журнал шлюза gateway.log"],
      ["Значения не меняются при изменении регистра", "Прибор шлёт неизменное значение", "Это нормальное поведение: прямая линия = постоянный сигнал"],
      ["В логе шлюза «server unreachable», растёт буфер", "Сервер недоступен", "Запустите сервер; буфер досылается автоматически (backfill)"],
      ["403 при изменениях в реестре", "У субъекта нет роли", "Назначьте роль: Настройки → Уровни доступа (субъект admin — platform_admin)"],
      ["Оглавление PDF/Word без номеров страниц", "Поля не обновлены", "В Word: правый клик по оглавлению — «Обновить поле»"],
    ],
    [30, 30, 40],
  ),

  h1("12. Свидетельство о приёмке"),
  body("Выполнены автоматические проверки: контрактные и модульные тесты Go (gofmt, go vet, go build, go test), сборка веб-панели, живые сценарии: монолитный запуск, микросервисный запуск, сквозной сценарий отказа сервера с полным досылом буфера, приём телеметрии от реального Modbus TCP-устройства, расчётные показатели. Результат — соответствие функциональным требованиям MVP."),
  caption("Таблица 10. Сведения о версии"),
  tbl(
    ["Поле", "Значение"],
    [
      ["Обозначение документа", "CAP-РЭ-01 (руководство по эксплуатации)"],
      ["Версия платформы", "MVP, коммит main (см. git log)"],
      ["Дата составления", "10.09.2026"],
      ["Состав документа", "12 разделов, 10 таблиц, 4 рисунка"],
      ["Язык", "Русский"],
    ],
    [40, 60],
  ),
  body("Документ сопровождает исходный код репозитория и обновляется при изменении состава системы. Вопросы эксплуатации — файлы README.md, LOCAL_DEVELOPMENT.md, TZ_DEVELOPMENT.md в корне репозитория.", { after: 0 }),
];

// ── Document assembly ───────────────────────────────────────────────────────
const doc = new Document({
  styles: {
    default: {
      document: {
        run: { font: F, size: 24, color: P.body },
        paragraph: { spacing: { line: 312 } },
      },
      heading1: { run: { font: F, size: 32, bold: true, color: P.primary }, paragraph: { spacing: { before: 360, after: 160, line: 312 }, outlineLevel: 0 } },
      heading2: { run: { font: F, size: 28, bold: true, color: P.primary }, paragraph: { spacing: { before: 240, after: 120, line: 312 }, outlineLevel: 1 } },
    },
  },
  sections: [
    { // 1. Cover
      properties: { page: { size: { width: 11906, height: 16838 }, margin: { top: 0, bottom: 0, left: 0, right: 0 } } },
      children: buildCoverR1({
        title: "Complex Automation Pro (CAP)",
        subtitle: "Руководство по эксплуатации и технический паспорт",
        englishLabel: "CAP OPERATING MANUAL",
        metaLines: [
          "Обозначение: CAP-РЭ-01",
          "Версия платформы: MVP",
          "Правообладатель: 4codegit",
          "Дата: 10.09.2026",
        ],
        footerLeft: "Complex Automation Pro",
        footerRight: "Лицензия MIT",
      }),
    },
    { // 2. Front matter: TOC (Roman)
      properties: {
        type: SectionType.NEXT_PAGE,
        page: {
          size: { width: 11906, height: 16838 },
          margin: { top: 1440, bottom: 1440, left: 1701, right: 1417 },
          pageNumbers: { start: 1, formatType: NumberFormat.UPPER_ROMAN },
        },
      },
      headers: { default: docHeader },
      footers: { default: pageNumFooter() },
      children: [
        new Paragraph({
          alignment: AlignmentType.CENTER, spacing: { before: 480, after: 360 },
          children: [new TextRun({ text: "СОДЕРЖАНИЕ", bold: true, size: 32, font: F, color: P.primary })],
        }),
        new TableOfContents("Содержание", { hyperlink: true, headingStyleRange: "1-2" }),
        new Paragraph({
          spacing: { before: 200 },
          children: [new TextRun({
            text: "Примечание: оглавление построено на полях документа. После редактирования обновите номера страниц: правый клик по оглавлению — «Обновить поле».",
            italics: true, size: 18, color: "888888", font: F,
          })],
        }),
        new Paragraph({ children: [new PageBreak()] }),
      ],
    },
    { // 3. Body (Arabic from 1)
      properties: {
        type: SectionType.NEXT_PAGE,
        page: {
          size: { width: 11906, height: 16838 },
          margin: { top: 1440, bottom: 1440, left: 1701, right: 1417 },
          pageNumbers: { start: 1, formatType: NumberFormat.DECIMAL },
        },
      },
      headers: { default: docHeader },
      footers: { default: pageNumFooter() },
      children: bodyChildren,
    },
  ],
});

Packer.toBuffer(doc).then((buf) => {
  fs.writeFileSync("CAP_manual.docx", buf);
  console.log("CAP_manual.docx written,", buf.length, "bytes");
});

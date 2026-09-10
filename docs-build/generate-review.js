// Trilingual project review (Tajik / Russian / English) for CAP.
// Run from docs-build: node generate-review.js
const {
  Document, Packer, Paragraph, TextRun, Footer, PageNumber,
  AlignmentType, HeadingLevel, LevelFormat,
} = require("docx");
const fs = require("fs");

const FONT = { ascii: "Times New Roman", hAnsi: "Times New Roman" };
const BODY = 24;      // 12 pt
const INDENT = 709;   // 1.25 cm

// ---------- builders ----------

function partTitle(text, langLabel) {
  return [
    new Paragraph({
      heading: HeadingLevel.HEADING_1,
      alignment: AlignmentType.CENTER,
      spacing: { before: 120, after: 60, line: 312 },
      children: [new TextRun({ text, bold: true, size: 32, color: "000000", font: FONT })],
    }),
    new Paragraph({
      alignment: AlignmentType.CENTER,
      spacing: { after: 240, line: 312 },
      children: [new TextRun({ text: langLabel, italics: true, size: 22, color: "555555", font: FONT })],
    }),
  ];
}

function subjectLine(text) {
  return new Paragraph({
    alignment: AlignmentType.CENTER,
    spacing: { after: 280, line: 312 },
    indent: { left: 400, right: 400 },
    children: [new TextRun({ text, italics: true, size: 22, color: "333333", font: FONT })],
  });
}

// Paragraph with a bold rubric lead-in ("Актуальность темы.") and normal body text.
function rubricPara(lead, rest) {
  return new Paragraph({
    alignment: AlignmentType.JUSTIFIED,
    indent: { firstLine: INDENT },
    spacing: { before: 120, after: 120, line: 312 },
    children: [
      new TextRun({ text: lead + " ", bold: true, size: BODY, color: "000000", font: FONT }),
      new TextRun({ text: rest, size: BODY, color: "000000", font: FONT }),
    ],
  });
}

function listPara(reference, text) {
  return new Paragraph({
    numbering: { reference, level: 0 },
    alignment: AlignmentType.JUSTIFIED,
    spacing: { after: 60, line: 312 },
    children: [new TextRun({ text, size: BODY, color: "000000", font: FONT })],
  });
}

function listIntro(text) {
  return new Paragraph({
    alignment: AlignmentType.JUSTIFIED,
    indent: { firstLine: INDENT },
    spacing: { before: 120, after: 100, line: 312 },
    children: [new TextRun({ text, bold: true, size: BODY, color: "000000", font: FONT })],
  });
}

function signature(reviewerLine, dateLine) {
  return [
    new Paragraph({
      spacing: { before: 480, after: 40, line: 312 },
      children: [new TextRun({ text: reviewerLine, size: BODY, color: "000000", font: FONT })],
    }),
    new Paragraph({
      spacing: { after: 40, line: 312 },
      children: [new TextRun({ text: dateLine, size: BODY, color: "000000", font: FONT })],
    }),
  ];
}

function numberingConfig(ref) {
  return {
    reference: ref,
    levels: [{
      level: 0,
      format: LevelFormat.DECIMAL,
      text: "%1.",
      alignment: AlignmentType.LEFT,
      style: { paragraph: { indent: { left: 720, hanging: 360 } } },
    }],
  };
}

function section(children) {
  return {
    properties: {
      page: {
        size: { width: 11906, height: 16838 },
        margin: { top: 1134, bottom: 1134, left: 1701, right: 850 },
      },
    },
    footers: {
      default: new Footer({
        children: [new Paragraph({
          alignment: AlignmentType.CENTER,
          children: [new TextRun({ children: [PageNumber.CURRENT], size: 18, color: "888888", font: FONT })],
        })],
      }),
    },
    children,
  };
}

// ---------- Part I: Тоҷикӣ ----------

const tj = [
  ...partTitle("РЕЦЕНЗИЯ", "ба забони тоҷикӣ"),
  subjectLine("барои лоиҳаи «Complex Automation Pro (CAP)» — платформаи мониторинги равандҳои технологии фабрикаи бойсозӣ дар реъи вақт"),

  rubricPara(
    "Аҳамияти мавзӯъ.",
    "Автоматонияи корхонаҳои коркарди маъдан ва назорати равандҳои технологӣ дар реъи вақт — аз қабули кон то фиристодани консентрат — яке аз самтҳои муҳими модернизатсияи саноати кӯҳию металлургӣ аст. Лоиҳаи CAP платформаи ягонаи мониторингро пешниҳод менамояд: дар як процесс панели идора, интерфейси JSON API ва канали ахбории WebSocket дар реъи вақт ба корбар пешниҳод мешаванд. Чунин услуби ягона кор бо маълумотро содда мекунад ва вобастагӣ ба нармафзорҳои гуногунро кам мекунад."
  ),

  rubricPara(
    "Тавсифи мухтасари кор.",
    "Платформа бо забони барномасозии Go навишта шудааст ва ҳамчун як файли бинарии статикӣ ҷамъ меояд, ки ҷойгиркунӣ ва таъмиру нигоҳдориро осон мекунад. Барои нигоҳдории маълумот дар реҷаи намоишӣ SQLite, дар реҷаи истеҳсолӣ PostgreSQL истифода мешавад. Дар таркиби система шлюзи канорӣ (edge gateway) бо ҳофизаи буферии маҳаллӣ ҷойгир аст: ҳангоми қатъ шудани алоқа телеметрия дар ҷой нигоҳ дошта мешавад ва пас аз барқарор шудани пайванд пурра ба сервер интиқол дода мешавад. Симулятори дарунсохт имкон медиҳад, ки кори система бе таҷҳизоти воқеӣ санҷида ва намоиш дода шавад. Интерфейси корбар дар асоси React, TypeScript ва Vite сохта шудааст, диаграммаҳо бо китобхонаи Recharts тарҳрезӣ шудаанд ва мавзӯъҳои равшан ва торик пешбинӣ шудаанд."
  ),

  listIntro("Ҷиҳатҳои мусбати лоиҳа:"),
  listPara("adv-tj", "Меъмории возеҳи модулӣ (api, gateway, store, service, simulator, control) бо тақсими дақиқи вазифаҳо байни қисмҳо."),
  listPara("adv-tj", "Дастгирии протоколҳои саноатии Modbus, OPC UA ва MQTT — заминаи пайвасти бевосита ба таҷҳизоти воқеии истеҳсолот."),
  listPara("adv-tj", "Риояи марзи бехатарии OT: воситаҳои мониторинг ва симулятор ҳуқуқи идораи таҷҳизоти саноатиро надоранд."),
  listPara("adv-tj", "Пӯшониши хуби код бо санҷишҳо (зиёда аз 50 санҷиши воҳидӣ) ва мавҷудияти маҷмӯи пурраи ҳуҷҷатҳо: нақшаи генералии рушд, каталоги ҳассосҳо, вазифаи техникӣ, тавсифи техникӣ ва роҳномаи истифода."),

  listIntro("Мулоҳизаҳо ва тавсияҳо:"),
  listPara("rem-tj", "Ҳангоми гузариш ба истеҳсолот захирагии базаи PostgreSQL ва низоми нусхаҳои эҳтиётии маълумот пешбинӣ гардад."),
  listPara("rem-tj", "Санҷиши боркунӣ барои шлюз ва API дар ҳаҷми мақсадноки ҳассосҳо гузаронда шавад."),
  listPara("rem-tj", "Озмоиши саноатии пайвасти ба контроллерҳои воқеӣ бо иштироки хидмати бехатарии саноатӣ анҷом дода шавад."),

  rubricPara(
    "Хулоса.",
    "Лоиҳаи CAP кори пурра ва аз ҷиҳати техникӣ сатҳбаланд аст: вазифаҳои гузошташуда ҳал шудаанд, маҷмӯи ҳуҷҷатҳо ва санҷишҳо вазъи хуби лоиҳаро тасдиқ мекунанд. Платформа барои истифода дар корхонаҳои коркарди маъдан тавсия мешавад. Кор баҳои «аъло» сазовор аст."
  ),

  ...signature(
    "Рецензент: _________________________ /_____________________________/",
    "(нақш, ном, номпудак, имзо)                    «____» ______________ 2026 г."
  ),
];

// ---------- Part II: Русский ----------

const ru = [
  ...partTitle("РЕЦЕНЗИЯ", "на русском языке"),
  subjectLine("на проект «Complex Automation Pro (CAP)» — платформу мониторинга технологических процессов обогатительной фабрики"),

  rubricPara(
    "Актуальность темы.",
    "Автоматизация и оперативный мониторинг технологических процессов переработки минерального сырья — от приёма руды до отгрузки концентрата — являются одним из ключевых направлений модернизации горно-металлургической отрасли. Проект CAP реализует единую платформу мониторинга: в одном процессе пользователю доступны панель управления, JSON API и потоковый WebSocket-канал реального времени. Такой подход упрощает работу с данными и снижает зависимость от разнородного проприетарного программного обеспечения."
  ),

  rubricPara(
    "Краткая характеристика работы.",
    "Платформа реализована на языке Go и собирается в единый статический бинарный файл, что упрощает развёртывание и сопровождение. Для хранения данных используются SQLite (демонстрационный режим) и PostgreSQL (промышленная эксплуатация). В состав системы входит пограничный шлюз (edge gateway) с локальным буферным хранилищем: при обрыве связи телеметрия накапливается на устройстве и после восстановления соединения в полном объёме досылается на сервер. Встроенный симулятор позволяет проверять и демонстрировать работу системы без подключения к реальному оборудованию. Веб-интерфейс выполнен на React, TypeScript и Vite, графики построены на библиотеке Recharts, предусмотрены светлая и тёмная темы."
  ),

  listIntro("Достоинства проекта:"),
  listPara("adv-ru", "Прозрачная модульная архитектура (api, gateway, store, service, simulator, control) с чётким разделением ответственности между компонентами."),
  listPara("adv-ru", "Поддержка промышленных протоколов Modbus, OPC UA и MQTT, что создаёт основу для интеграции с реальным производственным оборудованием."),
  listPara("adv-ru", "Соблюдение границы безопасности OT: средства мониторинга и симулятор не имеют права управлять технологическим оборудованием."),
  listPara("adv-ru", "Хорошее покрытие кода тестами (более 50 модульных проверок) и полный комплект документации: генеральный план развития, каталог датчиков, техническое задание, техническая спецификация и руководство по эксплуатации."),

  listIntro("Замечания и рекомендации:"),
  listPara("rem-ru", "При переходе к промышленной эксплуатации предусмотреть резервирование базы данных PostgreSQL и регламент резервного копирования."),
  listPara("rem-ru", "Провести нагрузочное тестирование шлюза и API на целевом объёме датчиков."),
  listPara("rem-ru", "Выполнить опытно-промышленные испытания интеграции с реальными контроллерами с участием службы промышленной безопасности."),

  rubricPara(
    "Заключение.",
    "Проект CAP представляет собой законченную, технически грамотную разработку. Поставленные задачи решены, качество подтверждается набором тестов и полной документацией. Платформа рекомендуется к использованию на предприятиях по переработке минерального сырья. Работа заслуживает оценки «отлично»."
  ),

  ...signature(
    "Рецензент: _________________________ /_____________________________/",
    "(должность, Ф.И.О., подпись)                   «____» ______________ 2026 г."
  ),
];

// ---------- Part III: English ----------

const en = [
  ...partTitle("REVIEW", "in English"),
  subjectLine("of the project \u201CComplex Automation Pro (CAP)\u201D \u2014 a vendor-neutral monitoring platform for mineral-processing operations"),

  rubricPara(
    "Relevance of the topic.",
    "Automation and real-time monitoring of mineral-processing operations \u2014 from ore receiving through crushing, grinding and flotation to thickening, filtration and concentrate dispatch \u2014 is one of the key directions in the modernisation of the mining and metallurgical industry. The CAP project delivers a single unified monitoring platform: within one process the user gets a web dashboard, a JSON API and a real-time WebSocket feed. This unified approach simplifies data handling and reduces dependence on heterogeneous proprietary software."
  ),

  rubricPara(
    "Summary of the work.",
    "The platform is written in Go and built as a single static binary, which simplifies deployment and maintenance. SQLite is used for the demo configuration and PostgreSQL for production. The system includes an edge gateway with a local buffer database: when the connection to the server is lost, telemetry is accumulated on the device and forwarded in full once the link is restored. A built-in simulator makes it possible to test and demonstrate the system without real equipment. The web interface is built on React, TypeScript and Vite, with charts rendered by Recharts; both light and dark themes are provided."
  ),

  listIntro("Strengths of the project:"),
  listPara("adv-en", "Clear modular architecture (api, gateway, store, service, simulator, control) with a clean separation of responsibilities between components."),
  listPara("adv-en", "Support for industrial protocols Modbus, OPC UA and MQTT, which provides a basis for integration with real production equipment."),
  listPara("adv-en", "Respect for the OT safety boundary: monitoring components and the simulator are not authorised to control industrial equipment."),
  listPara("adv-en", "Good test coverage of the code (more than 50 unit checks) and a complete documentation set: master plan, sensor catalogue, development task specification, technical specification and an operating manual."),

  listIntro("Remarks and recommendations:"),
  listPara("rem-en", "When moving to production, provide redundancy for the PostgreSQL database and a regular backup procedure."),
  listPara("rem-en", "Perform load testing of the gateway and the API at the target sensor scale."),
  listPara("rem-en", "Carry out pilot industrial trials of the integration with real controllers, involving the industrial safety service."),

  rubricPara(
    "Conclusion.",
    "The CAP project is a complete and technically sound piece of engineering. The stated objectives have been met, and the quality is supported by the test suite and full documentation. The platform is recommended for use at mineral-processing enterprises. The work deserves the highest mark."
  ),

  ...signature(
    "Reviewer: __________________________ /______________________________/",
    "(position, full name, signature)                 \u201C____\u201D ______________ 2026"
  ),
];

// ---------- assembly ----------

const doc = new Document({
  styles: {
    default: {
      document: {
        run: { font: FONT, size: BODY, color: "000000" },
        paragraph: { spacing: { line: 312 } },
      },
    },
  },
  numbering: {
    config: [numberingConfig("adv-tj"), numberingConfig("rem-tj"),
             numberingConfig("adv-ru"), numberingConfig("rem-ru"),
             numberingConfig("adv-en"), numberingConfig("rem-en")],
  },
  sections: [section(tj), section(ru), section(en)],
});

Packer.toBuffer(doc).then((buf) => {
  const out = "/home/narziev/Documents/AutoPro/docs/CAP_Рецензия.docx";
  fs.writeFileSync(out, buf);
  console.log("written:", out, buf.length, "bytes");
});

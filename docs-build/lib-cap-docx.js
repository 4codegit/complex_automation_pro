// Shared docx helpers for CAP deliverables (passport, operator manual).
// Implements design-system R1 cover recipe + common rules (1.3 spacing, tables, TOC 3-section).
const {
  Document, Packer, Paragraph, TextRun, Table, TableRow, TableCell,
  ImageRun, PageBreak, Header, Footer, PageNumber, NumberFormat, SectionType,
  AlignmentType, HeadingLevel, WidthType, BorderStyle, ShadingType,
  LevelFormat, TableOfContents, TableLayoutType, VerticalAlign,
} = require("docx");
const fs = require("fs");
const { imageSize } = require("image-size");

// ---------- palette ----------
const P = {
  coverBg: "0F1B2D", titleColor: "FFFFFF", subtitleColor: "B8C4D4",
  metaColor: "D5DEEA", accent: "38BDF8", footerColor: "7C8DA6",
  primary: "0F172A", body: "1A2332", secondary: "506070",
  thFill: "E8EEF5", border: "9AA6B2", inside: "D8DEE6", warn: "9A6A00",
};

const FONT = { ascii: "Times New Roman", hAnsi: "Times New Roman" };
const MONO = { ascii: "Courier New", hAnsi: "Courier New" };
const BODY = 24; // 12 pt

// ---------- borders ----------
const NB = { style: BorderStyle.NONE, size: 0, color: "FFFFFF" };
const allNoBorders = { top: NB, bottom: NB, left: NB, right: NB,
  insideHorizontal: NB, insideVertical: NB };
const noBorders = { top: NB, bottom: NB, left: NB, right: NB };

// ---------- cover recipe R1 helpers (design-system.md) ----------
function splitTitleLines(title, charsPerLine) {
  if (title.length <= charsPerLine) return [title];
  const breakAfter = new Set([..."，。、；：！？", ..."的与和及之在于为", ..."-_—–·/", ..." \t"]);
  const lines = [];
  let remaining = title;
  while (remaining.length > charsPerLine) {
    let breakAt = -1;
    for (let i = charsPerLine; i >= Math.floor(charsPerLine * 0.6); i--) {
      if (i < remaining.length && breakAfter.has(remaining[i - 1])) { breakAt = i; break; }
    }
    if (breakAt === -1) {
      const limit = Math.min(remaining.length, Math.ceil(charsPerLine * 1.3));
      for (let i = charsPerLine + 1; i < limit; i++) {
        if (breakAfter.has(remaining[i - 1])) { breakAt = i; break; }
      }
    }
    if (breakAt === -1) breakAt = charsPerLine;
    lines.push(remaining.slice(0, breakAt).trim());
    remaining = remaining.slice(breakAt).trim();
  }
  if (remaining) lines.push(remaining);
  if (lines.length > 1 && lines[lines.length - 1].length <= 2) {
    const last = lines.pop();
    lines[lines.length - 1] += last;
  }
  return lines;
}

function calcTitleLayout(title, maxWidthTwips, preferredPt = 40, minPt = 24) {
  const charWidth = (pt) => pt * 12; // Cyrillic/Latin ≈ 0.6 em
  const charsPerLine = (pt) => Math.floor(maxWidthTwips / charWidth(pt));
  let titlePt = preferredPt;
  let lines;
  while (titlePt >= minPt) {
    const cpl = charsPerLine(titlePt);
    if (cpl < 2) { titlePt -= 2; continue; }
    lines = splitTitleLines(title, cpl);
    if (lines.length <= 3) break;
    titlePt -= 2;
  }
  if (!lines || lines.length > 3) {
    lines = splitTitleLines(title, charsPerLine(minPt));
    titlePt = minPt;
  }
  return { titlePt, titleLines: lines };
}

function calcCoverSpacing(params) {
  const { titleLineCount = 1, titlePt = 36, hasSubtitle = false, hasEnglishLabel = false,
    metaLineCount = 0, fixedHeight = 800, pageHeight = 16838,
    marginTop = 0, marginBottom = 0 } = params;
  const SAFETY = 1200;
  const usableHeight = pageHeight - marginTop - marginBottom - SAFETY;
  const titleHeight = titleLineCount * (titlePt * 23 + 200);
  const subtitleHeight = hasSubtitle ? (12 * 23 + 600) : 0;
  const englishLabelHeight = hasEnglishLabel ? (9 * 23 + 600) : 0;
  const metaHeight = metaLineCount * (10 * 23 + 100);
  const implicitParaHeight = 3 * 300;
  const contentHeight = titleHeight + subtitleHeight + englishLabelHeight +
    metaHeight + fixedHeight + implicitParaHeight;
  const remainingSpace = usableHeight - contentHeight;
  const safeRemaining = Math.max(remainingSpace, 400);
  const FOOTER_MIN = 800;
  const rawTop = Math.floor(safeRemaining * 0.45);
  const rawBottom = Math.floor(safeRemaining * 0.45);
  const bottomSpacing = Math.max(rawBottom, FOOTER_MIN);
  const topSpacing = Math.max(rawTop - Math.max(0, FOOTER_MIN - rawBottom), 400);
  const midSpacing = Math.max(safeRemaining - topSpacing - bottomSpacing, 0);
  return { topSpacing, midSpacing, bottomSpacing };
}

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
      children: [new TextRun({ text: config.englishLabel.split("").join("  "),
        size: 18, color: P.accent, font: FONT, characterSpacing: 40 })],
    }));
  }
  for (let i = 0; i < titleLines.length; i++) {
    children.push(new Paragraph({
      indent: { left: padL },
      spacing: { after: i < titleLines.length - 1 ? 100 : 300, line: Math.ceil(titlePt * 23), lineRule: "atLeast" },
      children: [new TextRun({ text: titleLines[i], size: titleSize, bold: true,
        color: P.titleColor, font: FONT })],
    }));
  }
  if (config.subtitle) {
    children.push(new Paragraph({
      indent: { left: padL, right: padR }, spacing: { after: 800, line: 312 },
      children: [new TextRun({ text: config.subtitle, size: 24, color: P.subtitleColor, font: FONT })],
    }));
  }
  for (const line of (config.metaLines || [])) {
    children.push(new Paragraph({
      indent: { left: padL + 200 }, spacing: { after: 80 },
      border: { left: accentLeft },
      children: [new TextRun({ text: line, size: 24, color: P.metaColor, font: FONT })],
    }));
  }
  children.push(new Paragraph({ spacing: { before: spacing.bottomSpacing } }));
  children.push(new Paragraph({
    indent: { left: padL, right: padR },
    border: { top: { style: BorderStyle.SINGLE, size: 2, color: P.accent, space: 8 } },
    spacing: { before: 200 },
    children: [
      new TextRun({ text: config.footerLeft || "", size: 16, color: P.footerColor, font: FONT }),
      new TextRun({ text: "                                        " }),
      new TextRun({ text: config.footerRight || "", size: 16, color: P.footerColor, font: FONT }),
    ],
  }));
  return [new Table({
    width: { size: 100, type: WidthType.PERCENTAGE },
    layout: TableLayoutType.FIXED,
    borders: allNoBorders,
    rows: [new TableRow({
      height: { value: 16838, rule: "exact" },
      children: [new TableCell({
        shading: { type: ShadingType.CLEAR, fill: P.coverBg }, borders: noBorders,
        children,
      })],
    })],
  })];
}

// ---------- body builders ----------
function h1(text) {
  return new Paragraph({
    heading: HeadingLevel.HEADING_1,
    spacing: { before: 400, after: 160, line: 312 },
    children: [new TextRun({ text, bold: true, size: 32, color: P.primary, font: FONT })],
  });
}
function h2(text) {
  return new Paragraph({
    heading: HeadingLevel.HEADING_2,
    spacing: { before: 280, after: 120, line: 312 },
    children: [new TextRun({ text, bold: true, size: 28, color: P.primary, font: FONT })],
  });
}
function para(text, opts = {}) {
  return new Paragraph({
    alignment: AlignmentType.JUSTIFIED,
    indent: { firstLine: 709 },
    spacing: { after: 100, line: 312 },
    children: [new TextRun({ text, size: BODY, color: P.body, font: FONT, ...opts })],
  });
}
function lead(leadText, rest) {
  return new Paragraph({
    alignment: AlignmentType.JUSTIFIED,
    indent: { firstLine: 709 },
    spacing: { before: 60, after: 100, line: 312 },
    children: [
      new TextRun({ text: leadText + " ", bold: true, size: BODY, color: P.primary, font: FONT }),
      new TextRun({ text: rest, size: BODY, color: P.body, font: FONT }),
    ],
  });
}
function numbered(ref, text) {
  return new Paragraph({
    numbering: { reference: ref, level: 0 },
    alignment: AlignmentType.JUSTIFIED,
    spacing: { after: 60, line: 312 },
    children: [new TextRun({ text, size: BODY, color: P.body, font: FONT })],
  });
}
function bullet(text) {
  return new Paragraph({
    bullet: { level: 0 },
    alignment: AlignmentType.JUSTIFIED,
    spacing: { after: 60, line: 312 },
    children: [new TextRun({ text, size: BODY, color: P.body, font: FONT })],
  });
}
function numberingConfig(ref) {
  return {
    reference: ref,
    levels: [{
      level: 0, format: LevelFormat.DECIMAL, text: "%1.",
      alignment: AlignmentType.LEFT,
      style: { paragraph: { indent: { left: 720, hanging: 360 } } },
    }],
  };
}
function mono(text) {
  return new Paragraph({
    spacing: { before: 60, after: 120, line: 276 },
    indent: { left: 400 },
    shading: { type: ShadingType.CLEAR, fill: "F4F7FA" },
    children: [new TextRun({ text, size: 16, color: P.body, font: MONO })],
  });
}
function caption(text) {
  return new Paragraph({
    keepNext: true,
    spacing: { before: 160, after: 80, line: 312 },
    children: [new TextRun({ text, bold: true, size: 21, color: P.secondary, font: FONT })],
  });
}
function makeTable(headers, rows, widths) {
  const cellMargins = { top: 60, bottom: 60, left: 120, right: 120 };
  const mk = (text, isHeader, w) => new TableCell({
    children: [new Paragraph({
      spacing: { line: 276 },
      children: [new TextRun({ text: String(text), bold: isHeader, size: 20,
        color: isHeader ? P.primary : P.body, font: FONT })],
    })],
    shading: isHeader ? { type: ShadingType.CLEAR, fill: P.thFill } : undefined,
    margins: cellMargins,
    width: w ? { size: w, type: WidthType.PERCENTAGE } : undefined,
    verticalAlign: VerticalAlign.CENTER,
  });
  return new Table({
    width: { size: 100, type: WidthType.PERCENTAGE },
    borders: {
      top: { style: BorderStyle.SINGLE, size: 4, color: P.border },
      bottom: { style: BorderStyle.SINGLE, size: 4, color: P.border },
      left: NB, right: NB,
      insideHorizontal: { style: BorderStyle.SINGLE, size: 2, color: P.inside },
      insideVertical: NB,
    },
    rows: [
      new TableRow({
        tableHeader: true, cantSplit: true,
        children: headers.map((t, i) => mk(t, true, widths ? widths[i] : undefined)),
      }),
      ...rows.map(r => new TableRow({
        cantSplit: true,
        children: r.map((t, i) => mk(t, false, widths ? widths[i] : undefined)),
      })),
    ],
  });
}
function img(path, displayWidthPx = 620) {
  const buf = fs.readFileSync(path);
  const dim = imageSize(buf);
  const h = Math.round(displayWidthPx * dim.height / dim.width);
  return new Paragraph({
    alignment: AlignmentType.CENTER,
    spacing: { before: 60, after: 160 },
    children: [new ImageRun({ data: buf, type: "png",
      transformation: { width: displayWidthPx, height: h } })],
  });
}

// ---------- page furniture ----------
const pgSize = { width: 11906, height: 16838 };
const pgMargin = { top: 1134, bottom: 1134, left: 1701, right: 850 };
function pageNumFooter() {
  return new Footer({
    children: [new Paragraph({
      alignment: AlignmentType.CENTER,
      children: [new TextRun({ children: [PageNumber.CURRENT], size: 18, color: "888888", font: FONT })],
    })],
  });
}
function docHeader(text) {
  return new Header({
    children: [new Paragraph({
      alignment: AlignmentType.RIGHT,
      border: { bottom: { style: BorderStyle.SINGLE, size: 2, color: P.inside, space: 4 } },
      children: [new TextRun({ text, size: 16, color: "999999", font: FONT })],
    })],
  });
}

function tocSection(docTitleRu) {
  return [
    new Paragraph({
      alignment: AlignmentType.CENTER,
      spacing: { before: 480, after: 360 },
      children: [new TextRun({ text: docTitleRu, bold: true, size: 32, font: FONT, color: P.primary })],
    }),
    new TableOfContents("Оглавление", { hyperlink: true, headingStyleRange: "1-2" }),
    new Paragraph({
      spacing: { before: 200 },
      children: [new TextRun({
        text: "Примечание: оглавление построено на кодах полей. После редактирования документа щёлкните по оглавлению правой кнопкой мыши и выберите «Обновить поле», чтобы обновить номера страниц.",
        italics: true, size: 18, color: "888888", font: FONT })],
    }),
    new Paragraph({ children: [new PageBreak()] }),
  ];
}

function assembleDoc({ coverChildren, frontMatter, bodyChildren, numberingRefs, headerText }) {
  return new Document({
    styles: {
      default: {
        document: {
          run: { font: FONT, size: BODY, color: P.body },
          paragraph: { spacing: { line: 312 } },
        },
        heading1: {
          run: { font: FONT, size: 32, bold: true, color: P.primary },
          paragraph: { spacing: { before: 400, after: 160, line: 312 }, outlineLevel: 0 },
        },
        heading2: {
          run: { font: FONT, size: 28, bold: true, color: P.primary },
          paragraph: { spacing: { before: 280, after: 120, line: 312 }, outlineLevel: 1 },
        },
      },
    },
    numbering: { config: (numberingRefs || []).map(numberingConfig) },
    sections: [
      { // cover
        properties: { page: { size: pgSize, margin: { top: 0, bottom: 0, left: 0, right: 0 } } },
        children: coverChildren,
      },
      { // front matter (TOC) — Roman numerals
        properties: {
          type: SectionType.NEXT_PAGE,
          page: { size: pgSize, margin: pgMargin,
            pageNumbers: { start: 1, formatType: NumberFormat.UPPER_ROMAN } },
        },
        footers: { default: pageNumFooter() },
        children: frontMatter,
      },
      { // body — Arabic from 1
        properties: {
          type: SectionType.NEXT_PAGE,
          page: { size: pgSize, margin: pgMargin,
            pageNumbers: { start: 1, formatType: NumberFormat.DECIMAL } },
        },
        headers: { default: docHeader(headerText) },
        footers: { default: pageNumFooter() },
        children: bodyChildren,
      },
    ],
  });
}

async function writeDoc(doc, outPath) {
  const buf = await Packer.toBuffer(doc);
  fs.writeFileSync(outPath, buf);
  console.log("written:", outPath, buf.length, "bytes");
}

module.exports = {
  P, FONT, BODY, buildCoverR1, h1, h2, para, lead, numbered, bullet,
  mono, caption, makeTable, img, assembleDoc, writeDoc, tocSection,
};

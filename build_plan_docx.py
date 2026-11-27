"""把 docs/ 下 00–17 章 Markdown 合成《开心同学AI助手-技术方案书.docx》。

用法：
    python build_plan_docx.py

规则（AGENTS.md §9.4）：
- .docx 是产物，禁止手工编辑；改内容先改 Markdown，再重跑本脚本
- 版本号只在本文件的 VERSION / REVISIONS 与封面同步
"""

from __future__ import annotations

import datetime as dt
import re
import sys
from pathlib import Path

from docx import Document
from docx.enum.section import WD_SECTION
from docx.enum.style import WD_STYLE_TYPE
from docx.enum.table import WD_TABLE_ALIGNMENT
from docx.enum.text import WD_ALIGN_PARAGRAPH
from docx.oxml import OxmlElement
from docx.oxml.ns import qn
from docx.shared import Cm, Pt, RGBColor

ROOT = Path(__file__).resolve().parent
DOCS = ROOT / "docs"
OUT = DOCS / "开心同学AI助手-技术方案书.docx"

VERSION = "v7.1"
TODAY = "2026-11-27"
TITLE = "开心同学 AI 助手"
SUBTITLE = "技术方案书"

REVISIONS = [
    ("v7.1", TODAY, "新增 docs/19 全流程下载监督与分步修复向导；修复弹窗/验证码卡死与二次开始不修复"),
    ("v7.0", "2026-11-26", "按 docs/ 最新 Markdown 源全量重建；纳入 00–17 共 18 章，含自动更新、代码签名、运行隔离与实测进度"),
    ("v6.8", "2026-11-22", "上一版 Word 产物（生成脚本缺失，已由 v7.0 取代）"),
]

# 章顺序：文件名, 是否作为一级标题
CHAPTERS = [
    "00-方案体检与技术架构.md",
    "01-设计纲领与治理规范.md",
    "02-模型清单与接入方案.md",
    "03-桌面端技术方案-Tauri.md",
    "04-全端技术方案定稿.md",
    "05-多模型辩论模块方案.md",
    "06-模型与软件部署归属表.md",
    "07-技能部署功能方案.md",
    "08-数据库设计与管理.md",
    "09-技能实施规范.md",
    "10-网站对接与授权方案.md",
    "11-红线分级与纠偏.md",
    "12-成本计费与降级.md",
    "13-功能需求基线与差距.md",
    "14-智能体运行时与安全隔离.md",
    "15-权限矩阵草案.md",
    "16-人工验收清单.md",
    "17-项目进度与分段执行报表.md",
    "19-下载监督与分步修复向导.md",
]

BODY_FONT = "宋体"
HEAD_FONT = "微软雅黑"
CODE_FONT = "Consolas"

TABLE_HEADER_FILL = "D9E2F3"
CODE_FILL = "F4F4F4"

STATS = {"h1": 0, "h2": 0, "h3": 0, "tables": 0, "code": 0, "paras": 0}


# ---------------- 字体与样式 ----------------


def set_run_font(run, name: str, size: float | None = None, bold: bool | None = None,
                 italic: bool | None = None, color: tuple[int, int, int] | None = None) -> None:
    run.font.name = name
    rpr = run._element.get_or_add_rPr()
    rfonts = rpr.get_or_add_rFonts()
    rfonts.set(qn("w:eastAsia"), name)
    if size is not None:
        run.font.size = Pt(size)
    if bold is not None:
        run.bold = bold
    if italic is not None:
        run.italic = italic
    if color is not None:
        run.font.color.rgb = RGBColor(*color)


def setup_styles(doc: Document) -> None:
    normal = doc.styles["Normal"]
    normal.font.name = BODY_FONT
    normal.font.size = Pt(10.5)
    normal.paragraph_format.space_after = Pt(4)
    normal.paragraph_format.line_spacing = 1.35
    rpr = normal.element.get_or_add_rPr()
    rfonts = rpr.get_or_add_rFonts()
    rfonts.set(qn("w:eastAsia"), BODY_FONT)

    heading_sizes = {1: 18, 2: 14, 3: 12, 4: 11}
    for level, size in heading_sizes.items():
        style = doc.styles[f"Heading {level}"]
        style.font.name = HEAD_FONT
        style.font.size = Pt(size)
        style.font.bold = True
        style.font.color.rgb = RGBColor(0x1F, 0x3A, 0x5F)
        style.paragraph_format.space_before = Pt(12 if level == 1 else 8)
        style.paragraph_format.space_after = Pt(6)
        srpr = style.element.get_or_add_rPr()
        srfonts = srpr.get_or_add_rFonts()
        srfonts.set(qn("w:eastAsia"), HEAD_FONT)

    for name in ("List Bullet", "List Number"):
        style = doc.styles[name]
        style.font.name = BODY_FONT
        style.font.size = Pt(10.5)
        style.paragraph_format.space_after = Pt(2)
        srpr = style.element.get_or_add_rPr()
        srfonts = srpr.get_or_add_rFonts()
        srfonts.set(qn("w:eastAsia"), BODY_FONT)

    if "CodeBlock" not in [s.name for s in doc.styles]:
        code_style = doc.styles.add_style("CodeBlock", WD_STYLE_TYPE.PARAGRAPH)
        code_style.font.name = CODE_FONT
        code_style.font.size = Pt(8.5)
        code_style.paragraph_format.space_after = Pt(0)
        code_style.paragraph_format.line_spacing = 1.0
        crpr = code_style.element.get_or_add_rPr()
        crfonts = crpr.get_or_add_rFonts()
        crfonts.set(qn("w:eastAsia"), CODE_FONT)


def add_page_number_footer(section) -> None:
    footer = section.footer
    para = footer.paragraphs[0]
    para.alignment = WD_ALIGN_PARAGRAPH.CENTER
    run = para.add_run()
    set_run_font(run, BODY_FONT, size=9, color=(0x66, 0x66, 0x66))
    fld_begin = OxmlElement("w:fldChar")
    fld_begin.set(qn("w:fldCharType"), "begin")
    instr = OxmlElement("w:instrText")
    instr.set(qn("xml:space"), "preserve")
    instr.text = " PAGE "
    fld_end = OxmlElement("w:fldChar")
    fld_end.set(qn("w:fldCharType"), "end")
    run._r.append(fld_begin)
    run._r.append(instr)
    run._r.append(fld_end)


def add_header_text(section, text: str) -> None:
    header = section.header
    para = header.paragraphs[0]
    para.alignment = WD_ALIGN_PARAGRAPH.RIGHT
    run = para.add_run(text)
    set_run_font(run, HEAD_FONT, size=8.5, color=(0x88, 0x88, 0x88))


# ---------------- 基础块 ----------------


def shade(cell, fill: str) -> None:
    tcpr = cell._tc.get_or_add_tcPr()
    shd = OxmlElement("w:shd")
    shd.set(qn("w:val"), "clear")
    shd.set(qn("w:color"), "auto")
    shd.set(qn("w:fill"), fill)
    tcpr.append(shd)


def shade_paragraph(para, fill: str) -> None:
    ppr = para._p.get_or_add_pPr()
    shd = OxmlElement("w:shd")
    shd.set(qn("w:val"), "clear")
    shd.set(qn("w:color"), "auto")
    shd.set(qn("w:fill"), fill)
    ppr.append(shd)


INLINE_RE = re.compile(
    r"\[[^\]]+\]\([^)]+\)"
    r"|`[^`]+`"
    r"|\*\*[^\n]+?\*\*"
    r"|~~[^~\n]+~~"
    r"|\*[^*\n]+\*"
)


def add_inline(para, text: str, size: float = 10.5, bold: bool = False,
               color: tuple[int, int, int] | None = None) -> None:
    """把一段 Markdown 行内文本写入段落（支持 **粗**、*斜*、~~删~~、`码`、链接）。

    加粗/斜体/链接内部允许嵌套代码，递归解析。
    """
    text = text.replace("\\|", "|").replace("<br>", "\n").replace("<br/>", "\n")
    pos = 0
    for match in INLINE_RE.finditer(text):
        if match.start() > pos:
            _plain(para, text[pos:match.start()], size, bold, color)
        token = match.group(0)
        if token.startswith("["):
            label, url = re.match(r"\[([^\]]+)\]\(([^)]+)\)", token).groups()
            add_inline(para, label, size=size, bold=bold, color=(0x1F, 0x4E, 0x9A))
            if url.startswith("http"):
                tail = para.add_run(f"（{url}）")
                set_run_font(tail, BODY_FONT, size=size - 1.5, color=(0x66, 0x66, 0x66))
        elif token.startswith("`"):
            run = para.add_run(token[1:-1])
            set_run_font(run, CODE_FONT, size=size - 0.5, bold=bold)
        elif token.startswith("**"):
            add_inline(para, token[2:-2], size=size, bold=True, color=color)
        elif token.startswith("~~"):
            start = len(para.runs)
            add_inline(para, token[2:-2], size=size, bold=bold, color=color)
            for run in para.runs[start:]:
                run.font.strike = True
        elif token.startswith("*"):
            start = len(para.runs)
            add_inline(para, token[1:-1], size=size, bold=bold, color=color)
            for run in para.runs[start:]:
                run.italic = True
        pos = match.end()
    if pos < len(text):
        _plain(para, text[pos:], size, bold, color)


def _plain(para, text: str, size: float, bold: bool, color) -> None:
    parts = text.split("\n")
    for idx, part in enumerate(parts):
        if idx:
            para.add_run().add_break()
        if part:
            run = para.add_run(part)
            set_run_font(run, BODY_FONT, size=size, bold=bold, color=color)


def add_heading(doc: Document, level: int, text: str, page_break: bool = False) -> None:
    para = doc.add_paragraph(style=f"Heading {min(level, 4)}")
    if page_break:
        para.paragraph_format.page_break_before = True
    add_inline(para, text, size={1: 18, 2: 14, 3: 12, 4: 11}.get(level, 11), bold=True)
    STATS[f"h{min(level, 3)}"] = STATS.get(f"h{min(level, 3)}", 0) + 1


def add_paragraph(doc: Document, text: str) -> None:
    para = doc.add_paragraph()
    add_inline(para, text)
    STATS["paras"] += 1


def add_quote(doc: Document, lines: list[str]) -> None:
    text = "\n".join(lines).strip()
    if not text:
        return
    para = doc.add_paragraph()
    para.paragraph_format.left_indent = Cm(0.6)
    para.paragraph_format.space_before = Pt(2)
    shade_paragraph(para, "F7F7F7")
    add_inline(para, text, size=10, color=(0x55, 0x55, 0x55))
    STATS["paras"] += 1


def add_code_block(doc: Document, lines: list[str]) -> None:
    if not lines:
        return
    para = doc.add_paragraph(style="CodeBlock")
    para.paragraph_format.left_indent = Cm(0.3)
    para.paragraph_format.space_before = Pt(4)
    para.paragraph_format.space_after = Pt(6)
    para.paragraph_format.line_spacing = 1.0
    shade_paragraph(para, CODE_FILL)
    for idx, line in enumerate(lines):
        if idx:
            para.add_run().add_break()
        if line:
            run = para.add_run(line.replace("\t", "    "))
            set_run_font(run, CODE_FONT, size=8.5)
    STATS["code"] += 1


def split_table_row(line: str) -> list[str]:
    line = line.strip()
    if line.startswith("|"):
        line = line[1:]
    if line.endswith("|"):
        line = line[:-1]
    parts: list[str] = []
    buf = ""
    escaped = False
    for ch in line:
        if escaped:
            buf += ch
            escaped = False
        elif ch == "\\":
            escaped = True
        elif ch == "|":
            parts.append(buf.strip())
            buf = ""
        else:
            buf += ch
    parts.append(buf.strip())
    return parts


def is_separator_row(line: str) -> bool:
    cells = split_table_row(line)
    return bool(cells) and all(re.fullmatch(r":?-{2,}:?", c or "") for c in cells)


def is_table_start(lines: list[str], idx: int) -> bool:
    if not lines[idx].lstrip().startswith("|"):
        return False
    return idx + 1 < len(lines) and is_separator_row(lines[idx + 1])


def add_table(doc: Document, rows: list[list[str]]) -> None:
    if not rows:
        return
    ncols = max(len(r) for r in rows)
    table = doc.add_table(rows=0, cols=ncols)
    table.style = "Table Grid"
    table.alignment = WD_TABLE_ALIGNMENT.CENTER
    table.autofit = True
    for ri, row in enumerate(rows):
        cells = table.add_row().cells
        for ci in range(ncols):
            text = row[ci] if ci < len(row) else ""
            cell = cells[ci]
            cell.text = ""
            para = cell.paragraphs[0]
            para.paragraph_format.space_before = Pt(1)
            para.paragraph_format.space_after = Pt(1)
            para.paragraph_format.line_spacing = 1.05
            add_inline(para, text, size=9, bold=(ri == 0))
            if ri == 0:
                shade(cell, TABLE_HEADER_FILL)
    STATS["tables"] += 1
    doc.add_paragraph().paragraph_format.space_after = Pt(2)


def add_list_item(doc: Document, text: str, ordered: bool, level: int) -> None:
    style = "List Number" if ordered else "List Bullet"
    para = doc.add_paragraph(style=style)
    if level > 0:
        para.paragraph_format.left_indent = Cm(0.75 * (level + 1))
    add_inline(para, text)
    STATS["paras"] += 1


# ---------------- Markdown 渲染 ----------------


def render_markdown(doc: Document, path: Path, first_heading_as_chapter: bool) -> None:
    lines = path.read_text(encoding="utf-8").splitlines()
    idx = 0
    in_code = False
    code_buf: list[str] = []
    quote_buf: list[str] = []
    seen_h1 = False

    def flush_quote() -> None:
        nonlocal quote_buf
        if quote_buf:
            add_quote(doc, quote_buf)
            quote_buf = []

    while idx < len(lines):
        line = lines[idx]
        stripped = line.strip()

        if in_code:
            if stripped.startswith("```"):
                add_code_block(doc, code_buf)
                code_buf = []
                in_code = False
            else:
                code_buf.append(line.rstrip())
            idx += 1
            continue

        if stripped.startswith("```"):
            flush_quote()
            in_code = True
            idx += 1
            continue

        if not stripped:
            flush_quote()
            idx += 1
            continue

        if is_table_start(lines, idx):
            flush_quote()
            rows: list[list[str]] = []
            header = split_table_row(lines[idx])
            rows.append(header)
            idx += 2
            while idx < len(lines) and lines[idx].lstrip().startswith("|"):
                rows.append(split_table_row(lines[idx]))
                idx += 1
            add_table(doc, rows)
            continue

        heading = re.match(r"^(#{1,6})\s+(.+?)\s*$", line)
        if heading:
            flush_quote()
            level = len(heading.group(1))
            text = heading.group(2)
            page_break = False
            if level == 1:
                if first_heading_as_chapter and not seen_h1:
                    page_break = True
                    seen_h1 = True
                else:
                    page_break = not seen_h1
                    seen_h1 = True
            add_heading(doc, level, text, page_break=page_break)
            idx += 1
            continue

        if re.fullmatch(r"(-{3,}|\*{3,}|_{3,})", stripped):
            flush_quote()
            idx += 1
            continue

        if stripped.startswith(">"):
            quote_buf.append(stripped.lstrip(">").strip())
            idx += 1
            continue

        bullet = re.match(r"^(\s*)[-*+]\s+(.+)$", line)
        if bullet:
            flush_quote()
            level = len(bullet.group(1)) // 2
            add_list_item(doc, bullet.group(2), ordered=False, level=level)
            idx += 1
            continue

        numbered = re.match(r"^(\s*)\d+[.)]\s+(.+)$", line)
        if numbered:
            flush_quote()
            level = len(numbered.group(1)) // 2
            add_list_item(doc, numbered.group(2), ordered=True, level=level)
            idx += 1
            continue

        flush_quote()
        add_paragraph(doc, stripped)
        idx += 1

    if in_code and code_buf:
        add_code_block(doc, code_buf)
    flush_quote()


# ---------------- 封面、目录、附录 ----------------


def build_cover(doc: Document) -> None:
    for _ in range(5):
        doc.add_paragraph()
    title_para = doc.add_paragraph()
    title_para.alignment = WD_ALIGN_PARAGRAPH.CENTER
    run = title_para.add_run(TITLE)
    set_run_font(run, HEAD_FONT, size=34, bold=True, color=(0x1F, 0x3A, 0x5F))

    sub_para = doc.add_paragraph()
    sub_para.alignment = WD_ALIGN_PARAGRAPH.CENTER
    run = sub_para.add_run(SUBTITLE)
    set_run_font(run, HEAD_FONT, size=22, bold=False, color=(0x44, 0x44, 0x44))

    doc.add_paragraph()
    meta = doc.add_paragraph()
    meta.alignment = WD_ALIGN_PARAGRAPH.CENTER
    run = meta.add_run(f"版本 {VERSION}     {TODAY}")
    set_run_font(run, BODY_FONT, size=12, color=(0x66, 0x66, 0x66))

    note = doc.add_paragraph()
    note.alignment = WD_ALIGN_PARAGRAPH.CENTER
    run = note.add_run("本文件由 docs/ 下 00–17 章 Markdown 自动生成，禁止直接编辑")
    set_run_font(run, BODY_FONT, size=9.5, color=(0x88, 0x88, 0x88))


def build_toc(doc: Document) -> None:
    doc.add_page_break()
    para = doc.add_paragraph()
    run = para.add_run("目录")
    set_run_font(run, HEAD_FONT, size=18, bold=True, color=(0x1F, 0x3A, 0x5F))
    hint = doc.add_paragraph()
    run = hint.add_run("（在 Word 中右键目录 → 更新域，可刷新页码）")
    set_run_font(run, BODY_FONT, size=9, color=(0x88, 0x88, 0x88))

    toc_para = doc.add_paragraph()
    run = toc_para.add_run()
    fld_begin = OxmlElement("w:fldChar")
    fld_begin.set(qn("w:fldCharType"), "begin")
    instr = OxmlElement("w:instrText")
    instr.set(qn("xml:space"), "preserve")
    instr.text = ' TOC \\o "1-3" \\h \\z \\u '
    fld_sep = OxmlElement("w:fldChar")
    fld_sep.set(qn("w:fldCharType"), "separate")
    placeholder = OxmlElement("w:t")
    placeholder.text = "按 F9 或右键“更新域”生成目录"
    fld_end = OxmlElement("w:fldChar")
    fld_end.set(qn("w:fldCharType"), "end")
    run._r.append(fld_begin)
    run._r.append(instr)
    run._r.append(fld_sep)
    run._r.append(placeholder)
    run._r.append(fld_end)


def build_revisions(doc: Document) -> None:
    add_heading(doc, 1, "附录 A　版本与变更记录", page_break=True)
    add_table(doc, [["版本", "日期", "变更内容"]] + [[v, d, n] for v, d, n in REVISIONS])
    add_paragraph(doc, "文档职责与单一真相原则见 AGENTS.md 第九节；同一信息只保留一个权威位置，其他文档引用不复制。")


def build_document() -> Document:
    doc = Document()
    setup_styles(doc)

    section = doc.sections[0]
    section.page_width = Cm(21.0)
    section.page_height = Cm(29.7)
    section.left_margin = Cm(2.4)
    section.right_margin = Cm(2.2)
    section.top_margin = Cm(2.2)
    section.bottom_margin = Cm(2.0)
    add_header_text(section, f"{TITLE} · {SUBTITLE} {VERSION}")
    add_page_number_footer(section)

    props = doc.core_properties
    props.title = f"{TITLE}{SUBTITLE}"
    props.subject = f"{TITLE}技术方案（{VERSION}，{TODAY} 重建）"
    props.author = "开心同学项目组"
    props.comments = "由 build_plan_docx.py 从 docs/*.md 生成"

    build_cover(doc)
    build_toc(doc)

    for name in CHAPTERS:
        path = DOCS / name
        if not path.exists():
            raise FileNotFoundError(f"缺少章节文件：{path}")
        render_markdown(doc, path, first_heading_as_chapter=True)

    build_revisions(doc)
    return doc


def main() -> int:
    doc = build_document()
    doc.save(OUT)
    print(f"OK: {OUT}")
    print(f"chapters={len(CHAPTERS)} h1={STATS['h1']} h2={STATS['h2']} h3={STATS['h3']}")
    print(f"tables={STATS['tables']} code_blocks={STATS['code']} paragraphs={STATS['paras']}")
    return 0


if __name__ == "__main__":
    sys.exit(main())

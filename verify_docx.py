"""校验技术方案书 Word 产物：章节数、表格数、空表、段落数。

用法：
    python verify_docx.py "docs/开心同学AI助手-技术方案书.docx"
"""

from __future__ import annotations

import sys
from pathlib import Path

from docx import Document

EXPECTED_H1 = 19  # 18 章 + 附录 A
MIN_TABLES = 100
MIN_PARAGRAPHS = 800


def main() -> int:
    if len(sys.argv) < 2:
        print("用法: python verify_docx.py <docx 路径>")
        return 2

    path = Path(sys.argv[1])
    if not path.exists():
        print(f"文件不存在：{path}")
        return 2

    doc = Document(path)
    h1 = h2 = h3 = 0
    for para in doc.paragraphs:
        style = para.style.name if para.style else ""
        if style == "Heading 1":
            h1 += 1
        elif style == "Heading 2":
            h2 += 1
        elif style == "Heading 3":
            h3 += 1

    empty_tables = 0
    for table in doc.tables:
        rows = len(table.rows)
        if rows <= 1:
            empty_tables += 1

    paragraphs = len(doc.paragraphs)
    failed = []
    if h1 < EXPECTED_H1:
        failed.append(f"H1 章节数 {h1} < 预期 {EXPECTED_H1}")
    if len(doc.tables) < MIN_TABLES:
        failed.append(f"表格数 {len(doc.tables)} < 预期 {MIN_TABLES}")
    if empty_tables:
        failed.append(f"空表 {empty_tables} 个")
    if paragraphs < MIN_PARAGRAPHS:
        failed.append(f"段落数 {paragraphs} < 预期 {MIN_PARAGRAPHS}")

    print(f"OK: {path}" if not failed else f"FAIL: {path}")
    print(f"h1={h1} h2={h2} h3={h3}")
    print(f"tables={len(doc.tables)} empty_tables={empty_tables} paragraphs={paragraphs}")
    for item in failed:
        print(f"  - {item}")
    return 0 if not failed else 1


if __name__ == "__main__":
    sys.exit(main())

"""检查技术方案书里是否残留 Markdown 标记（代码块除外）。

用法：
    python check_docx_text.py "docs/开心同学AI助手-技术方案书.docx"
"""

from __future__ import annotations

import sys
from pathlib import Path

from docx import Document

PATTERNS = {
    "加粗 **": "**",
    "反引号": "`",
    "链接 ](": "](",
    "换行 <br>": "<br",
    "表头分隔 |---": "|---",
    "转义竖线 \\|": "\\|",
}


def iter_texts(doc):
    for para in doc.paragraphs:
        style = para.style.name if para.style else ""
        if style == "CodeBlock":
            continue
        yield para.text, "paragraph"
    for table in doc.tables:
        for row in table.rows:
            for cell in row.cells:
                for para in cell.paragraphs:
                    yield para.text, "table"


def main() -> int:
    if len(sys.argv) < 2:
        print("用法: python check_docx_text.py <docx 路径>")
        return 2

    path = Path(sys.argv[1])
    if not path.exists():
        print(f"文件不存在：{path}")
        return 2

    doc = Document(path)
    hits: list[tuple[str, str, str]] = []
    for text, where in iter_texts(doc):
        if not text:
            continue
        for label, token in PATTERNS.items():
            if token in text:
                snippet = text.strip().replace("\n", " ")[:90]
                hits.append((label, where, snippet))

    if not hits:
        print("OK: 无 Markdown 残留")
        return 0

    print(f"FAIL: 发现 {len(hits)} 处 Markdown 残留")
    for label, where, snippet in hits[:30]:
        print(f"  [{label}][{where}] {snippet}")
    if len(hits) > 30:
        print(f"  ... 其余 {len(hits) - 30} 处省略")
    return 1


if __name__ == "__main__":
    sys.exit(main())

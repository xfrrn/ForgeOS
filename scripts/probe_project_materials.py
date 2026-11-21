"""Exercise file imports, duplicate detection and conflicting edits in a designated test project."""

import argparse
import re
from urllib.parse import urljoin
from uuid import uuid4

from playwright.sync_api import expect, sync_playwright
from probe_workbench_login import login_to_workbench


def check_file_imports(page, prefix):
    markdown_name = prefix + "-guide"
    plain_name = prefix + "-notes"
    body = "# " + markdown_name + "\n\nA **Markdown** example."
    literal = "# " + plain_name + "\n\nKeep **asterisks** and `backticks`."
    files = [
        {"name": markdown_name + ".md", "mimeType": "text/markdown", "buffer": body.encode()},
        {"name": plain_name + ".txt", "mimeType": "text/plain", "buffer": literal.encode()},
    ]
    page.get_by_role("button", name="添加到 ForgeOS").click()
    page.locator('input[type="file"]').set_input_files(files)
    page.get_by_role("button", name="导入可保存的 2 份资料").click()
    expect(page.get_by_text("已保存 · 修订 1", exact=True)).to_have_count(2)
    receipts = page.get_by_role("link", name="查看已存资料")
    markdown_url, plain_url = [urljoin(page.url, link.get_attribute("href")) for link in receipts.all()]
    page.goto(markdown_url)
    prose = page.locator(".material-reader-prose")
    expect(prose.get_by_role("heading", name=markdown_name)).to_be_visible()
    expect(prose.locator("strong")).to_have_text("Markdown")
    expect(page.locator(".materials-origin")).to_contain_text("ForgeOS")
    page.goto(plain_url)
    expect(prose.locator("pre code")).to_have_text(literal)
    expect(prose.locator("strong")).to_have_count(0)
    page.get_by_role("button", name="添加到 ForgeOS").click()
    page.locator('input[type="file"]').set_input_files(files[0])
    expect(page.get_by_text("正文相同，跳过 · " + markdown_name, exact=True)).to_be_visible()
    expect(page.get_by_role("button", name="导入可保存的 0 份资料")).to_be_disabled()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("url", help="ForgeOS workbench origin")
    parser.add_argument("project", type=int, help="registered test project ID")
    args = parser.parse_args()
    key = "probe-" + uuid4().hex

    with sync_playwright() as pw:
        browser = pw.chromium.launch(channel="chrome")

        def editor():
            page = browser.new_context().new_page()
            page.goto(f"{args.url.rstrip('/')}/#/projects/{args.project}/materials")
            login_to_workbench(page)
            return page

        first = editor()
        first.get_by_text("资料管理与历史", exact=True).click()
        first.get_by_role("button", name="手动建档", exact=True).click()
        first.get_by_label("资料标识").fill(key)
        first.get_by_label("标题", exact=True).fill(key)
        first.get_by_label("来源", exact=True).fill("Browser acceptance probe")
        first.get_by_role("textbox", name="正文", exact=True).fill("Revision one")
        first.get_by_role("button", name="保存修订 1").click()
        expect(first.locator(".material-reader-prose")).to_have_text("Revision one")
        ordinary_link = first.context.new_page()
        ordinary_link.goto(f"{args.url.rstrip('/')}/#/projects/{args.project}/materials?key={key}")
        expect(ordinary_link.locator(".material-reader-prose")).to_have_text("Revision one")
        second = editor()
        second.get_by_label("检索资料").fill(key)
        second.get_by_role("button", name="检索", exact=True).click()
        second.get_by_role("button", name=re.compile(key + r".*ForgeOS")).click()
        for page, text in [(first, "Revision two"), (second, "Concurrent edit")]:
            page.get_by_role("button", name="编辑此修订").click()
            page.get_by_role("textbox", name="正文", exact=True).fill(text)
        first.get_by_role("button", name="保存修订 2").click()
        expect(first.locator(".material-reader-prose")).to_have_text("Revision two")
        with second.expect_response(lambda r: r.request.method == "PUT" and r.url.endswith("/" + key)) as conflict:
            second.get_by_role("button", name="保存修订 2").click()
        assert conflict.value.status == 409
        expect(second.get_by_role("textbox", name="正文", exact=True)).to_have_value("Concurrent edit")
        expect(second.locator(".material-content")).to_have_text("Revision two")
        second.get_by_role("button", name="以修订 2 为基础继续编辑").click()
        second.get_by_role("button", name="保存修订 3").click()
        expect(second.locator(".material-reader-prose")).to_have_text("Concurrent edit")
        for original in ["Revision two", "Revision one"]:
            second.get_by_role("button", name="上一修订").click()
            expect(second.locator(".material-reader-prose")).to_have_text(original)
        ordinary_link.reload()
        expect(ordinary_link.locator(".material-reader-prose")).to_have_text("Concurrent edit")
        ordinary_link.close()
        check_file_imports(second, key)
        browser.close()
    print(f"PASS project {args.project}, material {key}, revisions 1–3, Markdown/TXT imports and duplicate detection")


if __name__ == "__main__":
    main()

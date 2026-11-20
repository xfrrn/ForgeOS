"""Exercise conflicting edits through the Owner workbench in a designated test project."""

import argparse
import os
from uuid import uuid4

from playwright.sync_api import expect, sync_playwright
from probe_workbench_login import login_to_workbench


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("url", help="ForgeOS workbench origin")
    parser.add_argument("project", type=int, help="registered test project ID")
    args = parser.parse_args()
    token = os.environ["FORGEOS_OWNER_TOKEN"]
    key = "probe-" + uuid4().hex

    with sync_playwright() as pw:
        browser = pw.chromium.launch(channel="chrome")

        def editor():
            page = browser.new_context().new_page()
            page.goto(f"{args.url.rstrip('/')}/#/projects/{args.project}/materials")
            login_to_workbench(page)
            return page

        first = editor()
        first.get_by_role("button", name="导入资料", exact=True).click()
        first.get_by_label("资料标识").fill(key)
        first.get_by_label("标题", exact=True).fill(key)
        first.get_by_label("来源", exact=True).fill("Browser acceptance probe")
        first.get_by_label("正文", exact=True).fill("Revision one")
        first.get_by_role("button", name="保存修订 1").click()
        expect(first.locator(".material-content")).to_have_text("Revision one")
        second = editor()
        second.get_by_label("检索资料").fill(key)
        second.get_by_role("button", name="检索", exact=True).click()
        second.get_by_role("button", name="查看 · " + key, exact=True).click()
        for page, text in [(first, "Revision two"), (second, "Concurrent edit")]:
            page.get_by_role("button", name="编辑此修订").click()
            page.get_by_label("正文", exact=True).fill(text)
        first.get_by_role("button", name="保存修订 2").click()
        expect(first.locator(".material-content")).to_have_text("Revision two")
        with second.expect_response(lambda r: r.request.method == "PUT" and r.url.endswith("/" + key)) as conflict:
            second.get_by_role("button", name="保存修订 2").click()
        assert conflict.value.status == 409
        expect(second.get_by_label("正文", exact=True)).to_have_value("Concurrent edit")
        expect(second.locator(".material-content")).to_have_text("Revision two")
        second.get_by_role("button", name="以修订 2 为基础继续编辑").click()
        second.get_by_role("button", name="保存修订 3").click()
        expect(second.locator(".material-content")).to_have_text("Concurrent edit")
        for original in ["Revision two", "Revision one"]:
            second.get_by_role("button", name="上一修订").click()
            expect(second.locator(".material-content")).to_have_text(original)
        browser.close()
    print(f"PASS project {args.project}, material {key}, revisions 1–3")


if __name__ == "__main__":
    main()

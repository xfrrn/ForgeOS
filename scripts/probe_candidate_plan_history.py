"""Check an execution whose newest test plan is pending and previous revision is covered."""
import argparse
import os

from playwright.sync_api import expect, sync_playwright


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("url")
    parser.add_argument("task", type=int)
    parser.add_argument("execution", type=int)
    args = parser.parse_args()
    with sync_playwright() as pw:
        browser = pw.chromium.launch(channel="chrome")
        page = browser.new_page(viewport={"width": 390, "height": 830})
        page.goto(f"{args.url.rstrip('/')}/#/tasks/{args.task}")
        login = page.get_by_label("Owner 令牌")
        login.fill(os.environ["FORGEOS_OWNER_TOKEN"])
        login.press("Enter")
        summary = page.locator("summary").filter(has=page.get_by_text(f"执行 #{args.execution}", exact=True))
        attempt = page.locator("details.detail-attempt").filter(has=summary)
        if attempt.get_attribute("open") is None:
            summary.click()
        panel = attempt.locator("section").filter(has=page.get_by_role("heading", name="候选测试计划", exact=True))
        with page.expect_response(lambda r: "/test-plans?execution_id=" in r.url) as latest_response:
            panel.get_by_role("button", name="查看测试计划").click()
        latest = latest_response.value.json()
        assert latest["execution_id"] == args.execution and latest["state"] == "awaiting_plan"
        newest = latest["revision"]
        assert newest >= 2
        pending = panel.get_by_text(f"等待 Test Agent 生成用例 · 修订 {newest}", exact=True)
        expect(pending).to_be_visible()
        with page.expect_response(lambda r: r.url.endswith(f"/test-plans?execution_id={args.execution}&revision={newest - 1}")) as previous_response:
            panel.get_by_role("button", name="上一版测试计划").click()
        previous = previous_response.value.json()
        assert previous["state"] == "covered" and previous["revision"] == newest - 1
        digest = panel.get_by_text(previous["coverage"]["plan_sha256"], exact=True)
        expect(digest).to_be_visible()
        expect(panel.get_by_text(f"覆盖审查通过 · 修订 {newest - 1}", exact=True)).to_be_visible()
        expect(pending).to_have_count(0)
        panel.get_by_role("button", name="下一版测试计划").click()
        expect(pending).to_be_visible()
        expect(digest).to_have_count(0)
        browser.close()


if __name__ == "__main__":
    main()

"""Check the Owner overview and its task queues against a running control plane."""
import argparse
import os

from playwright.sync_api import expect, sync_playwright


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("url")
    args = parser.parse_args()
    origin = args.url.rstrip("/")

    with sync_playwright() as pw:
        browser = pw.chromium.launch(channel="chrome")
        page = browser.new_page()
        page.goto(origin)
        page.locator("#owner").fill(os.environ["FORGEOS_OWNER_TOKEN"])
        with page.expect_response(lambda r: r.url == origin + "/tasks?limit=1" and r.ok) as loaded:
            page.locator("#owner").press("Enter")
        counts = loaded.value.json()["counts"]
        overview = page.get_by_role("heading", name="工作台总览")
        metrics = page.get_by_role("navigation", name="任务状态")
        for key, label, title in [
            ("ALL", "全部任务", "任务列表"),
            ("RUNNING", "执行中", "执行中任务"),
            ("WAITING_INPUT", "待答问题", "待答问题"),
            ("WAITING_ACCEPTANCE", "待验收", "验收队列"),
        ]:
            expect(overview).to_be_visible()
            metrics.get_by_role("link", name=f"{label} {counts[key]}", exact=True).click()
            expect(page.get_by_role("heading", name=title, exact=True)).to_be_visible()
            page.go_back()
        page.set_viewport_size({"width": 390, "height": 844})
        expect(metrics).to_be_visible()
        assert page.evaluate("document.documentElement.scrollWidth <= innerWidth")
        browser.close()
    print("Overview counts, queue navigation and mobile width passed.")


if __name__ == "__main__":
    main()

"""Verify the decision lifecycle in a designated test project using two Owner sessions."""

import argparse
from uuid import uuid4

from playwright.sync_api import expect, sync_playwright
from probe_workbench_login import login_to_workbench


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("url")
    parser.add_argument("project", type=int)
    args = parser.parse_args()
    prefix = "probe-" + uuid4().hex
    endpoint = f"/projects/{args.project}/decisions"

    with sync_playwright() as pw:
        browser = pw.chromium.launch(channel="chrome")

        def login():
            page = browser.new_context().new_page()
            page.goto(args.url.rstrip("/") + f"/#/projects/{args.project}/knowledge")
            login_to_workbench(page)
            page.get_by_role("navigation", name="知识库导航").get_by_role("link", name="项目决策").click()
            return page

        def card(page, statement):
            return page.locator("article").filter(has=page.get_by_role("heading", name=statement, exact=False))

        def fill(page, statement):
            page.get_by_label("决定内容").fill(statement)
            page.get_by_label("来源", exact=True).fill("QA review")
            page.get_by_label("适用条件").fill("Local integration tests")
            page.get_by_label("证据引用", exact=False).fill("docs/testing.md\nIssue #1")

        def save(page):
            with page.expect_response(lambda r: r.request.method == "POST" and r.url.endswith(endpoint)) as saved:
                page.get_by_role("button", name="保存提案").click()
            assert saved.value.status == 200
            return saved.value.json()["id"]

        first = login()
        initial = prefix + "-original"
        first.get_by_role("button", name="提出决策").click()
        fill(first, initial)

        def lose_ack(route):
            response = route.fetch()
            assert response.status == 200
            response.dispose()
            route.abort()

        first.route("**" + endpoint, lose_ack, times=1)
        first.get_by_role("button", name="保存提案").click()
        expect(first.get_by_role("alert")).to_be_visible()
        original = save(first)
        expect(card(first, initial)).to_have_count(1)
        card(first, initial).get_by_role("button", name="确认决策").click()
        expect(card(first, initial).get_by_role("button", name="提议替代")).to_be_visible()
        second = login()
        second.goto(args.url.rstrip("/") + f"/#/projects/{args.project}/decisions/{original}")
        for page, suffix in [(first, "-left"), (second, "-right")]:
            card(page, initial).get_by_role("button", name="提议替代").click()
            fill(page, prefix + suffix)
            save(page)
        card(first, prefix + "-left").get_by_role("button", name="确认决策").click()
        expect(card(first, prefix + "-left").get_by_role("button", name="标记失效")).to_be_visible()
        with second.expect_response(lambda r: r.url.endswith("/status")) as stale:
            card(second, prefix + "-right").get_by_role("button", name="确认决策").click()
        assert stale.value.status == 409
        card(second, prefix + "-right").get_by_role("button", name="拒绝提案").click()
        expect(card(second, prefix + "-right").get_by_text("已拒绝 · owner")).to_be_visible()
        card(first, prefix + "-left").get_by_role("button", name="标记失效").click()
        expect(card(first, prefix + "-left").get_by_text("已失效 · owner")).to_be_visible()
        first.get_by_role("combobox", name="决策状态").click()
        first.get_by_role("option", name="已替代").click()
        first.get_by_role("region", name="决策列表").get_by_role("link").filter(has_text=initial).click()
        record = card(first, initial)
        record.get_by_text("证据与历史").click()
        expect(record.get_by_text("docs/testing.md", exact=True)).to_be_visible()
        expect(record.locator("ol li")).to_have_count(3)
        print(f"PASS project {args.project}, original {original}, statements {prefix}")
        browser.close()


if __name__ == "__main__":
    main()

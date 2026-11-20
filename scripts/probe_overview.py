"""Check the project landing page and project overview against a running control plane."""
import argparse
from urllib.parse import parse_qs, urlsplit

from playwright.sync_api import expect, sync_playwright
from probe_workbench_login import login_to_workbench


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("url")
    parser.add_argument("--project-id", type=int, help="Project to check, defaulting to the first registered project")
    args = parser.parse_args()
    origin = args.url.rstrip("/")

    with sync_playwright() as pw:
        browser = pw.chromium.launch(channel="chrome")
        page = browser.new_page()
        page.goto(origin)
        with (
            page.expect_response(lambda r: r.url == origin + "/projects" and r.ok) as directory,
            page.expect_response(lambda r: r.url == origin + "/workers/control-plane" and r.ok),
        ):
            login_to_workbench(page)
        projects = directory.value.json()["items"]
        assert projects or args.project_id, "Register a project before checking the workbench."
        project = args.project_id or projects[0]["id"]
        root = f"/#/projects/{project}"
        expect(page.get_by_role("heading", name="项目", exact=True)).to_be_visible()
        expect(page.get_by_role("region", name="运行状态").get_by_text("Worker 在线")).to_be_visible()

        def project_counts(response):
            url = urlsplit(response.url)
            query = parse_qs(url.query)
            return response.ok and url.path == "/tasks" and query.get("limit") == ["1"] and query.get("project_id") == [str(project)]

        with (
            page.expect_response(project_counts) as loaded,
            page.expect_response(lambda r: r.url == f"{origin}/projects/{project}/overview-context" and r.ok),
        ):
            page.goto(origin + root)
        counts = loaded.value.json()["counts"]
        overview = page.get_by_role("heading", name="项目概览", exact=True)
        metrics = page.get_by_role("navigation", name="任务状态")
        for key, label, title, route in [
            ("ALL", "全部任务", "项目任务", "/tasks"),
            ("RUNNING", "执行中", "项目任务", "/tasks?status=RUNNING"),
            ("WAITING_INPUT", "待答问题", "待答问题", "/questions"),
            ("WAITING_ACCEPTANCE", "待验收", "待验收", "/reviews?tab=acceptance"),
        ]:
            expect(overview).to_be_visible()
            metrics.get_by_role("link", name=f"{label} {counts.get(key, 0)}", exact=True).click()
            expect(page).to_have_url(origin + root + route)
            expect(page.get_by_role("heading", name=title, exact=True)).to_be_visible()
            page.go_back()
        page.set_viewport_size({"width": 390, "height": 844})
        expect(metrics).to_be_visible()
        assert page.evaluate("document.documentElement.scrollWidth <= innerWidth")
        page.get_by_role("link", name="运行环境", exact=True).click()
        expect(page.get_by_role("heading", name="控制面", exact=True)).to_be_visible()
        expect(page.get_by_text("PostgreSQL", exact=True)).to_be_visible()
        assert page.evaluate("document.documentElement.scrollWidth <= innerWidth")
        browser.close()
    print("Project landing, runtime status, scoped overview, queue navigation and mobile width passed.")


if __name__ == "__main__":
    main()

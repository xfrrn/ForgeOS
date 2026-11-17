"""Verify browser plan editing on a fresh Queue handbook test Issue with a running Management Worker."""
import argparse
import os
import time

from playwright.sync_api import expect, sync_playwright


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("url")
    parser.add_argument("task", type=int)
    args = parser.parse_args()
    origin = args.url.rstrip("/")
    path = f"/tasks/{args.task}"
    token = os.environ["FORGEOS_OWNER_TOKEN"]

    with sync_playwright() as pw:
        browser = pw.chromium.launch(channel="chrome")
        client = pw.request.new_context(base_url=origin, extra_http_headers={"Authorization": "Bearer " + token})

        def get(endpoint):
            response = client.get(endpoint, max_redirects=0)
            assert response.ok, (endpoint, response.status)
            return response.json()

        task = get(path)
        assert task["kind"] == "codex_issue" and not task.get("plan") and task["status"] == "READY"
        paragraph = "Paused jobs count toward waiting capacity."
        for phrase in ("docs/queue-preview/index.html", "Queue handbook", paragraph):
            assert phrase in task["spec"]["issue"]["body"]

        def login(route):
            page = browser.new_context().new_page()
            page.goto(origin + "/#" + route)
            page.get_by_label("Owner 令牌").fill(token)
            page.get_by_label("Owner 令牌").press("Enter")
            return page

        def reviewed(revision):
            deadline = time.monotonic() + 180
            while time.monotonic() < deadline:
                rev = get(f"{path}/plans/{revision}")
                if rev.get("review"):
                    return rev
                time.sleep(.5)
            raise TimeoutError(f"Management did not review revision {revision}")

        first = login(path)
        first.get_by_role("button", name="起草 Goal 计划").click()
        form = first.locator("form.plan-editor")
        form.get_by_label("Goal 目标").fill("Document the requested Queue handbook example.")
        form.get_by_label("最低决策等级").select_option("D2")
        form.get_by_label("里程碑名称").fill("Handbook page")
        form.get_by_label("工作项标识", exact=True).fill("queue-page")
        form.get_by_label("工作项目标").fill("Publish a self-contained page.")
        form.get_by_role("textbox", name="范围", exact=True).fill("Edit README.md instead of creating the requested HTML page.")
        form.get_by_label("验收条件").fill("README.md has a heading.")
        form.get_by_label("交付物").fill("README.md")
        context = ["Frozen Issue " + str(task["spec"]["issue"]["number"]), "Base " + task["spec"]["base_commit"]]
        form.get_by_label("上下文引用", exact=False).fill("\n".join(context))
        form.get_by_role("button", name="发布草案修订 1").click()
        expect(first.get_by_role("heading", name="Goal 计划 · 修订 1")).to_be_visible()
        original = reviewed(1)
        assert original["review"]["verdict"] == "revise" and original["review"]["effective_level"] == "D2"

        second = login("/notifications")
        second.locator(f'a[href="#{path}/plans/1"]').click()
        expect(second.get_by_text(original["review"]["session_id"], exact=True)).to_be_visible()
        for page in (first, second):
            page.get_by_role("button", name="以修订 1 编辑新草案").click()
        other = second.locator("form.plan-editor")
        other.get_by_label("Goal 目标").fill("Another Owner's unsent changes")

        scope = "Create only docs/queue-preview/index.html, a self-contained static HTML page. Change no other file."
        criteria = f"Verify the h1 equals Queue handbook and the only paragraph equals {paragraph} Verify only the requested file changed."
        form.get_by_role("textbox", name="范围", exact=True).fill(scope)
        form.get_by_label("验收条件").fill(criteria)
        form.get_by_label("交付物").fill("docs/queue-preview/index.html")
        form.get_by_role("button", name="添加里程碑").click()
        stage = form.locator(".plan-stage").last
        stage.get_by_label("里程碑名称").fill("Temporary review notes")
        stage.get_by_role("button", name="添加工作项").click()
        stage.get_by_role("button", name="移除工作项").click()
        stage.get_by_role("button", name="移除里程碑").click()
        dependencies = form.get_by_label("依赖工作项标识", exact=False)
        dependencies.fill("queue-page")
        publish = form.get_by_role("button", name="发布草案修订 2")
        with first.expect_response(lambda r: r.url.endswith(path + "/plans") and r.request.method == "POST") as invalid:
            publish.click()
        assert invalid.value.status == 400
        expect(form.get_by_role("alert")).to_contain_text("cycle")
        dependencies.fill("")

        def lose_receipt(route):
            assert route.fetch().status == 200
            route.abort("failed")

        first.route("**" + path + "/plans", lose_receipt, times=1)
        publish.click()
        expect(form.get_by_role("alert")).to_contain_text("未确认发布结果")
        expect(form.get_by_role("textbox", name="范围", exact=True)).to_have_value(scope)
        publish.click()
        expect(first.get_by_role("heading", name="Goal 计划 · 修订 2")).to_be_visible()

        with second.expect_response(lambda r: r.url.endswith(path + "/plans") and r.request.method == "POST") as conflict:
            other.get_by_role("button", name="发布草案修订 2").click()
        assert conflict.value.status == 409
        expect(other.get_by_label("Goal 目标")).to_have_value("Another Owner's unsent changes")
        expect(other.get_by_role("alert")).to_contain_text("本地编辑已保留")
        final = reviewed(2)
        assert final["review"]["verdict"] == "approve" and final["minimum_level"] == "D2"
        assert get(path)["plan"]["revision"] == 2 and get(path + "/plans/1") == original
        item = final["plan"]["milestones"][0]["items"][0]
        assert item["scope"] == scope and item["acceptance_criteria"] == criteria and item["context_refs"] == context
        assert not get(path).get("children")
        first.get_by_role("button", name="批准并激活修订 2").click()
        expect(first.get_by_text("当前生效版本", exact=True)).to_be_visible()
        active = get(path)
        assert active["active_revision"] == 2 and len(active["children"]) == 1 and active["children"][0]["plan_revision"] == 2
        client.dispose()
        browser.close()
    print(f"PASS Goal {args.task}, reviews {original['review']['session_id']} / {final['review']['session_id']}, child {active['children'][0]['id']}")


if __name__ == "__main__":
    main()

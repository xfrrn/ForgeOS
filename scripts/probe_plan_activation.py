"""Approve a rolling plan in an isolated test Goal and verify stale and lost HTTP responses."""
import argparse
import os

from playwright.sync_api import expect, sync_playwright


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("url")
    parser.add_argument("goal", type=int, help="test Goal with an unactivated draft")
    args = parser.parse_args()
    origin = args.url.rstrip("/")
    path = f"/tasks/{args.goal}"
    token = os.environ["FORGEOS_OWNER_TOKEN"]

    with sync_playwright() as pw:
        browser = pw.chromium.launch(channel="chrome")
        page = browser.new_page()
        client = pw.request.new_context(base_url=origin, extra_http_headers={"Authorization": "Bearer " + token})
        goal = client.get(path, max_redirects=0).json()
        assert goal["status"] == "PLANNING"
        draft = goal["plan"]
        first = draft["revision"]
        page.goto(f"{origin}/#{path}/plans/{first}")
        page.get_by_label("Owner 令牌").fill(token)
        page.get_by_label("Owner 令牌").press("Enter")
        approve = page.get_by_role("button", name=f"批准并激活修订 {first}", exact=True)
        expect(approve).to_be_enabled()

        def publish_before_approval(route):
            new = client.post(path + "/plans", data={
                "previous_revision": first, "plan": draft["plan"],
                "minimum_level": draft.get("minimum_level") or "",
            }, max_redirects=0)
            assert new.ok and new.json()["revision"] == first + 1
            route.continue_()

        page.route(f"**{path}/plans/{first}/activate", publish_before_approval, times=1)
        with page.expect_response(lambda r: r.url.endswith(f"/plans/{first}/activate")) as stale:
            approve.click()
        assert stale.value.status == 409
        expect(approve).to_have_count(0)
        assert client.get(path).json().get("active_revision") == goal.get("active_revision")

        revision = first + 1
        page.get_by_role("link", name=f"最新修订 · {revision}").click()
        endpoint = f"{path}/plans/{revision}/activate"

        def lose_response(route):
            assert route.fetch().status == 200
            route.abort("failed")

        page.route("**" + endpoint, lose_response, times=1)
        page.get_by_role("button", name=f"批准并激活修订 {revision}").click()
        expect(page.get_by_text("未收到批准结果", exact=False)).to_be_visible()
        expect(page.get_by_text("当前生效版本", exact=True)).to_be_visible()
        active = client.get(path).json()
        assert active["active_revision"] == revision and active["children"]
        repeated = client.post(endpoint, max_redirects=0)
        assert repeated.ok and repeated.json()["children"] == active["children"]
        client.dispose()
        browser.close()
    print(f"PASS Goal {args.goal}, stale revision {first}, active revision {revision}")


if __name__ == "__main__":
    main()

"""Exercise the unpacked extension in Chrome for Testing with disposable pages and no credentials."""

import argparse
import json
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlsplit

from playwright.sync_api import expect, sync_playwright


class Site(BaseHTTPRequestHandler):
    def do_HEAD(self):
        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("X-ForgeOS-Task", "1")
        self.send_header("X-ForgeOS-Execution", "2")
        self.send_header("X-ForgeOS-Commit", "a" * 40)
        self.end_headers()

    def do_GET(self):
        self.do_HEAD()
        self.wfile.write(b'<main><input type="password" id="private-id" name="private-name" value="private-value"></main>')

    def log_message(self, *args):
        pass


def press_action(context, page, extension_id, button):
    page.bring_to_front()
    root = context.browser.new_browser_cdp_session()
    try:
        tabs = root.send("Target.getTargets", {"filter": [{"type": "tab"}]})["targetInfos"]
        tab = next(t for t in tabs if t["url"] == page.url)
        root.send("Extensions.triggerAction", {"targetId": tab["targetId"], "id": extension_id})
        for _ in range(50):
            pages = root.send("Target.getTargets", {"filter": [{"type": "page"}]})["targetInfos"]
            popup = next((p for p in pages if p["url"] == f"chrome-extension://{extension_id}/popup.html"), None)
            if popup:
                break
            page.wait_for_timeout(100)
        assert popup, "The toolbar did not open the extension popup"
        channel = root.send("Target.attachToTarget", {"targetId": popup["targetId"], "flatten": False})["sessionId"]
        replies = {}

        def receive(event):
            message = json.loads(event["message"])
            if event["sessionId"] == channel and "id" in message:
                replies[message["id"]] = message

        root.on("Target.receivedMessageFromTarget", receive)

        def evaluate(expression):
            number = len(replies) + 1
            command = {"id": number, "method": "Runtime.evaluate", "params": {"expression": expression, "returnByValue": True}}
            root.send("Target.sendMessageToTarget", {"sessionId": channel, "message": json.dumps(command)})
            for _ in range(50):
                if number in replies:
                    return replies[number]["result"]["result"].get("value")
                page.wait_for_timeout(50)
            raise AssertionError("The extension popup did not answer")

        for _ in range(50):
            if evaluate(f"Boolean(document.querySelector('#{button}:not([hidden])'))"):
                break
            page.wait_for_timeout(100)
        else:
            raise AssertionError(evaluate("document.querySelector('#status').textContent"))
        evaluate(f"setTimeout(() => document.querySelector('#{button}').click(), 0)")
        if button == "pair":
            for _ in range(50):
                if evaluate("document.querySelector('#status').textContent.startsWith('已连接')"):
                    break
                page.wait_for_timeout(100)
            else:
                raise AssertionError("The workbench was not paired")
            evaluate("setTimeout(() => window.close(), 50)")
            page.wait_for_timeout(100)
    finally:
        root.detach()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--browser", required=True, help="Path to Chrome for Testing with Extensions.triggerAction support")
    args = parser.parse_args()
    extension = Path(__file__).resolve().parents[1] / "extension"
    with ThreadingHTTPServer(("127.0.0.1", 0), Site) as server, tempfile.TemporaryDirectory() as profile, sync_playwright() as pw:
        threading.Thread(target=server.serve_forever, daemon=True).start()
        context = pw.chromium.launch_persistent_context(profile, executable_path=args.browser, headless=True,
            ignore_default_args=["--disable-extensions"], args=["--enable-unsafe-extension-debugging",
            f"--disable-extensions-except={extension}", f"--load-extension={extension}"])
        try:
            p = context.pages[0]
            p.goto("chrome://extensions")
            extension_id = p.locator("extensions-item").get_attribute("id")
            owner = f"http://127.0.0.1:{server.server_port}"
            p.goto(owner + "/#/tasks")
            press_action(context, p, extension_id, "pair")
            preview = "b" * 48
            p = context.new_page()
            p.goto(f"http://p-{preview}.localhost:{server.server_port}/?discard-query#discard-fragment")
            press_action(context, p, extension_id, "pick")
            tip = p.get_by_text("点击要批注的元素，按 Esc 取消")
            expect(tip).to_be_visible()
            p.keyboard.press("Escape")
            expect(tip).to_have_count(0)
            press_action(context, p, extension_id, "pick")
            expect(tip).to_be_visible()
            with context.expect_page() as handoff:
                p.locator("input").click()
            destination = handoff.value
            destination.wait_for_load_state()
            assert destination.url.startswith(owner + f"/#/tasks/1/previews/{preview}/feedback?")
            assert all(s not in destination.url for s in ("discard-query", "discard-fragment", "private-id", "private-name", "private-value"))
            fields = parse_qs(urlsplit(destination.url).fragment.split("?", 1)[1])
            assert fields["commit"] == ["a" * 40] and fields["execution"] == ["2"] and fields["path"] == ["/"]
            assert p.locator(fields["element"][0]).get_attribute("type") == "password"
            print("PASS: paired origin, cancellation, structural locator, candidate, path and privacy")
        finally:
            context.close()
            server.shutdown()


if __name__ == "__main__":
    main()
